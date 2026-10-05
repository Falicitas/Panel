import SwiftUI

enum AppTab: String, CaseIterable, Identifiable, Hashable {
    // 双栏放在顶部；财务、记录和 Bash 暂未启用。
    case dualNotes
    case todolist
    case supplements
    case english
    case finance
    case records
    case bash

    var id: String { rawValue }

    var label: String {
        switch self {
        case .dualNotes:   "双栏"
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
        case .dualNotes:   "rectangle.split.2x1"
        case .todolist:    "checklist"
        case .supplements: "pills"
        case .english:     "headphones"
        case .finance:     "creditcard"
        case .records:     "chart.bar"
        case .bash:        "terminal"
        }
    }

    /// 已启用的模块可从侧栏切换，其余模块暂时灰显。
    /// 原代码已有可用功能不删, 后续解禁只需把 case 加回 true 分支。
    var isEnabled: Bool {
        switch self {
        case .dualNotes, .todolist, .supplements, .english: true
        case .finance, .records, .bash:         false
        }
    }

    var comingSoonHint: String {
        switch self {
        case .dualNotes, .todolist, .supplements, .english: ""
        case .finance, .records, .bash:         "测试期暂停, 优先 todolist / 保健品 / 英文"
        }
    }
}
