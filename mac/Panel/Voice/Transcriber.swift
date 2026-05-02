import Foundation

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

/// OpenRouter chat/completions 走 input_audio,模型 openai/gpt-4o-transcribe。
enum Transcriber {
    static let endpoint = URL(string: "https://openrouter.ai/api/v1/chat/completions")!
    static let model = "openai/gpt-4o-transcribe"
    static let prompt = "Transcribe the audio."
    static let timeoutSec: TimeInterval = 30

    static func transcribe(audioFile: URL) async throws -> String {
        guard let key = KeyStore.get(), !key.isEmpty else { throw TranscribeError.noKey }
        let data = try Data(contentsOf: audioFile)
        let b64 = data.base64EncodedString()
        let body: [String: Any] = [
            "model": model,
            "messages": [[
                "role": "user",
                "content": [
                    ["type": "text", "text": prompt],
                    ["type": "input_audio", "input_audio": ["data": b64, "format": "m4a"]]
                ]
            ]]
        ]
        var req = URLRequest(url: endpoint, timeoutInterval: timeoutSec)
        req.httpMethod = "POST"
        req.setValue("Bearer \(key)", forHTTPHeaderField: "Authorization")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (respData, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw TranscribeError.http(0, "no response")
        }
        if http.statusCode != 200 {
            let text = String(data: respData, encoding: .utf8) ?? "<binary>"
            throw TranscribeError.http(http.statusCode, text)
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

    /// 仅验证 key 可调通 (Settings 测试连接按钮用)。
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
