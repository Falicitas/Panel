import AVFoundation
import Foundation
import os

/// 把本地音频文件转写为句子级 EnglishSegment 数组。
///
/// 实现 (M5 v4): 复用 F12 的 OpenRouter pipeline + 段内字符比例线性估算 timestamp。
///   - OpenRouter STT 不透传 verbose_json + timestamp_granularities, word timestamps 拿不到。
///   - 改用 **小段策略**: 切片 12s/段 (.m4a, AVMutableComposition + AVAssetExportSession),
///     每段 ASR 拿全文 → 段内按 [.!?] 拆句 → 按字符数比例线性估算 [startMs, endMs)。
///     12s 段内通常 1-3 句, 估算误差 < 1s, 段播放体验可接受。
///   - 缺点: API 调用次数 ~ 4x (50s→12s), 4 分钟音频 = ~20 段 ≈ $0.05 ($0.0013/段)。
///   - 改进: Phase 1 直连 OpenAI API + whisper-1 verbose_json 拿真 word timestamps。
///
/// Console.app 看进度:
///   `log stream --predicate 'subsystem == "com.kinesis.panel" AND category == "speech"'`
enum SpeechSegmenter {

    private static let logger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// 每段 12s — 小段保证段内字符比例估算误差 < 1s。
    static let chunkSec: Double = 12.0

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

        var allSentences: [Sentence] = []
        var lastErr = ""
        var successChunks = 0
        var offset: Double = 0
        var chunkIdx = 0

        while offset < totalSec {
            let chunkActualSec = min(chunkSec, totalSec - offset)
            let chunkURL = URL(fileURLWithPath: NSTemporaryDirectory())
                .appendingPathComponent("panel-asr-\(UUID().uuidString).m4a")
            do {
                try await exportChunk(asset: asset, startSec: offset, durationSec: chunkActualSec, to: chunkURL)
                let bytes = (try? FileManager.default.attributesOfItem(atPath: chunkURL.path)[.size] as? Int) ?? 0
                let result = try await Transcriber.transcribeWithWordTimestamps(audioFile: chunkURL)
                successChunks += 1
                let chunkSentences = splitTextToSentences(
                    result.text, chunkOffsetSec: offset, chunkDurSec: chunkActualSec
                )
                logger.info("chunk \(chunkIdx + 1)/\(totalChunks): off=\(offset, format: .fixed(precision: 1))s bytes=\(bytes) chars=\(result.text.count) sentences=\(chunkSentences.count)")
                allSentences.append(contentsOf: chunkSentences)
            } catch {
                lastErr = "\(error.localizedDescription)"
                logger.warning("chunk \(chunkIdx + 1)/\(totalChunks) failed: \(error.localizedDescription, privacy: .public)")
            }
            try? FileManager.default.removeItem(at: chunkURL)
            offset += chunkSec
            chunkIdx += 1
        }

        logger.info("transcribe done: \(successChunks)/\(totalChunks) chunks ok, \(allSentences.count) sentences total")

        if allSentences.isEmpty {
            throw Failure.allChunksFailed(lastErr.isEmpty ? "no sentences" : lastErr)
        }

        return allSentences.enumerated().map { idx, s -> EnglishSegment in
            EnglishSegment(
                id: 0, audioID: 0, idx: idx,
                startMs: Int64(s.startSec * 1000),
                endMs: Int64(s.endSec * 1000),
                text: s.text
            )
        }
    }

    // MARK: - 内部类型

    private struct Sentence {
        let text: String
        let startSec: Double
        let endSec: Double
    }

    /// 段内拆句 + 字符数比例估算 timestamp。
    /// chunkDurSec 段总秒数, chunkOffsetSec 段在整段音频中的起点秒。
    /// 12s 段内通常 1-3 句, 误差 < 1s。
    private static func splitTextToSentences(
        _ text: String, chunkOffsetSec: Double, chunkDurSec: Double
    ) -> [Sentence] {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return [] }
        let parts = splitOnSentenceTerminators(trimmed)
        guard !parts.isEmpty else { return [] }
        let totalChars = parts.reduce(0) { $0 + $1.count }
        guard totalChars > 0 else { return [] }
        var charsBefore = 0
        var out: [Sentence] = []
        for p in parts {
            let startFrac = Double(charsBefore) / Double(totalChars)
            charsBefore += p.count
            let endFrac = Double(charsBefore) / Double(totalChars)
            out.append(Sentence(
                text: p,
                startSec: chunkOffsetSec + startFrac * chunkDurSec,
                endSec: chunkOffsetSec + endFrac * chunkDurSec
            ))
        }
        return out
    }

    /// 在 [.!?] 后跟空白处切句子, 保留标点。无标点时返回 [text] 整段。
    private static func splitOnSentenceTerminators(_ text: String) -> [String] {
        let pattern = "(?<=[.!?])\\s+"
        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
            return [text]
        }
        let nsText = text as NSString
        var lastIdx = 0
        var out: [String] = []
        regex.enumerateMatches(in: text, options: [], range: NSRange(location: 0, length: nsText.length)) { match, _, _ in
            guard let m = match else { return }
            let endOfSentence = m.range.location
            let s = nsText.substring(with: NSRange(location: lastIdx, length: endOfSentence - lastIdx))
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !s.isEmpty { out.append(s) }
            lastIdx = m.range.location + m.range.length
        }
        if lastIdx < nsText.length {
            let last = nsText.substring(from: lastIdx).trimmingCharacters(in: .whitespacesAndNewlines)
            if !last.isEmpty { out.append(last) }
        }
        return out.isEmpty ? [text] : out
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
