import Foundation

/// OpenRouter API key + 模型 持久化。明文存 UserDefaults。
/// 不走 Keychain — 避免每次访问触发"允许 Panel 访问钥匙串"对话框。
enum KeyStore {
    private static let keyForKey = "voice.openrouter_api_key"
    private static let keyForModel = "voice.openrouter_model"

    /// gemini-2.5-pro 实测中文不出繁体 (flash 仍有);贵约 4x 但单次 ~$0.0024,可接受。
    static let defaultModel = "google/gemini-2.5-pro"

    // MARK: - API Key

    static func get() -> String? {
        UserDefaults.standard.string(forKey: keyForKey)
    }

    @discardableResult
    static func set(_ value: String) -> Bool {
        UserDefaults.standard.set(value, forKey: keyForKey)
        return true
    }

    @discardableResult
    static func delete() -> Bool {
        UserDefaults.standard.removeObject(forKey: keyForKey)
        return true
    }

    static var hasKey: Bool {
        guard let v = get() else { return false }
        return !v.isEmpty
    }

    // MARK: - Model

    static func getModel() -> String {
        let v = UserDefaults.standard.string(forKey: keyForModel) ?? ""
        return v.isEmpty ? defaultModel : v
    }

    @discardableResult
    static func setModel(_ value: String) -> Bool {
        UserDefaults.standard.set(value, forKey: keyForModel)
        return true
    }

    // MARK: - Groq API Key (英文精听 ASR 专用, 直连 Groq Whisper Large V3 拿词级 timestamp)

    private static let keyForGroq = "voice.groq_api_key"

    static func getGroq() -> String? {
        UserDefaults.standard.string(forKey: keyForGroq)
    }

    @discardableResult
    static func setGroq(_ value: String) -> Bool {
        UserDefaults.standard.set(value, forKey: keyForGroq)
        return true
    }
}
