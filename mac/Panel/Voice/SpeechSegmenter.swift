import AVFoundation
import Foundation
import os

/// 把本地音频文件转写为句子级 EnglishSegment 数组。
///
/// 实现 (M5 v7 — 方案 B: 整段权威 + 切片锚 + fuzzy locate):
///   Phase 1: 整段 ASR 拿权威 fullText (无 chunk 边界漏词)。
///   Phase 2: 12s/段切片 ASR 拿 chunkTexts (每段对应明确时间锚 [offsetSec, +durSec])。
///   Phase 3: normalize fullText + 每 chunkText 为 token 序列 (lowercase 字母数字, 去标点)。
///   Phase 4: 对每个 chunkText, 用累加式 fuzzy locate (取首 6 词 + 末 6 词,
///            在 fullText cursor 之后找匹配) → chunkRange.charStart/charEnd in fullText。
///   Phase 5: splitFullTextToSentenceRanges(fullText) 拆完整句。
///   Phase 6: charPositionToSec 把句子的 fullText 字符位置 → 绝对秒数。
///
/// 优点: 文本来自整段 ASR (无漏词), timestamp 用 chunk 时间锚 (精度 ±1s)。
/// 限制: 整段 ASR 需 audio < 25MB (OpenAI 限制); 4-5 分钟 mp3 OK, 30 分钟 VOA 不行。
/// Fallback: 整段 ASR 失败 → 抛 asrFailed 错;  chunk 全失败 → 抛 allChunksFailed。
///
/// Console.app 看进度:
///   `log stream --predicate 'subsystem == "com.kinesis.panel" AND category == "speech"'`
enum SpeechSegmenter {

    private static let logger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// 主区间 12s — chunk 时间锚精度。
    static let chunkSec: Double = 12.0
    /// chunk 前后各 1.5s overlap 让 STT 边界词不省略, 提高 fuzzy locate 命中率。
    static let chunkOverlapSec: Double = 1.5

    enum Failure: Error, LocalizedError {
        case noKey
        case audioLoadFailed(String)
        case fullAsrFailed(String)
        case allChunksFailed(String)
        case noSentences

        var errorDescription: String? {
            switch self {
            case .noKey: "未设置 OpenRouter key (在 Settings 配)"
            case .audioLoadFailed(let s): "audio_load_failed: \(s)"
            case .fullAsrFailed(let s): "full_asr_failed: \(s)"
            case .allChunksFailed(let s): "all_chunks_failed: \(s)"
            case .noSentences: "no_sentences"
            }
        }
    }

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

        // === Phase 1: 整段 ASR 拿权威 fullText ===
        logger.info("phase 1 (full-asr) start: \(url.lastPathComponent, privacy: .public) totalSec=\(totalSec, format: .fixed(precision: 1))")
        let fullText: String
        do {
            let r = try await Transcriber.transcribeWithWordTimestamps(audioFile: url)
            fullText = r.text.trimmingCharacters(in: .whitespacesAndNewlines)
            logger.info("phase 1 done: fullText.length=\((fullText as NSString).length)")
        } catch {
            throw Failure.fullAsrFailed(error.localizedDescription)
        }
        guard !fullText.isEmpty else {
            throw Failure.fullAsrFailed("empty fullText")
        }

        // === Phase 2: 切片 ASR 拿时间锚 ===
        let totalChunks = Int(ceil(totalSec / chunkSec))
        logger.info("phase 2 (chunk-asr) start: chunks=\(totalChunks)")
        var chunkTexts: [ChunkText] = []
        var lastErr = ""
        var offset: Double = 0
        var chunkIdx = 0
        while offset < totalSec {
            let coreStart = offset
            let coreEnd = min(offset + chunkSec, totalSec)
            let coreDur = coreEnd - coreStart
            let isFirst = chunkIdx == 0
            let isLast = coreEnd >= totalSec
            let exportStart = isFirst ? coreStart : max(0, coreStart - chunkOverlapSec)
            let exportEnd = isLast ? coreEnd : min(totalSec, coreEnd + chunkOverlapSec)
            let exportDur = exportEnd - exportStart

            let chunkURL = URL(fileURLWithPath: NSTemporaryDirectory())
                .appendingPathComponent("panel-asr-\(UUID().uuidString).m4a")
            do {
                try await exportChunk(asset: asset, startSec: exportStart, durationSec: exportDur, to: chunkURL)
                let r = try await Transcriber.transcribeWithWordTimestamps(audioFile: chunkURL)
                let raw = r.text.trimmingCharacters(in: .whitespacesAndNewlines)
                if !raw.isEmpty {
                    chunkTexts.append(ChunkText(text: raw, offsetSec: coreStart, durSec: coreDur))
                }
                logger.info("chunk \(chunkIdx + 1)/\(totalChunks): core=[\(coreStart, format: .fixed(precision: 1)),\(coreEnd, format: .fixed(precision: 1))]s chars=\(raw.count)")
            } catch {
                lastErr = "\(error.localizedDescription)"
                logger.warning("chunk \(chunkIdx + 1) failed: \(error.localizedDescription, privacy: .public)")
            }
            try? FileManager.default.removeItem(at: chunkURL)
            offset += chunkSec
            chunkIdx += 1
        }
        guard !chunkTexts.isEmpty else {
            throw Failure.allChunksFailed(lastErr.isEmpty ? "no chunk text" : lastErr)
        }

        // === Phase 3: normalize fullText + chunkTexts 为 token 序列 ===
        let fullNorm = normalize(fullText)
        logger.info("phase 3: fullText.tokens=\(fullNorm.tokens.count)")

        // === Phase 4: 累加式 fuzzy locate, 算每个 chunk 在 fullText 中的字符范围 ===
        var chunkRanges: [ChunkRange] = []
        var cursorTok = 0
        var locateOk = 0
        var locateFallback = 0
        for (i, ct) in chunkTexts.enumerated() {
            let cn = normalize(ct.text)
            if let loc = locateChunkInFullText(
                chunkTokens: cn.tokens,
                fullNorm: fullNorm,
                cursorTokenIdx: cursorTok
            ) {
                chunkRanges.append(ChunkRange(
                    offsetSec: ct.offsetSec,
                    durSec: ct.durSec,
                    charStart: loc.startChar,
                    charEnd: loc.endChar
                ))
                cursorTok = loc.nextCursorTokenIdx
                locateOk += 1
            } else {
                // Fallback: 用时间比例反推 charStart/charEnd
                let nsLen = (fullText as NSString).length
                let estStart = Int((Double(nsLen) * ct.offsetSec / totalSec).rounded())
                let estEnd = Int((Double(nsLen) * (ct.offsetSec + ct.durSec) / totalSec).rounded())
                chunkRanges.append(ChunkRange(
                    offsetSec: ct.offsetSec,
                    durSec: ct.durSec,
                    charStart: max(0, min(nsLen, estStart)),
                    charEnd: max(0, min(nsLen, estEnd))
                ))
                locateFallback += 1
                logger.warning("chunk \(i) fuzzy locate failed, fallback to time ratio")
            }
        }
        logger.info("phase 4: located=\(locateOk) fallback=\(locateFallback)")

        // === Phase 5: 拆句 ===
        let sentenceRanges = splitFullTextToSentenceRanges(fullText)
        logger.info("phase 5: sentences=\(sentenceRanges.count)")
        guard !sentenceRanges.isEmpty else {
            throw Failure.noSentences
        }

        // === Phase 6: 字符 → 秒 → segment ===
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

    private struct ChunkRange {
        let offsetSec: Double
        let durSec: Double
        let charStart: Int  // inclusive, in fullText (NSString length)
        let charEnd: Int    // exclusive
    }

    private struct SentenceRange {
        let startChar: Int
        let endChar: Int
        let text: String
    }

    private struct NormalizedTokens {
        let tokens: [String]      // lowercase 字母+数字
        let origStarts: [Int]     // 每 token 在原文 (NSString) 中的起点
        let origEnds: [Int]       // 每 token 在原文中的末尾 (exclusive)
    }

    // MARK: - normalize + tokenize (lowercase 字母数字, 去标点)

    private static func normalize(_ s: String) -> NormalizedTokens {
        let nsText = s as NSString
        var tokens: [String] = []
        var starts: [Int] = []
        var ends: [Int] = []

        var inToken = false
        var tokStart = 0
        var currentToken = ""

        for i in 0..<nsText.length {
            let ch = nsText.character(at: i)
            let scalar = UnicodeScalar(ch)
            let isWord: Bool
            if let sc = scalar {
                isWord = CharacterSet.alphanumerics.contains(sc)
            } else {
                isWord = false
            }
            if isWord {
                if !inToken {
                    tokStart = i
                    currentToken = ""
                    inToken = true
                }
                // 用 String(utf16CodeUnits:count:) lowercased 拿到该字符的小写形式
                let chStr = String(utf16CodeUnits: [ch], count: 1).lowercased()
                currentToken.append(chStr)
            } else {
                if inToken {
                    tokens.append(currentToken)
                    starts.append(tokStart)
                    ends.append(i)
                    inToken = false
                    currentToken = ""
                }
            }
        }
        if inToken {
            tokens.append(currentToken)
            starts.append(tokStart)
            ends.append(nsText.length)
        }
        return NormalizedTokens(tokens: tokens, origStarts: starts, origEnds: ends)
    }

    // MARK: - fuzzy locate chunk in fullText (累加式)

    /// 在 fullNorm 的 [cursorTokenIdx, ?] 区间内 fuzzy locate chunkTokens, 返回:
    ///   - startChar: chunk 第一个对齐 token 在 fullText 的字符起点
    ///   - endChar:   chunk 最后一个对齐 token 在 fullText 的字符末尾 (exclusive)
    ///   - nextCursorTokenIdx: 下一个 chunk 的 cursor 起点 (= last matched token idx + 1)
    /// 算法:
    ///   - 取 chunk 前 headWindow 个 token 作 head, 末 tailWindow 个 token 作 tail
    ///   - head 在 cursor 之后 maxLookahead 内找完全匹配
    ///   - 找到 → 在 head 之后预期位置 ±5 token 内找 tail 完全匹配
    ///   - tail 找到 → 返回 (head 起点, tail 终点, tail 末 token + 1)
    /// chunk 太短时降级:
    ///   - chunk tokens < 4 → 整 chunk 序列在 cursor 之后找完全匹配
    private static func locateChunkInFullText(
        chunkTokens: [String],
        fullNorm: NormalizedTokens,
        cursorTokenIdx: Int
    ) -> (startChar: Int, endChar: Int, nextCursorTokenIdx: Int)? {
        guard !chunkTokens.isEmpty, cursorTokenIdx < fullNorm.tokens.count else { return nil }

        let headWindow = min(6, chunkTokens.count)
        let tailWindow = min(6, chunkTokens.count)

        // chunk 太短 → 整序列匹配
        if chunkTokens.count < 4 {
            if let idx = findTokenSequence(needle: chunkTokens, hay: fullNorm.tokens, startIdx: cursorTokenIdx, maxLookahead: 60) {
                let endTokIdx = idx + chunkTokens.count - 1
                return (
                    startChar: fullNorm.origStarts[idx],
                    endChar: fullNorm.origEnds[endTokIdx],
                    nextCursorTokenIdx: endTokIdx + 1
                )
            }
            return nil
        }

        let head = Array(chunkTokens.prefix(headWindow))
        let tail = Array(chunkTokens.suffix(tailWindow))

        guard let headIdx = findTokenSequence(needle: head, hay: fullNorm.tokens, startIdx: cursorTokenIdx, maxLookahead: 60) else {
            return nil
        }

        // tail 期望位置 = headIdx + chunkTokens.count - tailWindow, 允许 ±5 token 偏差
        let tailExpected = headIdx + chunkTokens.count - tailWindow
        let tailSearchStart = max(headIdx + headWindow, tailExpected - 5)
        guard let tailIdx = findTokenSequence(needle: tail, hay: fullNorm.tokens, startIdx: tailSearchStart, maxLookahead: 12) else {
            return nil
        }

        let startTok = headIdx
        let endTok = tailIdx + tailWindow - 1
        guard endTok < fullNorm.tokens.count else { return nil }

        return (
            startChar: fullNorm.origStarts[startTok],
            endChar: fullNorm.origEnds[endTok],
            nextCursorTokenIdx: endTok + 1
        )
    }

    /// 在 hay[startIdx..(startIdx+maxLookahead)] 内找 needle 完全匹配的位置, 返回起点 idx 或 nil。
    private static func findTokenSequence(needle: [String], hay: [String], startIdx: Int, maxLookahead: Int) -> Int? {
        guard !needle.isEmpty else { return nil }
        let endIdx = min(hay.count - needle.count + 1, startIdx + maxLookahead)
        if startIdx >= endIdx { return nil }
        for i in startIdx..<endIdx {
            var matched = true
            for j in 0..<needle.count {
                if hay[i + j] != needle[j] { matched = false; break }
            }
            if matched { return i }
        }
        return nil
    }

    // MARK: - 字符位置 → 秒 (复用 C 方案的算法, chunkRange 来自 fuzzy locate)

    private static func charPositionToSec(charPos: Int, chunks: [ChunkRange]) -> Double {
        var prevEndSec: Double = 0
        for chunk in chunks {
            if charPos < chunk.charStart {
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

    // MARK: - 句子拆分 (在 fullText 上)

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
            if let sr = makeSentenceRange(nsText: nsText, rawStart: lastIdx, rawEnd: m.range.location) {
                out.append(sr)
            }
            lastIdx = m.range.location + m.range.length
        }
        if lastIdx < nsText.length {
            if let sr = makeSentenceRange(nsText: nsText, rawStart: lastIdx, rawEnd: nsText.length) {
                out.append(sr)
            }
        }
        return out.isEmpty ? [SentenceRange(startChar: 0, endChar: nsText.length, text: fullText)] : out
    }

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

    // MARK: - 切片导出 (.m4a, AVMutableComposition)

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
