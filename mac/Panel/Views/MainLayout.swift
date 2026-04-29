import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        let sidebarHidden = state.splitVisibility != .all
        VStack(spacing: 0) {
            NotificationBar()
            NavigationSplitView(columnVisibility: $state.splitVisibility) {
                PrimarySidebarView(selected: $state.selectedTab)
            } content: {
                ContentColumn()
                    .navigationSplitViewColumnWidth(
                        min: sidebarHidden ? 0 : 280,
                        ideal: sidebarHidden ? 0 : 320,
                        max: sidebarHidden ? 0 : 400
                    )
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
