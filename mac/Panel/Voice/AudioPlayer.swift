import AVFoundation
import Foundation
import Observation

/// 单例的"段播放器"。同一时刻只放一个 segment (或整段, segIdx = .integerMin)。
/// 不切割实际 mp3 文件 — AVAudioPlayer 加载整文件, 设 currentTime 跳到区间起点,
/// 0.1s timer 检测 currentTime >= endSec 触发 stop / loop。
/// 支持 seek (Slider 拖动) / loop (循环) / rate (0.5-2.0 倍速)。
@Observable
@MainActor
final class SegmentPlayer {
    static let shared = SegmentPlayer()

    /// 整段播放占位 segIdx (T4 主音频整段播用)。
    static let wholeAudioSegIdx: Int = .min

    private(set) var playingAudioID: Int64?
    private(set) var playingSegIdx: Int?

    /// 当前播放进度 (绝对 audio 秒数, 不是相对段起点)。
    private(set) var currentSec: Double = 0
    /// 当前播放区间 [startSec, endSec)。
    private(set) var startSec: Double = 0
    private(set) var endSec: Double = 0
    /// 是否暂停中 (player 存在但未播放)。
    private(set) var paused: Bool = false

    /// 倍速 0.5 - 2.0, 实时改 player.rate 立即生效。
    var rate: Float = 1.0 {
        didSet {
            guard let p = player else { return }
            p.rate = rate
        }
    }
    /// 循环播放: 播到 endSec 时回 startSec 重播 (不 stop)。
    var loop: Bool = false

    private var player: AVAudioPlayer?
    private var progressTimer: Timer?

    private init() {}

    /// 是否正在播放给定 audio 的给定 segment (含暂停态)。
    func isPlaying(audioID: Int64, segIdx: Int) -> Bool {
        playingAudioID == audioID && playingSegIdx == segIdx
    }

    /// 开始播放区间 [startMs, endMs) of url。已在播则先 stop。
    /// segIdx = SegmentPlayer.wholeAudioSegIdx 表示整段播放 (T4)。
    func play(url: URL, audioID: Int64, segIdx: Int, startMs: Int, endMs: Int) throws {
        stop()
        guard endMs > startMs else { return }
        let p = try AVAudioPlayer(contentsOf: url)
        p.enableRate = true
        p.rate = rate
        p.prepareToPlay()
        p.currentTime = Double(startMs) / 1000.0
        guard p.play() else {
            throw NSError(domain: "SegmentPlayer", code: 1,
                          userInfo: [NSLocalizedDescriptionKey: "AVAudioPlayer.play() returned false"])
        }

        startSec = Double(startMs) / 1000.0
        endSec = Double(endMs) / 1000.0
        currentSec = startSec
        playingAudioID = audioID
        playingSegIdx = segIdx
        paused = false
        player = p

        startProgressTimer()
    }

    /// 拖进度条调用 — seek 到 sec (clamp 到 [startSec, endSec])。
    func seek(to sec: Double) {
        guard let p = player else { return }
        let target = max(startSec, min(endSec, sec))
        p.currentTime = target
        currentSec = target
    }

    /// 暂停, 不释放 player (resume 接着播)。
    func pause() {
        guard let p = player, p.isPlaying else { return }
        p.pause()
        paused = true
    }

    /// 暂停后恢复播放。
    func resume() {
        guard let p = player, !p.isPlaying else { return }
        p.play()
        paused = false
    }

    /// 立刻停止 + 释放。重置状态。
    func stop() {
        progressTimer?.invalidate()
        progressTimer = nil
        player?.stop()
        player = nil
        playingAudioID = nil
        playingSegIdx = nil
        currentSec = 0
        startSec = 0
        endSec = 0
        paused = false
    }

    // MARK: - private

    /// 0.1s 轮询 currentTime → 更新进度 + 检测 endSec 边界 (loop 或 stop)。
    ///
    /// 不再用 dispatch-after 兜底 stop — 旧版按 segDur*1.5+5s 强制 stop, rate 改慢
    /// / AVAudioPlayer audio session 被打断 / 系统级 pause 等场景下会在段没播完时
    /// 提前触发 stop, 表现为「播一半进度突然回到 0」。timer 是 main runloop 稳定调度,
    /// 漏触发的极端场景 cost 远小于"无故打断用户"的 cost。
    private func startProgressTimer() {
        progressTimer?.invalidate()
        let t = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self, let p = self.player else { return }
                self.currentSec = p.currentTime
                if p.currentTime >= self.endSec {
                    if self.loop {
                        p.currentTime = self.startSec
                        self.currentSec = self.startSec
                    } else {
                        self.stop()
                    }
                }
            }
        }
        progressTimer = t
        // Tolerance 大点不影响 0.1s 精度
        t.tolerance = 0.02
    }
}
