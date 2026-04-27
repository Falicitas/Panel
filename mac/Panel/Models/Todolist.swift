import Foundation

struct Todolist: Codable, Identifiable, Hashable {
    let name: String
    let path: String
    let done: Bool
    let items: [TodoItem]
    let rawMD: String

    var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name, path, done, items
        case rawMD = "raw_md"
    }
}

struct TodoItem: Codable, Hashable {
    let lineNo: Int
    let level: Int
    let text: String
    let checked: Bool
    let skip: Bool
    let parentChain: [String]

    enum CodingKeys: String, CodingKey {
        case level, text, checked, skip
        case lineNo = "line_no"
        case parentChain = "parent_chain"
    }
}
