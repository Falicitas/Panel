import SwiftUI
import AppKit

@main
struct PanelApp: App {
    @State private var state = AppState()
    @State private var voice = VoiceController.shared

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(state)
                .frame(minWidth: 900, minHeight: 560)
                .background(WindowAccessor { window in
                    // 去掉 titlebar 与下方 NotificationBar 之间那条 1-2px 分隔线。
                    // 保留 traffic lights / "Panel" 标题, 只消除 separator (macOS 11+)。
                    window.titlebarSeparatorStyle = .none
                })
                .task {
                    await state.loadAll()
                    voice.start()
                }
        }
        .windowResizability(.contentMinSize)

        Settings {
            VoiceSettingsView()
        }

        MenuBarExtra {
            MenuBarContent(controller: voice)
        } label: {
            MenuBarLabel(controller: voice)
        }
    }
}

/// 桥到 NSWindow 做 SwiftUI 不直接暴露的窗口配置 (titlebarSeparatorStyle 等)。
/// 0-size NSView, 不影响 layout; window 在 view 加进 hierarchy 后才可拿, 故 async 调用。
private struct WindowAccessor: NSViewRepresentable {
    let onWindow: (NSWindow) -> Void
    func makeNSView(context: Context) -> NSView {
        let v = NSView()
        DispatchQueue.main.async {
            if let w = v.window {
                onWindow(w)
            }
        }
        return v
    }
    func updateNSView(_: NSView, context: Context) {}
}
