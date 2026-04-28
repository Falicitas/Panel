import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        ZStack {
            MainLayout()
            if shouldShowWakeMask {
                WakeMaskView()
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: shouldShowWakeMask)
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

    /// 蒙版条件: 06:00+ 当日 mode 还没人为设置过 (mode_set_at == 0)。
    private var shouldShowWakeMask: Bool {
        let _ = state.timeTick
        let now = Date()
        let h = Calendar.current.component(.hour, from: now)
        guard h >= 6 else { return false }
        return (state.dailyState?.modeSetAt ?? 0) == 0
    }
}
