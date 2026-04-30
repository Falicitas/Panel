import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        // 折叠条件:
        //   - 用户系统 hide sidebar (state.splitVisibility != .all),或
        //   - 当前 tab 是 supplements (设计上不需要二级列)
        let collapseContent = state.splitVisibility != .all || state.selectedTab == .supplements
        VStack(spacing: 0) {
            NotificationBar()
            NavigationSplitView(columnVisibility: $state.splitVisibility) {
                PrimarySidebarView(selected: $state.selectedTab)
            } content: {
                ContentColumn()
                    .navigationSplitViewColumnWidth(
                        min: collapseContent ? 0 : 280,
                        ideal: collapseContent ? 0 : 320,
                        max: collapseContent ? 0 : 400
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
        case .supplements: EmptyView()  // supplements 走 2 列布局,内容全在 DetailColumn
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
        case .supplements: SupplementsView()
        case .records:     RecordsDetailView()
        case .bash:        BashDetailView()
        }
    }
}
