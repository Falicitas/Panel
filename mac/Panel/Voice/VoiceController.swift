import Foundation
import Observation
import AppKit
import os.log

/// 全局语音输入服务的协调器。@Observable 单例,启动时由 PanelApp 拉起。
///
/// step 4 state machine:idle / recording / transcribing / noPermission / noKey / error
/// step 5 加 60s 上限 / error flash 自恢复 / 重复 F12 边界。
@Observable
final class VoiceController {
    static let shared = VoiceController()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "voice.ctrl")

    enum State: Equatable {
        case idle
        case recording
        case transcribing
        case noPermission
        case noKey
        case error
    }

    private(set) var state: State = .idle
    private(set) var lastRecordingURL: URL?

    var isHotkeyActive: Bool { hotkey?.isActive ?? false }

    private var hotkey: HotkeyTap?
    private let recorder = Recorder()
    private var refreshTicker: Timer?
    private var recordingTimer: Timer?

    /// 录音上限 (秒);超过强制 stop 并 transcribe。
    private let maxRecordingSec: TimeInterval = 300
    /// error 自恢复时间 (秒) — 不让用户卡在 error 状态。
    private let errorFlashSec: TimeInterval = 1.5

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
        if hotkey?.isActive == false {
            os_log("refresh: hotkey 未 active, retry", log: Self.log, type: .info)
            hotkey?.start()
        }
        // 录音 / 转写中不打断
        if state == .recording || state == .transcribing { return }
        let perms = Permissions.check()
        if !perms.blockingGranted {
            state = .noPermission
            return
        }
        if !KeyStore.hasKey {
            state = .noKey
            return
        }
        state = .idle
    }

    private func handleF12() {
        os_log("handleF12: state=%{public}@", log: Self.log, type: .info, String(describing: state))
        switch state {
        case .idle:
            beginRecording()
        case .recording:
            endRecordingAndTranscribe()
        case .transcribing, .noKey, .noPermission, .error:
            // step 5 加 flash 提示;暂时静默
            return
        }
    }

    private func beginRecording() {
        guard recorder.start() else {
            os_log("recorder.start 失败", log: Self.log, type: .error)
            state = .error
            scheduleErrorClear()
            return
        }
        state = .recording
        // 5 分钟上限,到点强制停止并送转写。
        recordingTimer?.invalidate()
        recordingTimer = Timer.scheduledTimer(withTimeInterval: maxRecordingSec, repeats: false) { [weak self] _ in
            DispatchQueue.main.async {
                guard let self, self.state == .recording else { return }
                os_log("录音达 %.0fs 上限,强制 stop", log: Self.log, type: .info, self.maxRecordingSec)
                self.endRecordingAndTranscribe()
            }
        }
    }

    private func endRecordingAndTranscribe() {
        recordingTimer?.invalidate()
        recordingTimer = nil
        guard let url = recorder.stop() else {
            state = .error
            scheduleErrorClear()
            return
        }
        lastRecordingURL = url
        state = .transcribing
        os_log("录音完: %{public}@,开始转写", log: Self.log, type: .info, url.path)
        Task { await runTranscribe(url: url) }
    }

    private func runTranscribe(url: URL) async {
        do {
            let text = try await Transcriber.transcribe(audioFile: url)
            try? FileManager.default.removeItem(at: url)
            await MainActor.run {
                if !text.isEmpty {
                    os_log("转写结果 (%d chars): %{public}@", log: Self.log, type: .info, text.count, text)
                    TextInjector.inject(text)
                }
                self.state = .idle
            }
        } catch {
            try? FileManager.default.removeItem(at: url)
            os_log("转写失败: %{public}@", log: Self.log, type: .error, String(describing: error))
            await MainActor.run {
                self.state = .error
                self.scheduleErrorClear()
            }
        }
    }

    /// error 1.5s 后自动 refresh 回 idle (或 noKey/noPermission,根据当前条件)。
    /// 防止用户卡在 ⚠ 状态没法继续。
    private func scheduleErrorClear() {
        DispatchQueue.main.asyncAfter(deadline: .now() + errorFlashSec) { [weak self] in
            guard let self else { return }
            if self.state == .error {
                self.refresh()
            }
        }
    }
}
