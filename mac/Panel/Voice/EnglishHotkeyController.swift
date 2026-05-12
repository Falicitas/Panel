import AppKit
import Foundation
import os.log

/// 英文精听全局快捷键 controller (shared 单例).
///
/// 注册的 key (focused 在哪个 app 都生效, 通过 HotkeyTap.shared 的 CGEventTap):
///   - F4: 锚定段字幕显示 toggle (state.anchorShowText)
///   - F5: 锚定段 seek -5s (clamp 到段开头)
///   - F6: 锚定段 toggle play / pause / resume
///   - F7 / F8: 锚定段在所属列表上下移动 (环形循环)
///     · 锚在 .learn → currentAudio.segments 列表
///     · 锚在 .review → 复习队列所有段扁平列表 (跨 audio, 跟 UI 显示顺序一致)
///   - Shift+F7: 倍速 -0.1 (clamp 到 0.5)
///   - Shift+F8: 倍速 +0.1 (clamp 到 1.5)
///
/// 字幕 toggle 用 F4 而非 F10 — F10/F11 在 Apple/Cherry 键盘上走 HID 媒体事件
/// (NSSystemDefined), CGEventTap 收不到, 即使加 Option 也不行。F4 实测能拦下。
///
/// F5/F6 作用对象 = AppState.englishAnchor (用户在 EnglishSegmentRow / EnglishReviewRow
/// 点 📌 按钮设置). 全局只有一个锚定 — 锚新段自动取消旧锚。
/// 没锚定时 F5/F6 NSSound.beep() 提示用户先回 Panel 锚定; F7/F8 不依赖锚定 (全局倍速)。
///
/// 用户场景: 焦点在 Typora 等其他 app, 一边写笔记一边精听, 不需要切回 Panel
/// 就能控制锚定段播放. 跟 F12 (录音) 共用 HotkeyTap.shared.
@MainActor
final class EnglishHotkeyController {
    static let shared = EnglishHotkeyController()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "english.hotkey")

    private weak var stateRef: AppState?
    private let player = SegmentPlayer.shared

    /// F5 seek 回退秒数.
    private let seekBackSec: Double = 5.0

    /// F7/F8 倍速调整范围, 跟 MiniPlayerControls picker 选项 0.5..1.5 一致.
    private let rateMinTimes10: Int = 5
    private let rateMaxTimes10: Int = 15

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
        HotkeyTap.shared.register(keyCode: HotkeyTap.f7KeyCode) { [weak self] in
            Task { @MainActor in self?.shiftAnchorSegment(step: -1) }
        }
        HotkeyTap.shared.register(keyCode: HotkeyTap.f8KeyCode) { [weak self] in
            Task { @MainActor in self?.shiftAnchorSegment(step: +1) }
        }
        HotkeyTap.shared.register(keyCode: HotkeyTap.f4KeyCode) { [weak self] in
            Task { @MainActor in self?.handleSubtitleToggle() }
        }
        HotkeyTap.shared.register(keyCode: HotkeyTap.f7KeyCode, modifiers: .maskShift) { [weak self] in
            Task { @MainActor in self?.adjustRate(steps: -1) }
        }
        HotkeyTap.shared.register(keyCode: HotkeyTap.f8KeyCode, modifiers: .maskShift) { [weak self] in
            Task { @MainActor in self?.adjustRate(steps: +1) }
        }
        HotkeyTap.shared.start()
        os_log("F4-F8 + Shift+F7/F8 hotkeys registered", log: Self.log, type: .info)
    }

    /// 解锁定段 → 实际 audio + segment + source. 三种 nil 路径:
    ///   - 没锚定 → nil
    ///   - 锚定指向 .learn 但 currentAudio / segments 不匹配 (audio 切了 / 段被改了) → nil
    ///   - 锚定指向 .review 但复习队列里没这条 (已 finish / mastered / 配额延后) → nil
    private func anchoredSegment() -> (audio: EnglishAudio, segment: EnglishSegment, source: PlaySource)? {
        guard let st = stateRef, let anchor = st.englishAnchor else { return nil }

        switch anchor.source {
        case .review:
            guard let rev = st.englishReviewToday else { return nil }
            for group in rev.groups {
                guard group.audio.id == anchor.audioID else { continue }
                if let item = group.items.first(where: { $0.segIdx == anchor.segIdx }) {
                    return (group.audio, item.segment, .review)
                }
            }
            return nil

        case .learn:
            guard let today = st.englishToday,
                  let audio = today.currentAudio,
                  audio.id == anchor.audioID,
                  !audio.localPath.isEmpty else { return nil }
            guard let seg = today.segments.first(where: { $0.idx == anchor.segIdx }) else {
                return nil
            }
            return (audio, seg, .learn)

        case .whole, .archive:
            // 这两个 source 不应出现在锚定中 (UI 没暴露锚定按钮); 防御性 nil
            return nil
        }
    }

    /// F6: 锚定段三态 toggle.
    private func handleF6() {
        guard let (audio, seg, source) = anchoredSegment() else {
            beepNoAnchor(action: "F6")
            return
        }
        let bound = player.isPlaying(audioID: audio.id, segIdx: seg.idx, source: source)
        let playing = bound && !player.paused
        if playing {
            os_log("F6: pause %{public}@ seg %d", log: Self.log, type: .info, source.rawValue, seg.idx)
            player.pause()
            return
        }
        if bound {
            os_log("F6: resume %{public}@ seg %d", log: Self.log, type: .info, source.rawValue, seg.idx)
            player.resume()
            return
        }
        os_log("F6: play %{public}@ seg %d", log: Self.log, type: .info, source.rawValue, seg.idx)
        let url = URL(fileURLWithPath: audio.localPath)
        let endMs = audio.durationMs > 0
            ? min(Int(audio.durationMs), Int(seg.endMs))
            : Int(seg.endMs)
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: seg.idx,
                source: source,
                startMs: max(0, Int(seg.startMs)),
                endMs: endMs
            )
        } catch {
            os_log("F6 play failed: %{public}@", log: Self.log, type: .error, String(describing: error))
        }
    }

    /// F5: 锚定段 seek -5s, clamp 到段开头. 仅在 player 已绑定该锚定段 (在播 / 暂停中) 时生效.
    private func handleF5() {
        guard let (audio, seg, source) = anchoredSegment() else {
            beepNoAnchor(action: "F5")
            return
        }
        guard player.isPlaying(audioID: audio.id, segIdx: seg.idx, source: source) else {
            os_log("F5: %{public}@ seg %d not bound, ignore", log: Self.log, type: .info, source.rawValue, seg.idx)
            return
        }
        let target = max(player.startSec, player.currentSec - seekBackSec)
        os_log("F5: seek %{public}@ seg %d to %.2fs", log: Self.log, type: .info, source.rawValue, seg.idx, target)
        player.seek(to: target)
    }

    /// F7 (steps=-1) / F8 (steps=+1): 倍速 ±0.1, clamp 到 [0.5, 1.5].
    /// 不依赖锚定 (rate 是全局设置, 任何时候都能调).
    /// 用整数中间值避浮点累积误差 — 否则 0.5+0.1+0.1+... 会变成 0.6000001 之类,
    /// MiniPlayerControls 的 Picker 用 Float tag 比较, 不相等就显示不出选中态.
    private func adjustRate(steps: Int) {
        let cur = Int(round(player.rate * 10))
        let next = max(rateMinTimes10, min(rateMaxTimes10, cur + steps))
        if next == cur { return }
        player.rate = Float(next) / 10.0
        os_log("rate adjust: %.1fx → %.1fx", log: Self.log, type: .info,
               Double(cur) / 10.0, Double(next) / 10.0)
    }

    /// F4: toggle 锚定段字幕显示. 没锚定 → beep。
    /// 改 state.anchorShowText, 锚定 row 用此值 reactive 切换 textArea 显示。
    private func handleSubtitleToggle() {
        guard let st = stateRef, st.englishAnchor != nil else {
            beepNoAnchor(action: "F4")
            return
        }
        st.anchorShowText.toggle()
        os_log("F4: anchorShowText → %{public}@", log: Self.log, type: .info,
               st.anchorShowText ? "show" : "hide")
    }

    /// Shift+F7 (step=-1) / Shift+F8 (step=+1): 锚定段在所属列表上下移动 (环形循环).
    ///
    /// 行为按 anchor.source 分支:
    ///   - .learn → currentAudio.segments 列表里 segIdx ±1, 越界绕回另一端
    ///   - .review → 复习队列所有段扁平 [(audioID, segIdx)] 列表里 ±1, 跨 audio 切;
    ///     越界绕回另一端 (整个列表是循环)
    ///
    /// no-op + beep:
    ///   - 没锚定
    ///   - 列表为空 (currentAudio 无 / segments 空 / review groups 空)
    ///   - 锚定段不在当前列表里 (audio 切了 / segments 重新生成 / review item 已 finish)
    ///   - source 是 .whole / .archive (UI 不暴露锚定那两个 source, 防御性 beep)
    ///
    /// 副作用: anchorShowText reset = false (新段从隐藏字幕开始); 不停当前播放。
    private func shiftAnchorSegment(step: Int) {
        guard let st = stateRef, let anchor = st.englishAnchor else {
            os_log("Shift+F7/8: no anchor, beep", log: Self.log, type: .info)
            NSSound.beep()
            return
        }

        switch anchor.source {
        case .learn:
            shiftLearnAnchor(st: st, anchor: anchor, step: step)
        case .review:
            shiftReviewAnchor(st: st, anchor: anchor, step: step)
        case .whole, .archive:
            os_log("Shift+F7/8: source=%{public}@ unsupported, beep", log: Self.log, type: .info, anchor.source.rawValue)
            NSSound.beep()
        }
    }

    private func shiftLearnAnchor(st: AppState, anchor: EnglishAnchor, step: Int) {
        guard let today = st.englishToday,
              let audio = today.currentAudio,
              audio.id == anchor.audioID,
              !today.segments.isEmpty else {
            NSSound.beep()
            return
        }
        guard let curIdx = today.segments.firstIndex(where: { $0.idx == anchor.segIdx }) else {
            NSSound.beep()
            return
        }
        let n = today.segments.count
        let nextIdx = ((curIdx + step) % n + n) % n  // 环形, 处理负数
        let nextSeg = today.segments[nextIdx]
        os_log("Shift+F7/8 [learn]: seg %d → seg %d", log: Self.log, type: .info,
               anchor.segIdx, nextSeg.idx)
        st.setEnglishAnchor(audioID: audio.id, segIdx: nextSeg.idx, source: .learn)
    }

    private func shiftReviewAnchor(st: AppState, anchor: EnglishAnchor, step: Int) {
        guard let rev = st.englishReviewToday else {
            NSSound.beep()
            return
        }
        // (audioID, segIdx) flatten — 跟 UI 显示顺序一致 (groups 顺序 + group 内 items 顺序)
        let flat: [(audioID: Int64, segIdx: Int)] = rev.groups.flatMap { group in
            group.items.map { (group.audio.id, $0.segIdx) }
        }
        guard !flat.isEmpty else {
            NSSound.beep()
            return
        }
        guard let curIdx = flat.firstIndex(where: {
            $0.audioID == anchor.audioID && $0.segIdx == anchor.segIdx
        }) else {
            NSSound.beep()
            return
        }
        let n = flat.count
        let nextIdx = ((curIdx + step) % n + n) % n
        let next = flat[nextIdx]
        os_log("Shift+F7/8 [review]: %lld:%d → %lld:%d", log: Self.log, type: .info,
               anchor.audioID, anchor.segIdx, next.audioID, next.segIdx)
        st.setEnglishAnchor(audioID: next.audioID, segIdx: next.segIdx, source: .review)
    }

    /// F5/F6/F4/Shift+F7/8 没锚定 / 锚定段失效 → 系统蜂鸣提示用户回 Panel 锚定。
    /// Typora 等前台 app 焦点下用户能立即听到反馈, 不需要切回 Panel 看 lastError。
    private func beepNoAnchor(action: String) {
        os_log("%{public}@: no anchor, beep", log: Self.log, type: .info, action)
        NSSound.beep()
    }
}
