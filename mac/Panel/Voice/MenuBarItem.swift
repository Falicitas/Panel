import SwiftUI
import AppKit

struct MenuBarLabel: View {
    var controller: VoiceController

    var body: some View {
        Image(systemName: iconName)
    }

    private var iconName: String {
        switch controller.state {
        case .idle:         "mic"
        case .recording:    "mic.fill"
        case .transcribing: "arrow.triangle.2.circlepath"
        case .noPermission: "lock.shield"
        case .noKey:        "key.slash"
        case .error:        "exclamationmark.triangle.fill"
        }
    }
}

struct MenuBarContent: View {
    var controller: VoiceController
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        let perms = Permissions.check()
        Group {
            Text(statusText).foregroundStyle(.secondary)
            Divider()
            Text("权限诊断").foregroundStyle(.tertiary)
            Text(perms.inputMonitoring ? "✓ 输入监控 (F12 收事件)"   : "✗ 输入监控 (F12 收事件)")
            Text(perms.microphone      ? "✓ 麦克风 (录音)"          : "✗ 麦克风 (录音)")
            Text(perms.accessibility   ? "✓ 辅助功能 (注入文字)"     : "✗ 辅助功能 (注入文字) — 不阻塞,但注入会失败")
            Text(controller.isHotkeyActive ? "✓ F12 hotkey 已 install" : "✗ F12 hotkey 未 install (重启 Panel)")
            Text(KeyStore.hasKey       ? "✓ OpenRouter Key"        : "✗ OpenRouter Key (Cmd+, 设置)")
            if let url = controller.lastRecordingURL {
                Divider()
                Text("上次录音").foregroundStyle(.tertiary)
                Text(url.lastPathComponent).font(.caption.monospacedDigit())
                Button("在 Finder 显示 / 用 QuickLook 听") {
                    NSWorkspace.shared.activateFileViewerSelecting([url])
                }
            }
            Divider()
            Button("打开设置...") {
                bringSettingsToFront()
            }
            .keyboardShortcut(",", modifiers: .command)
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
        case .idle:         "就绪 — 按 F12 开始录音"
        case .recording:    "录音中 — 再按 F12 结束"
        case .transcribing: "转写中..."
        case .noPermission: "缺权限 — 看下方诊断"
        case .noKey:        "未设置 OpenRouter Key — Cmd+,"
        case .error:        "上次失败"
        }
    }

    /// 系统 SwiftUI Settings scene 在已存在窗口被其它 app/窗口遮挡时, openSettings()
    /// 不一定会把它 bring-to-front (尤其从 menu bar item 触发, app 当前不是 frontmost)。
    /// 三步保险:
    ///   1) NSApp.activate(ignoringOtherApps: true) — 先把 Panel 提到前台
    ///   2) openSettings() — 创建 (首次) 或激活 SwiftUI Settings 窗口
    ///   3) async 找 NSApp.windows 中 settings 窗口 → makeKeyAndOrderFront + orderFrontRegardless
    private func bringSettingsToFront() {
        NSApp.activate(ignoringOtherApps: true)
        openSettings()
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
            for w in NSApp.windows where isSettingsWindow(w) {
                w.makeKeyAndOrderFront(nil)
                w.orderFrontRegardless()
            }
        }
    }

    /// SwiftUI Settings 窗口的 NSWindow 没有标准 identifier API, 用 title 含 'settings'
    /// (英文 macOS) 或 '设置' (中文 macOS) 识别。
    private func isSettingsWindow(_ w: NSWindow) -> Bool {
        let t = w.title.lowercased()
        return t.contains("settings") || t.contains("设置") || t.contains("preferences")
    }
}
