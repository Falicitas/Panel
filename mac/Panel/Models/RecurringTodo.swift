import Foundation

enum RecurringKind: String, Codable, CaseIterable, Hashable {
    case morning
    case phone
    case free
}

struct RecurringTodo: Codable, Identifiable, Hashable {
    let id: Int64
    let content: String
    let sortOrder: Int
    let createdAt: Int64
    let kind: RecurringKind

    enum CodingKeys: String, CodingKey {
        case id, content, kind
        case sortOrder = "sort_order"
        case createdAt = "created_at"
    }
}
