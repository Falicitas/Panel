import Foundation

struct RecurringTodo: Codable, Identifiable, Hashable {
    let id: Int64
    let content: String
    let sortOrder: Int
    let createdAt: Int64

    enum CodingKeys: String, CodingKey {
        case id, content
        case sortOrder = "sort_order"
        case createdAt = "created_at"
    }
}
