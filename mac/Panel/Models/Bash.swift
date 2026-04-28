import Foundation

struct BashHistoryEntry: Codable, Identifiable, Hashable {
    let id: Int64
    let cmd: String
    let stdout: String
    let stderr: String
    let exitCode: Int
    let durationMs: Int64
    let createdAt: Int64

    enum CodingKeys: String, CodingKey {
        case id, cmd, stdout, stderr
        case exitCode = "exit_code"
        case durationMs = "duration_ms"
        case createdAt = "created_at"
    }
}

/// 后端 POST /api/exec 的响应:复用 history entry 字段 + timed_out flag。
struct BashExecResult: Codable, Hashable {
    let id: Int64
    let cmd: String
    let stdout: String
    let stderr: String
    let exitCode: Int
    let durationMs: Int64
    let createdAt: Int64
    let timedOut: Bool

    enum CodingKeys: String, CodingKey {
        case id, cmd, stdout, stderr
        case exitCode = "exit_code"
        case durationMs = "duration_ms"
        case createdAt = "created_at"
        case timedOut = "timed_out"
    }

    var asHistoryEntry: BashHistoryEntry {
        BashHistoryEntry(
            id: id, cmd: cmd, stdout: stdout, stderr: stderr,
            exitCode: exitCode, durationMs: durationMs, createdAt: createdAt
        )
    }
}
