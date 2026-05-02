import Foundation
import Observation
import AppKit
import os.log

/// 全局语音输入服务的协调器。@Observable 单例,启动时由 PanelApp 拉起。
///
/// step 2 state machine:idle / recording / noPermission / error
/// step 3 加 transcribing、step 5 加 noKey。
@Observable
final class VoiceController {
    static let shared = VoiceController()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "voice.ctrl")

    enum State: Equatable {
        case idle
        case recording
        case noPermission
        case error
    }

    private(set) var state: State = .idle
    /// step 3 验证用 — 上次录音文件 URL,菜单加按钮"在 Finder 显示"用。
    private(set) var lastRecordingURL: URL?

    /// 菜单诊断用 — hotkey tap 是否已成功 install (失败常见原因:Input Monitoring 未授权)
    var isHotkeyActive: Bool { hotkey?.isActive ?? false }

    private var hotkey: HotkeyTap?
    private let recorder = Recorder()
    private var refreshTicker: Timer?

    private init() {}

    func start() {
        if hotkey == nil {
            hotkey = HotkeyTap(onF12: { [weak self] in self?.handleF12() })
            hotkey?.start()
        }
        Task { @MainActor in
            if !Permissions.hasMicrophone() {
                _ = await Permissions.requestMicrophone()
            }
            if !Permissions.hasInputMonitoring() {
                _ = Permissions.requestInputMonitoring()
            }
            self.refresh()
        }
        NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.refresh()
        }
        if refreshTicker == nil {
            refreshTicker = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
                DispatchQueue.main.async { self?.refresh() }
            }
        }
    }

    func refresh() {
        // hotkey retry 顶部 — 跟 state 解耦,用户开了 Input Monitoring 后无论 state 都尝试 install。
        if hotkey?.isActive == false {
            os_log("refresh: hotkey 未 active, retry start", log: Self.log, type: .info)
            hotkey?.start()
        }
        // 录音中不打断 state
        if state == .recording { return }
        let perms = Permissions.check()
        if !perms.blockingGranted {
            state = .noPermission
            return
        }
        state = .idle
    }

    private func handleF12() {
        os_log("handleF12: state=%{public}@", log: Self.log, type: .info, String(describing: state))
        switch state {
        case .idle:
            guard recorder.start() else {
                os_log("recorder.start 失败 (mic 权限?)", log: Self.log, type: .error)
                state = .error
                return
            }
            state = .recording
        case .recording:
            let url = recorder.stop()
            state = .idle
            lastRecordingURL = url
            if let url {
                os_log("录音存到: %{public}@", log: Self.log, type: .info, url.path)
            }
            // step 4 才发去 transcribe;step 3 仅落文件供验证
        case .noPermission, .error:
            return
        }
    }
}
