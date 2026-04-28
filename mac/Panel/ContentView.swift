import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
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
