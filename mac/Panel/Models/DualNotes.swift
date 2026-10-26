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
