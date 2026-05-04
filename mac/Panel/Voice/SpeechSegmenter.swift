import AVFoundation
import Foundation
import os

/// 把本地音频文件转写为句子级 EnglishSegment 数组。
///
/// 实现 (M5 v10 — Groq Whisper Large V3 直连, 词级 timestamp):
///   - Transcriber.transcribeViaGroq 一次整段 ASR 拿 (text, words[]) — 词级 timestamp 100% 精确
///   - 词级 → 句子级: (a) 标点 .!? (b) gap > 0.6s (c) 累计 ≥ 12 词强切, 缩写不切
///   - N 句合 1 段 (sentencesPerSegment, 让段更长更连贯, 适合精听)
///
/// 优点 vs 之前所有方案:
///   - Groq 30x+ 实时, 11min audio < 20s, 无切片无 fuzzy
///   - 词级 timestamp 真实精确 (Groq 100% 兼容 OpenAI Whisper API)
///   - 文本质量高 (Whisper Large V3 是当前 SOTA 开源 ASR)
///   - $0.111/hour ≈ $0.018/11min
/// 限制:
///   - 单文件 ≤ 25MB (~30min @128kbps mp3) — 30min VOA Phase 1 任务
///   - 用户需 Settings 配 GROQ_API_KEY
///
/// Console.app 看进度:
///   `log stream --predicate 'subsystem == "com.kinesis.panel" AND category == "speech"'`
enum SpeechSegmenter {

    private static let logger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// 句子级 segment 合并: N 句合 1 段, 让段更长更连贯 (精听点 ▶ 听完整 N 句)。
    /// 3 适合 NPR 这种快语速多人对话 (单句平均 5-10 词, 3 句合段 ~15-30 词 5-10s 一段)。
    static let sentencesPerSegment: Int = 3

    /// 词间 gap > 此阈值 → 强制句子边界 (即使没有 .!? 标点)。
    static let sentenceGapSec: Double = 0.6

    /// 累计 ≥ 此 token 数仍未遇到 .!? 或 gap → 强切, 防一句巨长 (safety net)。
    /// 40 词放宽到 NPR 长句最大值的 1.5x, 让标点 / gap 主导切句 (实测 NPR 最长 ~25 词)。
    /// 极少触发 hard cap, 避免在词中切句导致段末缺 .!? 标点。
    static let maxTokensPerSentence: Int = 40

    enum Failure: Error, LocalizedError {
        case noKey
        case audioLoadFailed(String)
        case asrFailed(String)
        case noWords
        case noSentences

        var errorDescription: String? {
            switch self {
            case .noKey: "未设置 Groq key (Settings 加 voice.groq_api_key)"
            case .audioLoadFailed(let s): "audio_load_failed: \(s)"
            case .asrFailed(let s): "asr_failed: \(s)"
            case .noWords: "no_words_returned"
            case .noSentences: "no_sentences"
            }
        }
    }

    static func transcribe(url: URL) async throws -> [EnglishSegment] {
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

        logger.info("groq-asr start: \(url.lastPathComponent, privacy: .public) totalSec=\(totalSec, format: .fixed(precision: 1))")

        // 步骤 1: Groq 整段 ASR (词级 timestamp)
        let result: (text: String, words: [TranscribedWord])
        do {
            result = try await Transcriber.transcribeViaGroq(audioFile: url)
        } catch TranscribeError.noKey {
            throw Failure.noKey
        } catch {
            throw Failure.asrFailed(error.localizedDescription)
        }
        guard !result.words.isEmpty else { throw Failure.noWords }
        logger.info("groq-asr done: words=\(result.words.count) text.length=\(result.text.count)")

        // 步骤 2: 词级 → 句子级
        let sentences = splitWordsToSentences(result.words)
        guard !sentences.isEmpty else { throw Failure.noSentences }
        logger.info("phase split: sentences=\(sentences.count)")

        // 步骤 3: N 句合 1 段
        let merged = mergeSentenceGroups(sentences, perGroup: sentencesPerSegment)
        logger.info("phase merge: segments=\(merged.count) per_group=\(sentencesPerSegment)")

        // 步骤 4: 转 EnglishSegment
        return merged.enumerated().map { idx, st in
            EnglishSegment(
                id: 0, audioID: 0, idx: idx,
                startMs: Int64(st.startSec * 1000),
                endMs: Int64(st.endSec * 1000),
                text: st.text
            )
        }
    }

    // MARK: - 拆句 (词级 → 句子级)

    private struct SentenceTime {
        let startSec: Double
        let endSec: Double
        let text: String
    }

    /// 全局 TranscribedWord[] → 句子级 SentenceTime[]
    /// 句子边界规则 (满足任一即切):
    ///   (a) 上一词的 text 末字符 ∈ ".!?" (但缩写如 "U.S." / "Dr." 不切, 见 isAbbreviation)
    ///   (b) 下一词的 startSec - 上一词的 endSec > sentenceGapSec
    ///   (c) 当前累计 ≥ maxTokensPerSentence 词强切
    private static func splitWordsToSentences(_ words: [TranscribedWord]) -> [SentenceTime] {
        var out: [SentenceTime] = []
        var buf: [TranscribedWord] = []

        func flush() {
            guard !buf.isEmpty else { return }
            let start = buf.first!.startSec
            let end = buf.last!.endSec
            let text = buf.map(\.text).joined(separator: " ")
            out.append(SentenceTime(startSec: start, endSec: end, text: text))
            buf.removeAll(keepingCapacity: true)
        }

        for (i, w) in words.enumerated() {
            buf.append(w)

            // 规则 (c): 强切
            if buf.count >= maxTokensPerSentence {
                flush()
                continue
            }

            // 看下一词决定要不要在 w 之后切
            guard i + 1 < words.count else {
                flush()
                continue
            }
            let next = words[i + 1]

            // 规则 (b): gap
            let gap = next.startSec - w.endSec
            let gapBreak = gap > sentenceGapSec

            // 规则 (a): 标点 (排除缩写)
            let endsPunct = endsWithSentencePunctuation(w.text) && !isAbbreviation(w.text)

            if endsPunct || gapBreak {
                flush()
            }
        }
        flush()
        return out
    }

    /// 末字符是 . ! ? (考虑可能跟随引号: ."  ?" )
    private static func endsWithSentencePunctuation(_ s: String) -> Bool {
        let trimmed = s.trimmingCharacters(in: CharacterSet(charactersIn: "\"'»"))
        guard let last = trimmed.last else { return false }
        return last == "." || last == "!" || last == "?"
    }

    /// 缩写白名单, 这些 word 末尾 . 不切句。
    private static func isAbbreviation(_ s: String) -> Bool {
        let abbrs: Set<String> = [
            "Mr.", "Mrs.", "Ms.", "Dr.", "St.", "Jr.", "Sr.", "Prof.",
            "U.S.", "U.K.", "U.N.", "E.U.", "U.S.A.",
            "etc.", "vs.", "e.g.", "i.e.", "a.m.", "p.m.",
            "No.", "Inc.", "Co.", "Ltd.", "Corp.",
            "Sept.", "Oct.", "Nov.", "Dec.", "Jan.", "Feb.",
        ]
        return abbrs.contains(s)
    }

    // MARK: - 句子合并

    private static func mergeSentenceGroups(_ ss: [SentenceTime], perGroup: Int) -> [SentenceTime] {
        guard perGroup > 1, ss.count > 1 else { return ss }
        var out: [SentenceTime] = []
        var i = 0
        while i < ss.count {
            let end = min(i + perGroup, ss.count)
            let group = Array(ss[i..<end])
            out.append(SentenceTime(
                startSec: group.first!.startSec,
                endSec: group.last!.endSec,
                text: group.map(\.text).joined(separator: " ")
            ))
            i = end
        }
        return out
    }
}
