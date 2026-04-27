import Foundation

struct FocusResult: Codable, Hashable {
    let anchored: String?
    let found: Bool
    let text: String?
    let lineNo: Int?
    let breadcrumbs: [String]?

    enum CodingKeys: String, CodingKey {
        case anchored, found, text, breadcrumbs
        case lineNo = "line_no"
    }
}

struct AnchorResponse: Codable {
    let name: String?
}
