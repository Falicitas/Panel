import Foundation
import Observation
import AppKit

/// 全局语音输入服务的协调器。@Observable 单例,启动时由 PanelApp 拉起。
///
/// 当前 (step 1) state machine 只有 idle / noPermission / error 三态。
/// step 2 加 recording、step 3 加 transcribing、step 5 加 noKey。
@Observable
final class VoiceController {
    static let shared = VoiceController()

    enum State: Equatable {
        case idle
        case noPermission
        case error
        // step 2+ 会加: recording / transcribing / noKey
    }

    private(set) var state: State = .idle
    private var refreshTicker: Timer?

    private init() {}

    /// PanelApp 启动时调用。
    /// 主动请求 Mic + Input Monitoring (这俩是 F12 + 录音的必要权限,native
    /// 简短对话框,首次弹一次)。Accessibility 不主动 prompt — 用户在菜单诊断
    /// 看到 ✗ 自己去勾 (避免 ad-hoc 签名 trust 漂移触发的循环骚扰)。
    func start() {
        Task { @MainActor in
            if !Permissions.hasMicrophone() {
                _ = await Permissions.requestMicrophone()
            }
            if !Permissions.hasInputMonitoring() {
                _ = Permissions.requestInputMonitoring()
            }
            self.refresh()
        }
        // 用户在系统设置授权权限后切回 panel → didBecomeActive 触发 refresh,状态立即更新。
        NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.refresh()
        }
        // 兜底周期 refresh:用户没失焦也能在 5s 内感知权限变化。
        if refreshTicker == nil {
            refreshTicker = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
                DispatchQueue.main.async { self?.refresh() }
            }
        }
    }

    /// 重新评估外部条件 (权限) 决定 state。
    func refresh() {
        // step 2+ 会加 recording / transcribing 排除
        let perms = Permissions.check()
        if !perms.blockingGranted {
            state = .noPermission
            return
        }
        state = .idle
    }
}
