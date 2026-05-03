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
    /// chat/completions 路径下用,加"简体中文"提示因为 Gemini Flash 偶有繁体倾向。
    /// STT 专用模型不需要 prompt (它就是干这个的)。
    static let chatPrompt = "Transcribe the audio verbatim. For Chinese content, output Simplified Chinese (简体中文) only."
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
        let body: [String: Any] = [
            "model": "openai/gpt-4o-transcribe",
            "input_audio": ["data": b64, "format": "m4a"],
            "response_format": "verbose_json",
            "timestamp_granularities": ["word"]
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
            "input_audio": ["data": audioBase64, "format": "m4a"]
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
