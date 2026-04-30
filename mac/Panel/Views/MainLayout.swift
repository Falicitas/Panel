import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(spacing: 0) {
            NotificationBar()
            // supplements 走 2 列:不存在二级 todolists 目录,直接 sidebar + 大版面
            // 其他 tab (todolist / finance / records / bash) 暂仍走 3 列
            if state.selectedTab == .supplements {
                SupplementsLayout()
            } else {
                ThreeColLayout()
            }
        }
    }
}

private struct SupplementsLayout: View {
    @Environment(AppState.self) private var state
    var body: some View {
        @Bindable var state = state
        NavigationSplitView(columnVisibility: $state.splitVisibility) {
            PrimarySidebarView(selected: $state.selectedTab)
        } detail: {
            SupplementsView()
        }
    }
}

private struct ThreeColLayout: View {
    @Environment(AppState.self) private var state
    var body: some View {
        @Bindable var state = state
        // 系统 hide sidebar (⌃⌘S) 时,跟着把 content 列宽收成 0
        let sidebarHidden = state.splitVisibility != .all
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

private struct ContentColumn: View {
    @Environment(AppState.self) private var state
    var body: some View {
        switch state.selectedTab {
        case .todolist:    TodolistContentView()
        case .finance:     FinanceContentView()
        case .supplements: EmptyView()  // 不会进到这里 (走 SupplementsLayout)
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
        case .supplements: EmptyView()  // 不会进到这里
        case .records:     RecordsDetailView()
        case .bash:        BashDetailView()
        }
    }
}
