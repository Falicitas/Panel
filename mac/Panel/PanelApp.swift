import SwiftUI
import AppKit
import Speech

@main
struct PanelApp: App {
    @State private var state = AppState()
    @State private var voice = VoiceController.shared

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(state)
                .focusedSceneValue(\.pageZoomState, state)
                .environment(SegmentPlayer.shared)
                .frame(minWidth: 560, minHeight: 560)
                .background(PageZoomShortcuts { state.performPageZoom($0) })
                .background(WindowAccessor { window in
                    // 去掉 titlebar 与下方 NotificationBar 之间那条 1-2px 分隔线。
                    // 保留 traffic lights / "Panel" 标题, 只消除 separator (macOS 11+)。
                    window.titlebarSeparatorStyle = .none
                })
                .task {
                    UrgentNotifier.shared.setUp()
                    // 首次启动弹"语音识别"系统对话框 (用户允许 / 拒绝);
                    // 拒绝时 EnglishState 后续 transcribe 会标 segments_status=failed,UI 提示
                    SFSpeechRecognizer.requestAuthorization { _ in }
                    await state.loadAll()
                    state.ensureSegmentsForCurrentAudio()
                    voice.start()
                    EnglishHotkeyController.shared.bind(state: state)
                }
        }
        .windowResizability(.contentMinSize)
        .commands { PageZoomCommands() }

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
        let v = StartupFocusView()
        DispatchQueue.main.async {
            if let w = v.window {
                onWindow(w)
                // 只在窗口创建时设置中性的初始焦点，避免工具栏按钮被自动选中。
                w.initialFirstResponder = v
                w.makeFirstResponder(v)
            }
        }
        return v
    }
    func updateNSView(_: NSView, context: Context) {}
}

/// 可接收初始键盘焦点的空视图，不绘制焦点环，也不拦截后续键盘导航。
private final class StartupFocusView: NSView {
    override var acceptsFirstResponder: Bool { true }
}

private struct PageZoomStateKey: FocusedValueKey {
    typealias Value = AppState
}

private extension FocusedValues {
    var pageZoomState: AppState? {
        get { self[PageZoomStateKey.self] }
        set { self[PageZoomStateKey.self] = newValue }
    }
}

private struct PageZoomCommands: Commands {
    @FocusedValue(\.pageZoomState) private var state

    var body: some Commands {
        CommandGroup(after: .toolbar) {
            Divider()
            Button("放大页面") { state?.performPageZoom(.zoomIn) }
                .keyboardShortcut("+", modifiers: .command)
                .disabled(state?.canZoomIn != true)
            Button("缩小页面") { state?.performPageZoom(.zoomOut) }
                .keyboardShortcut("-", modifiers: .command)
                .disabled(state?.canZoomOut != true)
            Button("实际大小（100％）") { state?.performPageZoom(.reset) }
                .keyboardShortcut("0", modifiers: .command)
                .disabled(state?.supportsPageZoom != true)
            if let state, state.supportsPageZoom {
                Text("页面缩放：\(state.pageZoomPercent)％")
            }
        }
    }
}
