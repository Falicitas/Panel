import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        // 系统 hide sidebar (⌃⌘S) 把 splitVisibility 切到 .doubleColumn,我们读这个
        // 派生 sidebarHidden,在 3 列模式下把 content 列宽一并收为 0。
        let sidebarHidden = state.splitVisibility != .all
        VStack(spacing: 0) {
            NotificationBar()
            // supplements 走 2 列:不存在二级 todolists 目录,直接 sidebar + 大版面
            // 其他 tab (todolist / finance / records / bash) 走 3 列
            if state.selectedTab == .supplements {
                NavigationSplitView(columnVisibility: $state.splitVisibility) {
                    PrimarySidebarView(selected: $state.selectedTab)
                } detail: {
                    SupplementsView()
                }
            } else {
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
        // 切 tab 时重置 splitVisibility 为 .all。
        // Bug: 在 supplements (2 列) 收了 sidebar 后切回 todolist (3 列),
        // 新 NavigationSplitView 起手就接 .doubleColumn,sidebar 出生即隐藏 +
        // macOS 没把自动 sidebar toggle 加到 toolbar,用户被卡死无法重显。
        // 每次切 tab 强制 .all 起手,sidebar 永远默认可见 (用户可在新 tab 内
        // 重新 ⌃⌘S 收起)。
        .onChange(of: state.selectedTab) { _, _ in
            if state.splitVisibility != .all {
                state.splitVisibility = .all
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
        case .supplements: EmptyView()  // 不会进到这里 (走 2 列分支)
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
