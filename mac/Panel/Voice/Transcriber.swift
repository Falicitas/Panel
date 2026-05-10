import Foundation
import os

enum TranscribeError: Error, LocalizedError {
    case noKey
    case http(Int, String)
    case decode

    var errorDescription: String? {
        switch self {
        case .noKey: return "未设置 OpenRouter key"
        case .http(let code, let body): return "HTTP \(code): \(String(body.prefix(200)))"
        case .decode: return "解析响应失败"
        }
    }
}

/// whisper verbose_json 返回的 word-level 时间戳条目。
struct TranscribedWord: Sendable {
    let text: String
    let startSec: Double
    let endSec: Double
}

/// 两条 endpoint:
/// - 专用 STT (/api/v1/audio/transcriptions): openai/gpt-4o-transcribe / openai/whisper-* 等
///   走这条,JSON body 含 input_audio (data + format),响应 { text }
/// - 通用 chat/completions (/api/v1/chat/completions): gemini / gpt-4o-audio-preview 等
///   走这条,messages 数组里嵌 input_audio,响应 choices[0].message.content
///
/// 根据 model name 自动判断,用户在 Settings 改 model 不用关心 endpoint。
enum Transcriber {
    static let chatEndpoint = URL(string: "https://openrouter.ai/api/v1/chat/completions")!
    static let sttEndpoint  = URL(string: "https://openrouter.ai/api/v1/audio/transcriptions")!
    /// Groq Whisper Large V3 直连 endpoint (兼容 OpenAI Whisper API), 词级 timestamp 真实有效。
    /// OpenRouter 的 STT API 把 timestamp_granularities 字段 strip 掉了, 必须直连 Groq 才能拿。
    static let groqSTTEndpoint = URL(string: "https://api.groq.com/openai/v1/audio/transcriptions")!
    /// chat/completions 路径下用,加"简体中文"提示因为 Gemini Flash 偶有繁体倾向。
    static let chatPrompt = "Transcribe the audio verbatim. For Chinese content, output Simplified Chinese (简体中文) only."
    /// STT 专用 endpoint (whisper / gpt-4o-transcribe) 的 prompt 字段语义是"示例文本",
    /// 模型会模仿其风格(简繁、专有名词写法、标点)。短中文音频常因上下文不足切到繁体,
    /// 给一段简体示例即可稳定引导。中英混合场景英文部分不受中文 prompt 影响。
    static let sttPromptZh = "以下是普通话录音,中英文混合。中文部分使用简体中文,例如这样的文字。"
    static let timeoutSec: TimeInterval = 120

    static func transcribe(audioFile: URL) async throws -> String {
        guard let key = KeyStore.get(), !key.isEmpty else { throw TranscribeError.noKey }
        let model = KeyStore.getModel()
        let data = try Data(contentsOf: audioFile)
        let b64 = data.base64EncodedString()
        if isSTTModel(model) {
            return try await viaSTT(model: model, audioBase64: b64, key: key)
        } else {
            return try await viaChat(model: model, audioBase64: b64, key: key)
        }
    }

    private static let sttLogger = Logger(subsystem: "com.kinesis.panel", category: "speech")

    /// OpenRouter STT 转写音频, 优先返回 word-level timestamps。
    /// 用 model = openai/gpt-4o-transcribe (F12 已验证可用)。
    /// 试探性带上 response_format / timestamp_granularities — 透传则有 words, 不透传则空。
    /// 返回 (text, words);words 可能为空,调用方需要 fallback。
    /// 注意: OpenRouter STT 只接 JSON + base64 (multipart 会被当 JSON 解析报错), 故走 F12 同款 JSON 路径。
    static func transcribeWithWordTimestamps(audioFile: URL) async throws -> (text: String, words: [TranscribedWord]) {
        guard let key = KeyStore.get(), !key.isEmpty else { throw TranscribeError.noKey }
        let audioData = try Data(contentsOf: audioFile)
        let b64 = audioData.base64EncodedString()
        // format 必须跟实际 audio 编码匹配, 否则 OpenAI 解码失败返 400。
        // 写死 "m4a" 时, 整段 mp3 (例如 manual-1.mp3) 会被 server 拒。
        let format = formatHint(forFile: audioFile)
        let body: [String: Any] = [
            "model": "openai/gpt-4o-transcribe",
            "input_audio": ["data": b64, "format": format],
            "response_format": "verbose_json",
            "timestamp_granularities": ["word"],
            "prompt": sttPromptZh
        ]
        var req = URLRequest(url: sttEndpoint, timeoutInterval: timeoutSec)
        req.httpMethod = "POST"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (respData, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw TranscribeError.http(0, "no response") }
        let snippet = String(data: respData, encoding: .utf8)?.prefix(300).description ?? "<non-utf8>"
        sttLogger.info("STT http=\(http.statusCode) body[0..300]=\(snippet, privacy: .public)")
        if http.statusCode != 200 {
            throw TranscribeError.http(http.statusCode, String(data: respData, encoding: .utf8) ?? "<binary>")
        }
        guard let json = try JSONSerialization.jsonObject(with: respData) as? [String: Any] else {
            throw TranscribeError.decode
        }
        let text = (json["text"] as? String) ?? ""
        let wordsRaw = (json["words"] as? [[String: Any]]) ?? []
        let words: [TranscribedWord] = wordsRaw.compactMap { w in
            guard let t = w["word"] as? String,
                  let s = readDouble(w["start"]),
                  let e = readDouble(w["end"])
            else { return nil }
            return TranscribedWord(text: t, startSec: s, endSec: e)
        }
        return (text: text.trimmingCharacters(in: .whitespacesAndNewlines), words: words)
    }

    /// 从文件扩展名推断 OpenAI/OpenRouter STT 接受的 format hint。
    /// 已知支持: mp3, mp4, m4a, wav, webm, flac, ogg。未识别 fallback 到 mp3。
    private static func formatHint(forFile url: URL) -> String {
        switch url.pathExtension.lowercased() {
        case "mp3":  return "mp3"
        case "m4a":  return "m4a"
        case "mp4":  return "mp4"
        case "wav":  return "wav"
        case "webm": return "webm"
        case "flac": return "flac"
        case "ogg":  return "ogg"
        default:     return "mp3"
        }
    }

    private static func readDouble(_ any: Any?) -> Double? {
        if let d = any as? Double { return d }
        if let n = any as? NSNumber { return n.doubleValue }
        if let i = any as? Int { return Double(i) }
        return nil
    }

    /// 模型名含 transcribe / whisper → 专用 STT endpoint。
    private static func isSTTModel(_ model: String) -> Bool {
        let m = model.lowercased()
        return m.contains("transcribe") || m.contains("whisper")
    }

    // MARK: - STT 专用 endpoint

    private static func viaSTT(model: String, audioBase64: String, key: String) async throws -> String {
        let body: [String: Any] = [
            "model": model,
            "input_audio": ["data": audioBase64, "format": "m4a"],
            "prompt": sttPromptZh
        ]
        var req = URLRequest(url: sttEndpoint, timeoutInterval: timeoutSec)
        req.httpMethod = "POST"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (respData, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw TranscribeError.http(0, "no response") }
        if http.statusCode != 200 {
            throw TranscribeError.http(http.statusCode, String(data: respData, encoding: .utf8) ?? "<binary>")
        }
        guard let json = try JSONSerialization.jsonObject(with: respData) as? [String: Any],
              let text = json["text"] as? String
        else {
            throw TranscribeError.decode
        }
        return text.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    // MARK: - 通用 chat/completions endpoint

    private static func viaChat(model: String, audioBase64: String, key: String) async throws -> String {
        let body: [String: Any] = [
            "model": model,
            "messages": [[
                "role": "user",
                "content": [
                    ["type": "text", "text": chatPrompt],
                    ["type": "input_audio", "input_audio": ["data": audioBase64, "format": "m4a"]]
                ]
            ]]
        ]
        var req = URLRequest(url: chatEndpoint, timeoutInterval: timeoutSec)
        req.httpMethod = "POST"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (respData, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw TranscribeError.http(0, "no response") }
        if http.statusCode != 200 {
            throw TranscribeError.http(http.statusCode, String(data: respData, encoding: .utf8) ?? "<binary>")
        }
        guard let json = try JSONSerialization.jsonObject(with: respData) as? [String: Any],
              let choices = json["choices"] as? [[String: Any]],
              let first = choices.first,
              let message = first["message"] as? [String: Any],
              let content = message["content"] as? String
        else {
            throw TranscribeError.decode
        }
        return content.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    // MARK: - Groq 直连 (英文精听 ASR, 词级 timestamp)

    /// 直连 Groq Whisper Large V3, multipart/form-data, response_format=verbose_json + word timestamps。
    /// 走这条而非 OpenRouter 因为 OpenRouter STT 把 timestamp_granularities 字段 strip 掉了 (实证)。
    /// Groq 100% 兼容 OpenAI Whisper API spec, 30x+ 实时速度, ~$0.018/11min。
    /// 上限: 单文件 ≤ 25MB (~30min @128kbps mp3)。
    static func transcribeViaGroq(audioFile: URL) async throws -> (text: String, words: [TranscribedWord]) {
        guard let key = KeyStore.getGroq(), !key.isEmpty else { throw TranscribeError.noKey }
        let audioData = try Data(contentsOf: audioFile)
        let boundary = "----panel-\(UUID().uuidString)"
        let format = formatHint(forFile: audioFile)
        let filename = "audio.\(format)"
        let mime = mimeForFormat(format)

        var body = Data()
        func appendField(_ name: String, _ value: String) {
            body.append("--\(boundary)\r\n".data(using: .utf8)!)
            body.append("Content-Disposition: form-data; name=\"\(name)\"\r\n\r\n".data(using: .utf8)!)
            body.append(value.data(using: .utf8)!)
            body.append("\r\n".data(using: .utf8)!)
        }
        appendField("model", "whisper-large-v3")
        appendField("response_format", "verbose_json")
        appendField("timestamp_granularities[]", "word")
        appendField("language", "en")

        body.append("--\(boundary)\r\n".data(using: .utf8)!)
        body.append("Content-Disposition: form-data; name=\"file\"; filename=\"\(filename)\"\r\n".data(using: .utf8)!)
        body.append("Content-Type: \(mime)\r\n\r\n".data(using: .utf8)!)
        body.append(audioData)
        body.append("\r\n".data(using: .utf8)!)
        body.append("--\(boundary)--\r\n".data(using: .utf8)!)

        var req = URLRequest(url: groqSTTEndpoint, timeoutInterval: timeoutSec)
        req.httpMethod = "POST"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        req.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        req.httpBody = body

        let (respData, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw TranscribeError.http(0, "no response") }
        let snippet = String(data: respData, encoding: .utf8)?.prefix(300).description ?? "<non-utf8>"
        sttLogger.info("Groq STT http=\(http.statusCode) bytes=\(respData.count) body[0..300]=\(snippet, privacy: .public)")
        if http.statusCode != 200 {
            throw TranscribeError.http(http.statusCode, String(data: respData, encoding: .utf8) ?? "<binary>")
        }
        guard let json = try JSONSerialization.jsonObject(with: respData) as? [String: Any] else {
            throw TranscribeError.decode
        }
        let text = (json["text"] as? String) ?? ""
        let wordsRaw = (json["words"] as? [[String: Any]]) ?? []
        let words: [TranscribedWord] = wordsRaw.compactMap { w in
            guard let t = w["word"] as? String,
                  let s = readDouble(w["start"]),
                  let e = readDouble(w["end"])
            else { return nil }
            return TranscribedWord(text: t, startSec: s, endSec: e)
        }
        return (text: text.trimmingCharacters(in: .whitespacesAndNewlines), words: words)
    }

    private static func mimeForFormat(_ f: String) -> String {
        switch f {
        case "mp3":  return "audio/mpeg"
        case "m4a":  return "audio/mp4"
        case "mp4":  return "audio/mp4"
        case "wav":  return "audio/wav"
        case "webm": return "audio/webm"
        case "flac": return "audio/flac"
        case "ogg":  return "audio/ogg"
        default:     return "application/octet-stream"
        }
    }

    // MARK: - validateKey

    static func validateKey(_ key: String) async -> Bool {
        var req = URLRequest(url: URL(string: "https://openrouter.ai/api/v1/auth/key")!, timeoutInterval: 10)
        req.httpMethod = "GET"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        do {
            let (_, resp) = try await URLSession.shared.data(for: req)
            return (resp as? HTTPURLResponse)?.statusCode == 200
        } catch {
            return false
        }
    }
}
