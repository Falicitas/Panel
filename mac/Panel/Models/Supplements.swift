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
    /// "all" (默认: 所有 window 都勾才算 item 当日完成) | "any" (任一 window 勾上即算)。
    /// 老 backend 不返该字段时默认 "all", 行为不变。
    var completionMode: String

    enum CodingKeys: String, CodingKey {
        case id, name, type, windows, notes, dose, enabled
        case pausedUntil = "paused_until"
        case completionMode = "completion_mode"
    }

    init(
        id: Int64, name: String, type: String, windows: [SupplementWindow],
        notes: String, dose: String, enabled: Bool, pausedUntil: Int64?,
        completionMode: String = "all"
    ) {
        self.id = id
        self.name = name
        self.type = type
        self.windows = windows
        self.notes = notes
        self.dose = dose
        self.enabled = enabled
        self.pausedUntil = pausedUntil
        self.completionMode = completionMode
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try c.decode(Int64.self, forKey: .id)
        self.name = try c.decode(String.self, forKey: .name)
        self.type = try c.decode(String.self, forKey: .type)
        self.windows = try c.decode([SupplementWindow].self, forKey: .windows)
        self.notes = try c.decode(String.self, forKey: .notes)
        self.dose = try c.decode(String.self, forKey: .dose)
        self.enabled = try c.decode(Bool.self, forKey: .enabled)
        self.pausedUntil = try c.decodeIfPresent(Int64.self, forKey: .pausedUntil)
        self.completionMode = try c.decodeIfPresent(String.self, forKey: .completionMode) ?? "all"
    }
}

struct SupplementWindow: Codable, Hashable {
    var start: String
    var end: String
}

/// GET /api/supplements/today 的 windowStatus (per-window 粒度)
struct SupplementWindowStatus: Codable, Hashable {
    let start: String
    let end: String
    let startHM: String     // 解析后 HH:MM
    let endHM: String
    let status: String      // "passed" | "active" | "upcoming" | "unknown"
    let doneToday: Bool     // 该 window 当日是否已勾
    let requiresMeal: String  // "breakfast"/"lunch"/"dinner" — 表达式引用了未触发的餐;前端隐藏

    enum CodingKeys: String, CodingKey {
        case start, end, status
        case startHM = "start_hm"
        case endHM = "end_hm"
        case doneToday = "done_today"
        case requiresMeal = "requires_meal"
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
    let completionMode: String  // "all" | "any" — UI 提示用
    let windows: [SupplementWindowStatus]
    let hasActive: Bool

    enum CodingKeys: String, CodingKey {
        case id, name, type, dose, notes, enabled, paused, windows
        case pausedUntil = "paused_until"
        case doneToday = "done_today"
        case completionMode = "completion_mode"
        case hasActive = "has_active"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try c.decode(Int64.self, forKey: .id)
        self.name = try c.decode(String.self, forKey: .name)
        self.type = try c.decode(String.self, forKey: .type)
        self.dose = try c.decode(String.self, forKey: .dose)
        self.notes = try c.decode(String.self, forKey: .notes)
        self.enabled = try c.decode(Bool.self, forKey: .enabled)
        self.pausedUntil = try c.decodeIfPresent(Int64.self, forKey: .pausedUntil)
        self.paused = try c.decode(Bool.self, forKey: .paused)
        self.doneToday = try c.decode(Bool.self, forKey: .doneToday)
        self.completionMode = try c.decodeIfPresent(String.self, forKey: .completionMode) ?? "all"
        self.windows = try c.decode([SupplementWindowStatus].self, forKey: .windows)
        self.hasActive = try c.decode(Bool.self, forKey: .hasActive)
    }
}

struct SupplementToday: Codable, Hashable {
    let date: String
    let items: [SupplementTodayItem]
    let activeCount: Int    // 当前正在窗口内的项数 (enabled & 未暂停 & has_active)
    let totalActive: Int    // enabled & 未暂停 的总项数
    let totalDoses: Int     // 所有 enabled & 未暂停 项的 window 数之和
    let doneDoses: Int      // 上面里已勾过的 window 数

    enum CodingKeys: String, CodingKey {
        case date, items
        case activeCount = "active_count"
        case totalActive = "total_active"
        case totalDoses = "total_doses"
        case doneDoses = "done_doses"
    }
}

struct DailyState: Codable, Hashable {
    let date: String
    var mode: String         // "normal" | "focus" | "sleep" | "outside"
    var wakeTime: String     // "HH:MM" or ""
    var modeSetAt: Int64
    var breakfastAt: String  // "HH:MM" or "" — 用户主动触发的早餐时刻
    var lunchAt: String
    var dinnerAt: String
    // 三餐暂停截止 (unix 秒, nil = 未暂停)。跨日保留 (跟 supplement.pausedUntil 同语义)。
    // backend 从 meal_settings 表 join 进 DailyState 返回, mac UI 单点取数据渲染暂停状态。
    var breakfastPausedUntil: Int64?
    var lunchPausedUntil: Int64?
    var dinnerPausedUntil: Int64?

    enum CodingKeys: String, CodingKey {
        case date, mode
        case wakeTime = "wake_time"
        case modeSetAt = "mode_set_at"
        case breakfastAt = "breakfast_at"
        case lunchAt = "lunch_at"
        case dinnerAt = "dinner_at"
        case breakfastPausedUntil = "breakfast_paused_until"
        case lunchPausedUntil = "lunch_paused_until"
        case dinnerPausedUntil = "dinner_paused_until"
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
