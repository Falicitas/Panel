import SwiftUI

struct MenuBarLabel: View {
    var controller: VoiceController

    var body: some View {
        Image(systemName: iconName)
    }

    private var iconName: String {
        switch controller.state {
        case .idle:         "mic"
        case .recording:    "mic.fill"
        case .noPermission: "lock.shield"
        case .error:        "exclamationmark.triangle.fill"
        }
    }
}

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
            Text(controller.isHotkeyActive ? "✓ F12 hotkey 已 install" : "✗ F12 hotkey 未 install (重启 Panel)")
            if let url = controller.lastRecordingURL {
                Divider()
                Text("上次录音").foregroundStyle(.tertiary)
                Text(url.lastPathComponent).font(.caption.monospacedDigit())
                Button("在 Finder 显示 / 用 QuickLook 听") {
                    NSWorkspace.shared.activateFileViewerSelecting([url])
                }
            }
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
        case .idle:         "就绪 (step 3 — 按 F12 真录音落 m4a)"
        case .recording:    "录音中 — 再按 F12 结束并存文件"
        case .noPermission: "缺权限 — 看下方诊断"
        case .error:        "上次失败 (录音初始化?)"
        }
    }
}
