import SwiftUI

@main
struct PanelApp: App {
    @State private var state = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(state)
                .frame(minWidth: 900, minHeight: 560)
                .task {
                    await state.loadAll()
                }
        }
        .windowResizability(.contentMinSize)
    }
}
