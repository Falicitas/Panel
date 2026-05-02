import AVFoundation
import ApplicationServices
import CoreGraphics

/// 三个权限分工:
/// - Input Monitoring (CGEventTap 收 F12)       — 必须,缺则 F12 收不到
/// - Microphone (AVAudioRecorder 录音)          — 必须,缺则录不了音
/// - Accessibility (CGEvent 注入文字到其他 App) — 仅注入阶段需要,缺时录音/转写仍能跑;
///                                              不主动 prompt (那个引导对话框跟 ad-hoc
///                                              签名 hash 漂移会形成"每次重 build 都
///                                              要去系统设置勾"的痛苦循环);用户在菜单
///                                              诊断里看到 ✗ 时自己去勾。
enum Permissions {
    struct Status: Equatable {
        let inputMonitoring: Bool
        let microphone: Bool
        let accessibility: Bool

        /// 仅 Input Monitoring + Microphone 是阻塞性。
        /// Accessibility 缺只影响最后注入,不阻塞使用。
        var blockingGranted: Bool { inputMonitoring && microphone }
    }

    static func check() -> Status {
        Status(
            inputMonitoring: hasInputMonitoring(),
            microphone: hasMicrophone(),
            accessibility: hasAccessibility()
        )
    }

    // MARK: - Input Monitoring

    static func hasInputMonitoring() -> Bool {
        CGPreflightListenEventAccess()
    }

    /// 弹系统对话框请求 Input Monitoring,授权后让 Panel 出现在
    /// 「系统设置 > 隐私与安全性 > 输入监控」。
    @discardableResult
    static func requestInputMonitoring() -> Bool {
        CGRequestListenEventAccess()
    }

    // MARK: - Microphone

    static func hasMicrophone() -> Bool {
        AVCaptureDevice.authorizationStatus(for: .audio) == .authorized
    }

    static func requestMicrophone() async -> Bool {
        await AVCaptureDevice.requestAccess(for: .audio)
    }

    // MARK: - Accessibility (silent check, 不主动 prompt)

    static func hasAccessibility() -> Bool {
        AXIsProcessTrusted()
    }
}
