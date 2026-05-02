import Foundation

/// OpenRouter API key 持久化。明文存 UserDefaults。
/// 不走 Keychain — 避免每次访问触发"允许 Panel 访问钥匙串"对话框。
enum KeyStore {
    private static let key = "voice.openrouter_api_key"

    static func get() -> String? {
        UserDefaults.standard.string(forKey: key)
    }

    @discardableResult
    static func set(_ value: String) -> Bool {
        UserDefaults.standard.set(value, forKey: key)
        return true
    }

    @discardableResult
    static func delete() -> Bool {
        UserDefaults.standard.removeObject(forKey: key)
        return true
    }

    static var hasKey: Bool {
        guard let v = get() else { return false }
        return !v.isEmpty
    }
}
