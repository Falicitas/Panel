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
            return
        }
        state = .recording
    }

    private func endRecordingAndTranscribe() {
        guard let url = recorder.stop() else {
            state = .error
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
            }
        }
    }
}
