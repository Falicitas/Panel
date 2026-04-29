import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        VStack(spacing: 0) {
            NotificationBar()
            NavigationSplitView(columnVisibility: $state.splitVisibility) {
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
        case .finance:     FinanceContentView()
        case .supplements: SupplementsContentView()
        case .records:     RecordsContentView()
        case .bash:        BashContentView()
        }
    }
}

private struct DetailColumn: View {
    @Environment(AppState.self) private var state
    var body: some View {
        switch state.selectedTab {
        case .todolist:    DetailView()
        case .finance:     FinanceDetailView()
        case .supplements: SupplementsDetailView()
        case .records:     RecordsDetailView()
        case .bash:        BashDetailView()
        }
    }
}
