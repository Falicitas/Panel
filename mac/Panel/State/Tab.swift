import SwiftUI

enum AppTab: String, CaseIterable, Identifiable, Hashable {
    // 常用的保健品和英文在前，Todolist 紧接英文；后 3 个暂未启用。
    case supplements
    case english
    case todolist
    case finance
    case records
    case bash

    var id: String { rawValue }

    var label: String {
        switch self {
        case .todolist:    "Todolist"
        case .supplements: "食物/保健品"
        case .english:     "英文"
        case .finance:     "财务"
        case .records:     "记录"
        case .bash:        "Bash"
        }
    }

    var icon: String {
        switch self {
        case .todolist:    "checklist"
        case .supplements: "pills"
        case .english:     "headphones"
        case .finance:     "creditcard"
        case .records:     "chart.bar"
        case .bash:        "terminal"
        }
    }

    /// 测试期前 3 个 enabled, 后 3 个 disabled (sidebar 灰显, 不可切)。
    /// 原代码已有可用功能不删, 后续解禁只需把 case 加回 true 分支。
    var isEnabled: Bool {
        switch self {
        case .todolist, .supplements, .english: true
        case .finance, .records, .bash:         false
        }
    }

    var comingSoonHint: String {
        switch self {
        case .todolist, .supplements, .english: ""
        case .finance, .records, .bash:         "测试期暂停, 优先 todolist / 保健品 / 英文"
        }
    }
}
