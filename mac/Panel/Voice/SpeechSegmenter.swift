import AVFoundation
import Foundation
import os
import Speech

/// 把本地音频文件转写为句子级 EnglishSegment 数组。
///
/// 实现:
///   - Apple Speech Framework 的 `SFSpeechURLRecognitionRequest` 有 **1 分钟硬限制**
///     (任何长于 60s 的输入直接返回 "No speech detected")。
///   - 故先用 `AVAssetExportSession` 把整段切成 ≤50s/段 (.m4a),串行 ASR,
///     合并 word timestamps (每段 timestamp + offset)。
///   - 再走句子边界算法 + 缩写合并 pass。
///
/// 优先 on-device (隐私 + 离线)。on-device 需用户系统已下载 en-US 增强听写模型
/// (System Settings → Siri 与 Spotlight → 听写)。
///
/// 在 Console.app 看进度:`log stream --predicate 'subsystem == "com.kinesis.panel" AND category == "speech"'`
enum SpeechSegmenter {

    private static let logger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// 每段 ASR 最长时长 (留 10s 余地避免触 60s 限制)。
    static let chunkSec: Double = 50.0

    enum Failure: Error, LocalizedError {
        case notAuthorized
        case recognizerUnavailable
        case noOnDeviceModel
        case audioLoadFailed(String)
        case allChunksFailed(String)

        var errorDescription: String? {
            switch self {
            case .notAuthorized: "needs_speech_authorization"
            case .recognizerUnavailable: "recognizer_unavailable"
            case .noOnDeviceModel: "needs_on_device_model"
            case .audioLoadFailed(let s): "audio_load_failed: \(s)"
            case .allChunksFailed(let s): "all_chunks_failed: \(s)"
            }
        }
    }

    /// 转写本地音频文件 → 句子级 EnglishSegment 数组 (id/audioID 都为 0,由调用方填回)。
    static func transcribe(url: URL) async throws -> [EnglishSegment] {
        guard SFSpeechRecognizer.authorizationStatus() == .authorized else {
            logger.error("not authorized")
            throw Failure.notAuthorized
        }
        guard let recognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US")),
              recognizer.isAvailable else {
            logger.error("recognizer unavailable")
            throw Failure.recognizerUnavailable
        }
        guard recognizer.supportsOnDeviceRecognition else {
            logger.error("on-device model not available")
            throw Failure.noOnDeviceModel
        }

        let asset = AVURLAsset(url: url)
        let totalSec: Double
        do {
            totalSec = try await asset.load(.duration).seconds
        } catch {
            logger.error("load duration failed: \(error.localizedDescription)")
            throw Failure.audioLoadFailed(error.localizedDescription)
        }
        guard totalSec.isFinite, totalSec > 0 else {
            throw Failure.audioLoadFailed("invalid duration: \(totalSec)")
        }

        let totalChunks = Int(ceil(totalSec / chunkSec))
        logger.info("transcribe start: \(url.lastPathComponent, privacy: .public), totalSec=\(totalSec), chunks=\(totalChunks)")

        var allWords: [TimedWord] = []
        var lastErr: String = ""
        var successChunks = 0
        var offset: Double = 0
        var chunkIdx = 0

        while offset < totalSec {
            let chunkActualSec = min(chunkSec, totalSec - offset)
            let chunkURL = URL(fileURLWithPath: NSTemporaryDirectory())
                .appendingPathComponent("panel-asr-\(UUID().uuidString).m4a")
            do {
                try await exportChunk(asset: asset, startSec: offset, durationSec: chunkActualSec, to: chunkURL)
                let chunkWords = try await transcribeOneChunk(url: chunkURL, recognizer: recognizer)
                successChunks += 1
                logger.info("chunk \(chunkIdx + 1)/\(totalChunks): off=\(offset, format: .fixed(precision: 1))s dur=\(chunkActualSec, format: .fixed(precision: 1))s words=\(chunkWords.count)")
                for w in chunkWords {
                    allWords.append(TimedWord(
                        text: w.substring,
                        timestamp: w.timestamp + offset,
                        duration: w.duration
                    ))
                }
            } catch {
                lastErr = "\(error.localizedDescription)"
                logger.warning("chunk \(chunkIdx + 1)/\(totalChunks) failed: \(error.localizedDescription, privacy: .public)")
            }
            try? FileManager.default.removeItem(at: chunkURL)
            offset += chunkSec
            chunkIdx += 1
        }

        logger.info("transcribe done: \(successChunks)/\(totalChunks) chunks ok, words=\(allWords.count)")

        if allWords.isEmpty {
            throw Failure.allChunksFailed(lastErr.isEmpty ? "no transcription" : lastErr)
        }

        let raw = splitToSentences(allWords)
        let merged = mergeAbbreviationSplits(raw)
        return merged.enumerated().map { idx, group -> EnglishSegment in
            let first = group.first!
            let last = group.last!
            let text = group.map(\.text).joined(separator: " ")
            return EnglishSegment(
                id: 0, audioID: 0, idx: idx,
                startMs: Int64(first.timestamp * 1000),
                endMs: Int64((last.timestamp + last.duration) * 1000),
                text: text
            )
        }
    }

    // MARK: - 内部类型 / 切片 / ASR 单段

    private struct TimedWord {
        let text: String
        let timestamp: Double  // 秒,整段音频内的绝对位置
        let duration: Double
    }

    /// 用 AVAssetExportSession 把 [startSec, startSec+durationSec) 段导出为 .m4a。
    private static func exportChunk(
        asset: AVURLAsset, startSec: Double, durationSec: Double, to outputURL: URL
    ) async throws {
        guard let exporter = AVAssetExportSession(asset: asset, presetName: AVAssetExportPresetAppleM4A) else {
            throw Failure.audioLoadFailed("create AVAssetExportSession failed")
        }
        exporter.outputFileType = .m4a
        exporter.outputURL = outputURL
        exporter.timeRange = CMTimeRange(
            start: CMTime(seconds: startSec, preferredTimescale: 1000),
            duration: CMTime(seconds: durationSec, preferredTimescale: 1000)
        )
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

    private static func transcribeOneChunk(url: URL, recognizer: SFSpeechRecognizer) async throws -> [SFTranscriptionSegment] {
        let request = SFSpeechURLRecognitionRequest(url: url)
        request.requiresOnDeviceRecognition = true
        request.shouldReportPartialResults = false
        return try await withCheckedThrowingContinuation { cont in
            var resumed = false
            recognizer.recognitionTask(with: request) { result, error in
                guard !resumed else { return }
                if let error {
                    resumed = true
                    cont.resume(throwing: error)
                    return
                }
                guard let result, result.isFinal else { return }
                resumed = true
                cont.resume(returning: result.bestTranscription.segments)
            }
        }
    }

    // MARK: - 句子边界算法

    /// 切句规则: 上词以 .!? 结尾 OR 下词与当前词的 gap > 0.6s OR 累计 ≥12 词且 gap > 0.3s 强切。
    private static func splitToSentences(_ words: [TimedWord]) -> [[TimedWord]] {
        var out: [[TimedWord]] = []
        var cur: [TimedWord] = []
        for (i, w) in words.enumerated() {
            cur.append(w)
            let isLast = i == words.count - 1
            let endsWithPunct = w.text.last.map { ".!?".contains($0) } ?? false
            let nextGap: Double
            if isLast {
                nextGap = 0
            } else {
                let next = words[i + 1]
                nextGap = next.timestamp - (w.timestamp + w.duration)
            }
            let strongCut = cur.count >= 12 && nextGap > 0.3
            if isLast || endsWithPunct || nextGap > 0.6 || strongCut {
                out.append(cur)
                cur = []
            }
        }
        if !cur.isEmpty { out.append(cur) }
        return out
    }

    /// 合并缩写跨切 (U.S. / Dr. + 下句首词小写) — 防止误切句子。
    private static func mergeAbbreviationSplits(_ groups: [[TimedWord]]) -> [[TimedWord]] {
        var out: [[TimedWord]] = []
        for g in groups {
            if let prev = out.last, let prevLast = prev.last,
               looksLikeAbbreviation(prevLast.text),
               let firstWord = g.first, let firstChar = firstWord.text.first,
               firstChar.isLowercase {
                out.removeLast()
                out.append(prev + g)
            } else {
                out.append(g)
            }
        }
        return out
    }

    private static func looksLikeAbbreviation(_ s: String) -> Bool {
        guard s.hasSuffix("."), s.count <= 4 else { return false }
        let body = s.dropLast()
        return body.allSatisfy { $0.isUppercase || $0 == "." || $0.isLetter }
    }
}
