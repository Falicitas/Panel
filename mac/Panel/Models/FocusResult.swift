import Foundation

/// FocusItem 是 group 里单条 focus bullet 的结构 (跟 FocusResult 顶层 text/lineNo/breadcrumbs
/// 字段语义同, 但不含 anchored/found)。mac UI 在 group ≥ 2 时用此结构遍历切换。
struct FocusItem: Codable, Hashable, Identifiable {
    let text: String
    let lineNo: Int
    let breadcrumbs: [String]

    var id: Int { lineNo }

    enum CodingKeys: String, CodingKey {
        case text, breadcrumbs
        case lineNo = "line_no"
    }
}

struct FocusResult: Codable, Hashable {
    let anchored: String?
    let found: Bool
    let text: String?
    let lineNo: Int?
    let breadcrumbs: [String]?
    /// 所有未完成且标 [[[focus]]] 的 bullets, 按 lineNo asc。backend 仅当 ≥ 2 项时下发,
    /// 单项 / 无 focus 标记时为 nil → mac UI 不显示切换控件。
    let group: [FocusItem]?

    enum CodingKeys: String, CodingKey {
        case anchored, found, text, breadcrumbs, group
        case lineNo = "line_no"
    }
}

struct AnchorResponse: Codable {
    let name: String?
}
