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
