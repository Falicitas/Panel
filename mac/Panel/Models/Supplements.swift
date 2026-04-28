import Foundation

struct Supplement: Codable, Identifiable, Hashable {
    let id: Int64
    var name: String
    var type: String  // "food" | "supplement"
    var windows: [SupplementWindow]
    var notes: String
}

struct SupplementWindow: Codable, Hashable {
    var start: String
    var end: String
}

struct DailyState: Codable, Hashable {
    let date: String
    var mode: String       // "normal" | "focus" | "sleep" | "outside"
    var wakeTime: String   // "HH:MM" or ""
    var modeSetAt: Int64

    enum CodingKeys: String, CodingKey {
        case date, mode
        case wakeTime = "wake_time"
        case modeSetAt = "mode_set_at"
    }
}

struct StateSettings: Codable, Hashable {
    var focusStart: String
    var sleepStart: String

    enum CodingKeys: String, CodingKey {
        case focusStart = "focus_start"
        case sleepStart = "sleep_start"
    }
}

// Foundation 已有 Notification,这里命名 PanelNotification 避歧义
struct PanelNotification: Codable, Identifiable, Hashable {
    let id: Int64
    let source: String     // "supplement" | "finance" | "records" | "manual"
    let sourceRef: String
    let title: String
    let body: String
    let level: String      // "info" | "warn" | "alert"
    let createdAt: Int64
    let dismissedAt: Int64?

    enum CodingKeys: String, CodingKey {
        case id, source, title, body, level
        case sourceRef = "source_ref"
        case createdAt = "created_at"
        case dismissedAt = "dismissed_at"
    }
}
