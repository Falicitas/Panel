import SwiftUI

enum AppTab: String, CaseIterable, Identifiable, Hashable {
    case todolist
    case finance
    case supplements
    case english
    case records
    case bash

    var id: String { rawValue }

    var label: String {
        switch self {
        case .todolist:    "Todolist"
        case .finance:     "财务"
        case .supplements: "保健品"
        case .english:     "英文"
        case .records:     "记录"
        case .bash:        "Bash"
        }
    }

    var icon: String {
        switch self {
        case .todolist:    "checklist"
        case .finance:     "creditcard"
        case .supplements: "pills"
        case .english:     "headphones"
        case .records:     "chart.bar"
        case .bash:        "terminal"
        }
    }

    var isEnabled: Bool {
        switch self {
        case .todolist, .finance, .supplements, .english, .records, .bash: true
        }
    }

    var comingSoonHint: String {
        switch self {
        case .todolist, .finance, .supplements, .english, .records, .bash: ""
        }
    }
}
