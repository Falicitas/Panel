import Foundation

/// OpenRouter API key + 模型 持久化。明文存 UserDefaults。
/// 不走 Keychain — 避免每次访问触发"允许 Panel 访问钥匙串"对话框。
enum KeyStore {
    private static let keyForKey = "voice.openrouter_api_key"
    private static let keyForModel = "voice.openrouter_model"

    static let defaultModel = "google/gemini-2.5-flash"

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
}
