import AVFoundation
import Foundation
import os

/// 把本地音频文件转写为句子级 EnglishSegment 数组。
///
/// 实现 (M5 v5 — 全局字符位置 ↔ 时间双向映射):
///   - 12s/段 切片 (.m4a, AVMutableComposition + AVAssetExportSession)
///   - 每段 ASR 拿全文, 不直接拆句, 记录 (text, offsetSec, durSec)
///   - 全部段文本拼成 fullText (段间用 1 char 空格分隔), 同时记录每段在 fullText 中的
///     字符范围 [charStart, charEnd)
///   - 在 fullText 全局上按 [.!?] 拆句 → 每句 [startChar, endChar)
///   - charPositionToSec(charPos): 落在哪个 chunk → chunk.offsetSec + (relCharPos /
///     chunkChars) * chunk.durSec;落在段间空隙 → 取上一段末尾秒
///   - 跨 chunk 边界的句子自然拼回成完整句, 时间戳延伸到该句真实跨度
///
/// 优点: chunk 边界腰斩长句的问题被根治, 不依赖 word timestamps (OpenRouter STT 不返)。
/// Tradeoff: 横跨多 chunk 的长句时长真实 (可能 20+s), 点 ▶ 听完整句 — 这是精听需要的。
///
/// Console.app 看进度:
///   `log stream --predicate 'subsystem == "com.kinesis.panel" AND category == "speech"'`
enum SpeechSegmenter {

    private static let logger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// 主区间 12s — 用于 chunkRange 的 offsetSec/durSec 和字符比例。
    static let chunkSec: Double = 12.0
    /// 前后各加 1.5s overlap — 让 STT 在边界附近的词有充足上下文不至于"保守省略"。
    /// 拿到 ~15s 文本后, 按比例 + 词边界裁回主区间对应的核心文本。
    static let chunkOverlapSec: Double = 1.5

    enum Failure: Error, LocalizedError {
        case noKey
        case audioLoadFailed(String)
        case allChunksFailed(String)

        var errorDescription: String? {
            switch self {
            case .noKey: "未设置 OpenRouter key (在 Settings 配)"
            case .audioLoadFailed(let s): "audio_load_failed: \(s)"
            case .allChunksFailed(let s): "all_chunks_failed: \(s)"
            }
        }
    }

    /// 转写本地音频文件 → 句子级 EnglishSegment 数组 (id/audioID 都为 0,由调用方填回)。
    static func transcribe(url: URL) async throws -> [EnglishSegment] {
        guard let key = KeyStore.get(), !key.isEmpty else {
            logger.error("no OpenRouter key configured")
            throw Failure.noKey
        }
        _ = key

        let asset = AVURLAsset(url: url)
        let totalSec: Double
        do {
            totalSec = try await asset.load(.duration).seconds
        } catch {
            throw Failure.audioLoadFailed(error.localizedDescription)
        }
        guard totalSec.isFinite, totalSec > 0 else {
            throw Failure.audioLoadFailed("invalid duration: \(totalSec)")
        }

        let totalChunks = Int(ceil(totalSec / chunkSec))
        logger.info("transcribe start: \(url.lastPathComponent, privacy: .public) totalSec=\(totalSec, format: .fixed(precision: 1)) chunks=\(totalChunks) chunkSec=\(chunkSec, format: .fixed(precision: 1))")

        // Phase 1: 每段独立 ASR, 收集文本 + chunk 时间范围
        var chunkTexts: [ChunkText] = []
        var lastErr = ""
        var successChunks = 0
        var offset: Double = 0
        var chunkIdx = 0

        while offset < totalSec {
            let coreStart = offset
            let coreEnd = min(offset + chunkSec, totalSec)
            let coreDur = coreEnd - coreStart
            let isFirst = chunkIdx == 0
            let isLast = coreEnd >= totalSec
            // export 实际跨度 = core ± overlap (clamp 边界)
            let exportStart = isFirst ? coreStart : max(0, coreStart - chunkOverlapSec)
            let exportEnd = isLast ? coreEnd : min(totalSec, coreEnd + chunkOverlapSec)
            let exportDur = exportEnd - exportStart
            let leadingFrac = exportDur > 0 ? (coreStart - exportStart) / exportDur : 0
            let trailingFrac = exportDur > 0 ? (exportEnd - coreEnd) / exportDur : 0

            let chunkURL = URL(fileURLWithPath: NSTemporaryDirectory())
                .appendingPathComponent("panel-asr-\(UUID().uuidString).m4a")
            do {
                try await exportChunk(asset: asset, startSec: exportStart, durationSec: exportDur, to: chunkURL)
                let bytes = (try? FileManager.default.attributesOfItem(atPath: chunkURL.path)[.size] as? Int) ?? 0
                let result = try await Transcriber.transcribeWithWordTimestamps(audioFile: chunkURL)
                successChunks += 1
                let raw = result.text.trimmingCharacters(in: .whitespacesAndNewlines)
                // overlap 让 STT 边界词不省略, 但不裁剪 — 接受跨段少量词重复 (比缺失好)。
                // 后续可加 fuzzy 拼接去重, 当前优先文本完整性。
                _ = leadingFrac; _ = trailingFrac  // silence unused warnings
                if !raw.isEmpty {
                    chunkTexts.append(ChunkText(text: raw, offsetSec: coreStart, durSec: coreDur))
                }
                logger.info("chunk \(chunkIdx + 1)/\(totalChunks): core=[\(coreStart, format: .fixed(precision: 1)),\(coreEnd, format: .fixed(precision: 1))]s export=\(exportDur, format: .fixed(precision: 1))s bytes=\(bytes) chars=\(raw.count)")
            } catch {
                lastErr = "\(error.localizedDescription)"
                logger.warning("chunk \(chunkIdx + 1)/\(totalChunks) failed: \(error.localizedDescription, privacy: .public)")
            }
            try? FileManager.default.removeItem(at: chunkURL)
            offset += chunkSec
            chunkIdx += 1
        }

        if chunkTexts.isEmpty {
            throw Failure.allChunksFailed(lastErr.isEmpty ? "no text returned" : lastErr)
        }

        // Phase 2: 拼 fullText + 维护每段字符范围 [charStart, charEnd) — 段间用 1 char 空格分隔
        let fullText = NSMutableString()
        var chunkRanges: [ChunkRange] = []
        for ct in chunkTexts {
            if fullText.length > 0 {
                fullText.append(" ")  // 段间分隔, 不属于任何 chunk 范围 (落在间隙)
            }
            let charStart = fullText.length
            fullText.append(ct.text)
            let charEnd = fullText.length
            chunkRanges.append(ChunkRange(
                offsetSec: ct.offsetSec,
                durSec: ct.durSec,
                charStart: charStart,
                charEnd: charEnd
            ))
        }

        // Phase 3: 全局拆句, 拿每句的字符范围
        let sentenceRanges = splitFullTextToSentenceRanges(fullText as String)

        logger.info("transcribe done: \(successChunks)/\(totalChunks) chunks ok, fullText.length=\(fullText.length) sentences=\(sentenceRanges.count)")

        // Phase 4: 字符范围 → 秒数范围 → EnglishSegment
        return sentenceRanges.enumerated().map { idx, sr -> EnglishSegment in
            let startSec = charPositionToSec(charPos: sr.startChar, chunks: chunkRanges)
            let endSec = charPositionToSec(charPos: sr.endChar, chunks: chunkRanges)
            return EnglishSegment(
                id: 0, audioID: 0, idx: idx,
                startMs: Int64(startSec * 1000),
                endMs: Int64(endSec * 1000),
                text: sr.text
            )
        }
    }

    // MARK: - 内部类型

    private struct ChunkText {
        let text: String
        let offsetSec: Double
        let durSec: Double
    }

    /// chunk 在 fullText 中的字符范围 + 在整段音频中的时间范围。
    /// 字符位置以 NSString.length (UTF-16 code unit) 度量, 跟 NSRegularExpression 一致。
    private struct ChunkRange {
        let offsetSec: Double
        let durSec: Double
        let charStart: Int  // inclusive
        let charEnd: Int    // exclusive
    }

    private struct SentenceRange {
        let startChar: Int  // inclusive, in fullText (NSString)
        let endChar: Int    // exclusive
        let text: String    // 已 trim
    }

    // MARK: - 全局字符位置 ↔ 时间映射

    /// 把 fullText 中的字符位置转成绝对秒数。
    /// 落在某 chunk 的 [charStart, charEnd] 区间 → chunk.offsetSec + (local/chunkChars) * chunk.durSec
    /// 落在两段间隙 (空格) → 上一段末尾秒 (= 下一段开始秒)
    /// 超出末尾 → 最后一段末尾秒
    private static func charPositionToSec(charPos: Int, chunks: [ChunkRange]) -> Double {
        var prevEndSec: Double = 0
        for chunk in chunks {
            if charPos < chunk.charStart {
                // 落在 prev 和当前 chunk 之间的间隙
                return prevEndSec
            }
            if charPos <= chunk.charEnd {
                let chunkChars = chunk.charEnd - chunk.charStart
                if chunkChars == 0 { return chunk.offsetSec }
                let local = max(0, charPos - chunk.charStart)
                let ratio = min(1.0, Double(local) / Double(chunkChars))
                return chunk.offsetSec + ratio * chunk.durSec
            }
            prevEndSec = chunk.offsetSec + chunk.durSec
        }
        return prevEndSec
    }

    // MARK: - 句子拆分 (全局 fullText)

    /// 在 fullText 上按 [.!?] 后跟空白处拆句子, 返回每句的 [startChar, endChar) 字符范围 + trim 后文本。
    /// 跨 chunk 边界的句子在这里自然拼回 (段间空格也是 \s, 不会触发拆分如果上一段没标点结尾)。
    private static func splitFullTextToSentenceRanges(_ fullText: String) -> [SentenceRange] {
        let nsText = fullText as NSString
        if nsText.length == 0 { return [] }
        let pattern = "(?<=[.!?])\\s+"
        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
            return [SentenceRange(startChar: 0, endChar: nsText.length, text: fullText)]
        }
        var lastIdx = 0
        var out: [SentenceRange] = []
        regex.enumerateMatches(in: fullText, options: [], range: NSRange(location: 0, length: nsText.length)) { match, _, _ in
            guard let m = match else { return }
            // 句子覆盖 [lastIdx, m.range.location), 即标点之后切开 (标点保留在上一句)
            if let sr = makeSentenceRange(nsText: nsText, rawStart: lastIdx, rawEnd: m.range.location) {
                out.append(sr)
            }
            lastIdx = m.range.location + m.range.length
        }
        // 最后一句 (无尾随分隔符)
        if lastIdx < nsText.length {
            if let sr = makeSentenceRange(nsText: nsText, rawStart: lastIdx, rawEnd: nsText.length) {
                out.append(sr)
            }
        }
        return out.isEmpty ? [SentenceRange(startChar: 0, endChar: nsText.length, text: fullText)] : out
    }

    /// 从 fullText 的 [rawStart, rawEnd) 抽出去掉前后空白的句子, 同时收紧字符范围。
    private static func makeSentenceRange(nsText: NSString, rawStart: Int, rawEnd: Int) -> SentenceRange? {
        guard rawStart < rawEnd else { return nil }
        let raw = nsText.substring(with: NSRange(location: rawStart, length: rawEnd - rawStart))
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return nil }
        let leading = leadingWhitespaceUTF16Length(raw)
        let trailing = trailingWhitespaceUTF16Length(raw)
        return SentenceRange(
            startChar: rawStart + leading,
            endChar: rawEnd - trailing,
            text: trimmed
        )
    }

    /// 用 UTF-16 code unit 计数 (跟 NSString.length / NSRegularExpression 一致)。
    // MARK: - 裁剪到核心区间 (overlap 文本去除)

    /// 输入 raw text (包含前后 overlap), 按 leadingFrac/trailingFrac 比例裁掉前后,
    /// 并对齐到词边界 (最近的空格), 避免在词中间切。
    /// leadingFrac=0 (第一段) → 不裁前;trailingFrac=0 (最后一段) → 不裁后。
    private static func trimToCorePortion(_ text: String, leadingFrac: Double, trailingFrac: Double) -> String {
        let nsText = text as NSString
        let totalLen = nsText.length
        if totalLen == 0 { return "" }

        let rawLeading = Int((Double(totalLen) * max(0, leadingFrac)).rounded(.down))
        let rawTrailing = Int((Double(totalLen) * max(0, trailingFrac)).rounded(.down))
        var alignedStart = min(rawLeading, totalLen)
        var alignedEnd = max(0, totalLen - rawTrailing)

        // 词边界 align: 前界往后挪到下一个空格之后, 跳过空格
        if leadingFrac > 0 && alignedStart < alignedEnd {
            while alignedStart < alignedEnd && !isASCIISpace(nsText.character(at: alignedStart)) {
                alignedStart += 1
            }
            while alignedStart < alignedEnd && isASCIISpace(nsText.character(at: alignedStart)) {
                alignedStart += 1
            }
        }
        // 后界往前挪到上一个空格之前
        if trailingFrac > 0 && alignedStart < alignedEnd {
            while alignedEnd > alignedStart && !isASCIISpace(nsText.character(at: alignedEnd - 1)) {
                alignedEnd -= 1
            }
            while alignedEnd > alignedStart && isASCIISpace(nsText.character(at: alignedEnd - 1)) {
                alignedEnd -= 1
            }
        }

        guard alignedStart < alignedEnd else { return "" }
        return nsText.substring(with: NSRange(location: alignedStart, length: alignedEnd - alignedStart))
    }

    private static func isASCIISpace(_ ch: unichar) -> Bool {
        // 0x20 space, 0x09 tab, 0x0A LF, 0x0D CR, 0xA0 NBSP
        return ch == 0x20 || ch == 0x09 || ch == 0x0A || ch == 0x0D || ch == 0xA0
    }

    private static func leadingWhitespaceUTF16Length(_ s: String) -> Int {
        var count = 0
        for scalar in s.unicodeScalars {
            if CharacterSet.whitespacesAndNewlines.contains(scalar) {
                count += scalar.utf16.count
            } else {
                break
            }
        }
        return count
    }

    private static func trailingWhitespaceUTF16Length(_ s: String) -> Int {
        var count = 0
        for scalar in s.unicodeScalars.reversed() {
            if CharacterSet.whitespacesAndNewlines.contains(scalar) {
                count += scalar.utf16.count
            } else {
                break
            }
        }
        return count
    }

    // MARK: - 切片 (.m4a)

    /// 切片走 AVMutableComposition (不用 exporter.timeRange — 后者对 mp3 输入会产出
    /// 看似 m4a 但实质静音的文件, 让 STT 模型 hallucinate "The quick brown fox" 等训练样本)。
    private static func exportChunk(
        asset: AVURLAsset, startSec: Double, durationSec: Double, to outputURL: URL
    ) async throws {
        let sourceTracks = try await asset.loadTracks(withMediaType: .audio)
        guard let sourceTrack = sourceTracks.first else {
            throw Failure.audioLoadFailed("no audio track in source")
        }
        let composition = AVMutableComposition()
        guard let compoTrack = composition.addMutableTrack(
            withMediaType: .audio, preferredTrackID: kCMPersistentTrackID_Invalid
        ) else {
            throw Failure.audioLoadFailed("create composition track failed")
        }
        let range = CMTimeRange(
            start: CMTime(seconds: startSec, preferredTimescale: 1000),
            duration: CMTime(seconds: durationSec, preferredTimescale: 1000)
        )
        try compoTrack.insertTimeRange(range, of: sourceTrack, at: .zero)

        guard let exporter = AVAssetExportSession(asset: composition, presetName: AVAssetExportPresetAppleM4A) else {
            throw Failure.audioLoadFailed("create AVAssetExportSession failed")
        }
        exporter.outputFileType = .m4a
        exporter.outputURL = outputURL
        await withCheckedContinuation { cont in
            exporter.exportAsynchronously {
                cont.resume()
            }
        }
        if exporter.status != .completed {
            let msg = exporter.error?.localizedDescription ?? "status=\(exporter.status.rawValue)"
            throw Failure.audioLoadFailed("export chunk failed: \(msg)")
        }
    }
}
