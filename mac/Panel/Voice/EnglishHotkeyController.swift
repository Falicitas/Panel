import Foundation
import os.log

/// 英文精听全局快捷键 controller (shared 单例).
///
/// 注册的 key:
///   - F6: 「最新一个没勾的段落」toggle play / pause / resume
///   - F5: 同段 seek -5s (clamp 到段开头)
///
/// 「最新一个没勾的段落」= currentAudio.segments 中第一个 idx 不在 doneSegIdx 的段.
/// 没 audio / 全勾完 / segments 空 → no-op.
///
/// 用户场景: 焦点在 Typora 等其他 app, 一边写笔记一边精听, 不需要切回 Panel
/// 就能控制段落播放. 跟 F12 (录音) 共用 HotkeyTap.shared.
@MainActor
final class EnglishHotkeyController {
    static let shared = EnglishHotkeyController()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "english.hotkey")

    private weak var stateRef: AppState?
    private let player = SegmentPlayer.shared

    /// F5 seek 回退秒数.
    private let seekBackSec: Double = 5.0

    private init() {}

    /// PanelApp.task 启动后调一次. state 是 SwiftUI 持有的 AppState (@Observable),
    /// weak 引用避免循环.
    func bind(state: AppState) {
        self.stateRef = state
        HotkeyTap.shared.register(keyCode: HotkeyTap.f5KeyCode) { [weak self] in
            Task { @MainActor in self?.handleF5() }
        }
        HotkeyTap.shared.register(keyCode: HotkeyTap.f6KeyCode) { [weak self] in
            Task { @MainActor in self?.handleF6() }
        }
        HotkeyTap.shared.start()
        os_log("F5/F6 hotkeys registered", log: Self.log, type: .info)
    }

    /// "最新一个没勾的段落": currentAudio.segments 中第一个 idx 不在 doneSegIdx 的段.
    /// 没 audio / 全勾完 / segments 空 → nil.
    private func currentSegment() -> (audio: EnglishAudio, segment: EnglishSegment)? {
        guard let st = stateRef,
              let today = st.englishToday,
              let audio = today.currentAudio,
              !today.segments.isEmpty,
              !audio.localPath.isEmpty else { return nil }
        let doneSet = Set(today.doneSegIdx)
        guard let seg = today.segments.first(where: { !doneSet.contains($0.idx) }) else {
            return nil
        }
        return (audio, seg)
    }

    /// F6: 当前段三态 toggle.
    ///   - 在播 (bound && !paused) → pause
    ///   - 已绑定但暂停 → resume
    ///   - 未绑定 (该段没在播 / player 空 / 在播别段) → play 该段从段头
    private func handleF6() {
        guard let (audio, seg) = currentSegment() else {
            os_log("F6: no current segment (no audio / all done / no localPath)", log: Self.log, type: .info)
            return
        }
        let bound = player.isPlaying(audioID: audio.id, segIdx: seg.idx)
        let playing = bound && !player.paused
        if playing {
            os_log("F6: pause seg %d", log: Self.log, type: .info, seg.idx)
            player.pause()
            return
        }
        if bound {
            os_log("F6: resume seg %d", log: Self.log, type: .info, seg.idx)
            player.resume()
            return
        }
        os_log("F6: play seg %d", log: Self.log, type: .info, seg.idx)
        let url = URL(fileURLWithPath: audio.localPath)
        let endMs = audio.durationMs > 0
            ? min(Int(audio.durationMs), Int(seg.endMs))
            : Int(seg.endMs)
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: seg.idx,
                startMs: max(0, Int(seg.startMs)),
                endMs: endMs
            )
        } catch {
            os_log("F6 play failed: %{public}@", log: Self.log, type: .error, String(describing: error))
        }
    }

    /// F5: 当前段 seek -5s, clamp 到段开头.
    ///
    /// 仅在 player 已绑定到该段 (在播 / 暂停中) 时生效. 未绑定时 no-op —
    /// 用户没在听这段, 回退没意义; 想从段头开始用 F6.
    private func handleF5() {
        guard let (audio, seg) = currentSegment() else {
            os_log("F5: no current segment", log: Self.log, type: .info)
            return
        }
        guard player.isPlaying(audioID: audio.id, segIdx: seg.idx) else {
            os_log("F5: seg %d not bound, ignore", log: Self.log, type: .info, seg.idx)
            return
        }
        let target = max(player.startSec, player.currentSec - seekBackSec)
        os_log("F5: seek seg %d to %.2fs", log: Self.log, type: .info, seg.idx, target)
        player.seek(to: target)
    }
}
