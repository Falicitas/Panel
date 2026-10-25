import SwiftUI

@MainActor
struct ContentView: View {
    @Environment(AppState.self) private var state
    // 每个窗口保留自己的网页，窗口间不共享 NSView。
    @StateObject private var autoDL = AutoDLController()

    var body: some View {
        // 打开应用即可使用；日间状态和 recurring 设置不再阻挡主界面。
        MainLayout()
            .environmentObject(autoDL)
            .task { await state.startPolling() }
            .alert(
                "加载错误",
                isPresented: Binding(
                    // 双栏与 AutoDL 服务独立运行，不显示主后端的加载错误。
                    get: { ![AppTab.dualNotes, .autodl].contains(state.selectedTab) && state.lastError != nil },
                    set: { if !$0 { state.lastError = nil } }
                ),
                actions: { Button("OK", role: .cancel) {} },
                message: { Text(state.lastError ?? "") }
            )
    }
}
