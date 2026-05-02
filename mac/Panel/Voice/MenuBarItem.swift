import SwiftUI

/// MenuBarExtra 的 label 视图 — 状态切换 SF Symbol。
/// @Observable 自动观察 controller.state 变化触发刷新。
struct MenuBarLabel: View {
    var controller: VoiceController

    var body: some View {
        Image(systemName: iconName)
    }

    private var iconName: String {
        switch controller.state {
        case .idle:         "mic"
        case .noPermission: "lock.shield"
        case .error:        "exclamationmark.triangle.fill"
        }
    }
}

/// MenuBarExtra 展开后的菜单内容。
struct MenuBarContent: View {
    var controller: VoiceController

    var body: some View {
        let perms = Permissions.check()
        Group {
            Text(statusText).foregroundStyle(.secondary)
            Divider()
            Text("权限诊断").foregroundStyle(.tertiary)
            Text(perms.inputMonitoring ? "✓ 输入监控 (F12 收事件)"   : "✗ 输入监控 (F12 收事件)")
            Text(perms.microphone      ? "✓ 麦克风 (录音)"          : "✗ 麦克风 (录音)")
            Text(perms.accessibility   ? "✓ 辅助功能 (注入文字)"     : "✗ 辅助功能 (注入文字) — 不阻塞")
            Divider()
            Button("打开「输入监控」系统设置") {
                if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent") {
                    NSWorkspace.shared.open(url)
                }
            }
            Button("打开「辅助功能」系统设置") {
                if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
                    NSWorkspace.shared.open(url)
                }
            }
            Divider()
            Button("退出 Panel") {
                NSApp.terminate(nil)
            }
            .keyboardShortcut("q", modifiers: .command)
        }
    }

    private var statusText: String {
        switch controller.state {
        case .idle:         "就绪 (step 1 — F12 还没接,只展示菜单)"
        case .noPermission: "缺权限 — 看下方诊断"
        case .error:        "上次失败"
        }
    }
}
