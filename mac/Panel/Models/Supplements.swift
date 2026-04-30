import Foundation

struct Supplement: Codable, Identifiable, Hashable {
    let id: Int64
    var name: String
    var type: String  // "food" | "supplement"
    var windows: [SupplementWindow]
    var notes: String
    var dose: String        // "1 颗" / "5 滴" / "30g" 等,可空
    var enabled: Bool       // false = 临时停用,通知不弹,统计不计 expected
    var pausedUntil: Int64? // unix 秒,> now 时不弹通知

    enum CodingKeys: String, CodingKey {
        case id, name, type, windows, notes, dose, enabled
        case pausedUntil = "paused_until"
    }
}

struct SupplementWindow: Codable, Hashable {
    var start: String
    var end: String
}

/// GET /api/supplements/today 的 windowStatus
struct SupplementWindowStatus: Codable, Hashable {
    let start: String
    let end: String
    let startHM: String     // 解析后 HH:MM
    let endHM: String
    let status: String      // "passed" | "active" | "upcoming" | "unknown"

    enum CodingKeys: String, CodingKey {
        case start, end, status
        case startHM = "start_hm"
        case endHM = "end_hm"
    }
}

struct SupplementTodayItem: Codable, Identifiable, Hashable {
    let id: Int64
    let name: String
    let type: String
    let dose: String
    let notes: String
    let enabled: Bool
    let pausedUntil: Int64?
    let paused: Bool
    let doneToday: Bool
    let windows: [SupplementWindowStatus]
    let hasActive: Bool

    enum CodingKeys: String, CodingKey {
        case id, name, type, dose, notes, enabled, paused, windows
        case pausedUntil = "paused_until"
        case doneToday = "done_today"
        case hasActive = "has_active"
    }
}

struct SupplementToday: Codable, Hashable {
    let date: String
    let items: [SupplementTodayItem]
    let activeCount: Int    // 当前正在窗口内的项数 (enabled & 未暂停 & has_active)
    let doneCount: Int      // 当日已勾过的总数
    let totalActive: Int    // enabled & 未暂停 的总项数

    enum CodingKeys: String, CodingKey {
        case date, items
        case activeCount = "active_count"
        case doneCount = "done_count"
        case totalActive = "total_active"
    }
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
