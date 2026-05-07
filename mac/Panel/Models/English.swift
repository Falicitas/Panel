import Foundation

/// 英文练习模块的全部 DTO。后端 JSON 字段为 snake_case, 前端 camelCase, 用 CodingKeys 映射。

struct EnglishAudio: Codable, Identifiable, Hashable, Sendable {
    let id: Int64
    let source: String
    let sourceRef: String
    var title: String
    var description: String
    var publishedAt: Int64
    var durationMs: Int64
    var audioURL: String
    var localPath: String
    var transcript: String
    var segmentsStatus: String  // pending|running|ready|failed
    var segmentsFailedReason: String
    var fetchedAt: Int64
    var lastPlayedAt: Int64?
    var archivedAt: Int64?
    var topic: String  // free-text: finance/tech/science/current_affairs/general/""

    enum CodingKeys: String, CodingKey {
        case id, source, title, description, transcript, topic
        case sourceRef = "source_ref"
        case publishedAt = "published_at"
        case durationMs = "duration_ms"
        case audioURL = "audio_url"
        case localPath = "local_path"
        case segmentsStatus = "segments_status"
        case segmentsFailedReason = "segments_failed_reason"
        case fetchedAt = "fetched_at"
        case lastPlayedAt = "last_played_at"
        case archivedAt = "archived_at"
    }

    /// 后端可能给空字符串当 NULL,后续 init 兼容(JSONDecoder 默认对 optional String 字段
    /// 缺省视为 nil 是 OK 的;localPath 是 String 非 optional 所以必有值,空字符串 = 未下载)
}

struct EnglishSegment: Codable, Identifiable, Hashable, Sendable {
    let id: Int64
    let audioID: Int64
    let idx: Int
    let startMs: Int64
    let endMs: Int64
    let text: String

    enum CodingKeys: String, CodingKey {
        case id, idx, text
        case audioID = "audio_id"
        case startMs = "start_ms"
        case endMs = "end_ms"
    }
}

struct EnglishWindow: Codable, Hashable, Sendable {
    var start: String
    var end: String
}

struct EnglishSettings: Codable, Hashable, Sendable {
    var windows: [EnglishWindow]
    var thresholdMin: Int
    var enabled: Bool

    enum CodingKeys: String, CodingKey {
        case windows, enabled
        case thresholdMin = "threshold_min"
    }
}

/// 后端 GET /api/english/today 响应内的 window 状态。
struct EnglishWindowStatus: Codable, Identifiable, Hashable, Sendable {
    let index: Int
    let startHM: String
    let endHM: String
    let status: String       // upcoming | active | passed | unknown
    let minutesDone: Double
    let reached: Bool
    let snoozeCount: Int

    var id: Int { index }

    enum CodingKeys: String, CodingKey {
        case index, status, reached
        case startHM = "start_hm"
        case endHM = "end_hm"
        case minutesDone = "minutes_done"
        case snoozeCount = "snooze_count"
    }
}

/// 后端 GET /api/english/today 响应。
struct EnglishToday: Codable, Hashable, Sendable {
    let date: String
    let windows: [EnglishWindowStatus]
    let todayDone: Bool
    let thresholdMin: Int
    let totalMinutes: Double
    let currentAudio: EnglishAudio?
    let segments: [EnglishSegment]
    let doneSegIdx: [Int]

    enum CodingKeys: String, CodingKey {
        case date, windows, segments
        case todayDone = "today_done"
        case thresholdMin = "threshold_min"
        case totalMinutes = "total_minutes"
        case currentAudio = "current_audio"
        case doneSegIdx = "done_seg_idx"
    }
}

/// 后端 GET /api/english/audios/{id} 响应。
struct EnglishAudioDetail: Codable, Hashable, Sendable {
    let audio: EnglishAudio
    let segments: [EnglishSegment]
}

/// 后端 GET /api/english/audios/candidate?topic=&exclude=&current= 响应。
/// audio == nil 表示候选已抽完 (UI 显示空状态)。
struct EnglishCandidateResp: Codable, Hashable, Sendable {
    let audio: EnglishAudio?
    let segments: [EnglishSegment]
    let descriptionPreview: String

    enum CodingKeys: String, CodingKey {
        case audio, segments
        case descriptionPreview = "description_preview"
    }
}

/// 后端 POST /api/english/audios/{id}/skip 请求 body。
struct EnglishSkipReq: Codable, Sendable {
    let topic: String
    let selectedAudioID: Int64

    enum CodingKeys: String, CodingKey {
        case topic
        case selectedAudioID = "selected_audio_id"
    }
}

/// 后端 POST /api/english/audios/{id}/skip 响应。
struct EnglishSkipResp: Codable, Hashable, Sendable {
    let archivedID: Int64
    let nextAudio: EnglishAudio?
    let segments: [EnglishSegment]
    let descriptionPreview: String

    enum CodingKeys: String, CodingKey {
        case segments
        case archivedID = "archived_id"
        case nextAudio = "next_audio"
        case descriptionPreview = "description_preview"
    }
}
