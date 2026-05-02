import SwiftUI

@main
struct PanelApp: App {
    @State private var state = AppState()
    @State private var voice = VoiceController.shared

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(state)
                .frame(minWidth: 900, minHeight: 560)
                .task {
                    await state.loadAll()
                    voice.start()
                }
        }
        .windowResizability(.contentMinSize)

        MenuBarExtra {
            MenuBarContent(controller: voice)
        } label: {
            MenuBarLabel(controller: voice)
        }
    }
}
