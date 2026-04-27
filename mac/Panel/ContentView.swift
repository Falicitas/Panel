import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state
    @State private var selectedName: String?

    var body: some View {
        @Bindable var state = state
        NavigationSplitView {
            SidebarView(selectedName: $selectedName)
        } detail: {
            DetailView()
        }
        .onChange(of: state.anchor, initial: true) { _, newValue in
            selectedName = newValue
        }
        .onChange(of: selectedName) { _, newValue in
            guard newValue != state.anchor else { return }
            Task { await state.setAnchor(name: newValue) }
        }
        .task {
            await state.startPolling()
        }
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
