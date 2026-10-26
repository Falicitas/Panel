import Foundation

struct DualNotesRow: Codable, Sendable {
    let id: String
    let left: String
    let right: String
}

struct DualNotesDay: Codable, Sendable {
    let date: String
    let revision: Int
    let rows: [DualNotesRow]
}

struct DualNotesPage: Codable, Sendable {
    let days: [DualNotesDay]
}

struct ShortTermWorkspaceRow: Codable, Sendable {
    let id: String
    let content: String
}

struct ShortTermWorkspaceTable: Codable, Sendable {
    let id: String
    let header: String
    let rows: [ShortTermWorkspaceRow]
}

struct ShortTermWorkspace: Codable, Sendable {
    let month: String
    let revision: Int
    let tables: [ShortTermWorkspaceTable]
}

struct ShortTermWorkspaceCopyRequest: Codable, Sendable {
    let operationId: String
    let sourceMonth: String
    let targetMonth: String
    let sourceRevision: Int
    let targetRevision: Int
    let tableIds: [String]
}

struct ShortTermWorkspaceCopyResult: Codable, Sendable {
    let operationId: String
    let target: ShortTermWorkspace
    let copiedTableIds: [String]
}
