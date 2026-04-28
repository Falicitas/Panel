import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        VStack(spacing: 0) {
            NotificationBar()
            NavigationSplitView {
                PrimarySidebarView(selected: $state.selectedTab)
            } content: {
                ContentColumn()
            } detail: {
                DetailColumn()
            }
        }
    }
}

private struct ContentColumn: View {
    @Environment(AppState.self) private var state
    var body: some View {
        switch state.selectedTab {
        case .todolist:    TodolistContentView()
        case .micro:       MicroContentView()
        case .finance:     FinanceContentView()
        case .supplements: SupplementsContentView()
        case .records:     RecordsContentView()
        default:           ComingSoonView(tab: state.selectedTab)
        }
    }
}

private struct DetailColumn: View {
    @Environment(AppState.self) private var state
    var body: some View {
        switch state.selectedTab {
        case .todolist:    DetailView()
        case .micro:       MicroDetailView()
        case .finance:     FinanceDetailView()
        case .supplements: SupplementsDetailView()
        case .records:     RecordsDetailView()
        default:           ComingSoonView(tab: state.selectedTab)
        }
    }
}
