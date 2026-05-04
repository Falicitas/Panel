import SwiftUI

struct MainLayout: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        // 系统 hide sidebar (⌃⌘S) 把 splitVisibility 切到 .doubleColumn,我们读这个
        // 派生 sidebarHidden,在 sidebar 隐藏时把 content 列宽一并收为 0,
        // 视觉上把 .doubleColumn 等价为 .detailOnly,即一二级一起折叠。
        let sidebarHidden = state.splitVisibility != .all
        // supplements 没有二级 todolist 列表,content 列宽永久收为 0;
        // 但 NavigationSplitView 结构始终是 3 列,不重建,
        // 这样系统原生的 hidesidebar 按钮跨 tab 切换不丢。
        // supplements / english 都没有二级 list,content 列宽永久收为 0
        let collapseTabs: Set<AppTab> = [.supplements, .english]
        let collapseContent = sidebarHidden || collapseTabs.contains(state.selectedTab)
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
        case .todolist:
            TodolistContentView()
                .overlay { SleepGuardOverlay() }
        case .finance:     FinanceContentView()
        case .supplements: Color.clear  // 列宽收 0;Color.clear 防 macOS 因为完全空内容把 toggle 一并丢
        case .english:     Color.clear
        case .records:     RecordsContentView()
        case .bash:        BashContentView()
        }
    }
}

private struct DetailColumn: View {
    @Environment(AppState.self) private var state
    var body: some View {
        switch state.selectedTab {
        case .todolist:
            DetailView()
                .overlay { SleepGuardOverlay() }
        case .finance:     FinanceDetailView()
        case .supplements: SupplementsView()
        case .english:     EnglishView()
        case .records:     RecordsDetailView()
        case .bash:        BashDetailView()
        }
    }
}
