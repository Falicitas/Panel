import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        // 打开应用即可使用；日间状态和 recurring 设置不再阻挡主界面。
        MainLayout()
            .task { await state.startPolling() }
            .alert(
                "加载错误",
                isPresented: Binding(
                    get: { state.lastError != nil },
                    set: { if !$0 { state.lastError = nil } }
                ),
                actions: { Button("OK", role: .cancel) {} },
                message: { Text(state.lastError ?? "") }
            )
    }
}
