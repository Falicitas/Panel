import Foundation

struct ListEntry: Codable, Identifiable, Hashable {
    let name: String
    let totalTodo: Int
    let done: Int
    let skipped: Int

    var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name
        case totalTodo = "total_todo"
        case done, skipped
    }
}

struct TodolistListResponse: Codable {
    let active: [ListEntry]
    let done: [ListEntry]
}
