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
    /// 若停在段末尾 (progressTimer 触发末尾 pause 留下的位置), 自动 seek 回段头重播 —
    /// 否则 p.play() 立刻又被 progressTimer 检测末尾再次 pause, 死循环。
    func resume() {
        guard let p = player, !p.isPlaying else { return }
        if p.currentTime >= endSec - 0.05 {
            p.currentTime = startSec
            currentSec = startSec
        }
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

    /// 0.1s 轮询 currentTime → 更新进度 + 检测 endSec 边界 (loop 或 pause-at-end)。
    ///
    /// 只在 p.isPlaying 真在播时更新 currentSec + 检查边界。pause / 系统 interruption
    /// 让 p.isPlaying=false 时直接 no-op, 保留 currentSec 让 resume 接着上次位置播,
    /// 避免 AVAudioPlayer 内部 currentTime 重置时 UI 进度条被反向覆盖回 0。
    ///
    /// 段末尾自然结束: pause 但不 stop — 让用户用 F5 从末尾回退 5s 重听末尾片段。
    /// 用户按 F6 走 resume(), resume 内部检测末尾自动 seek 回段头重播。
    ///
    /// 不再用 dispatch-after 兜底 stop — 旧版按 segDur*1.5+5s 强制 stop, rate 改慢
    /// / audio session 被打断 / 系统级 pause 等场景下会在段没播完时提前触发 stop。
    private func startProgressTimer() {
        progressTimer?.invalidate()
        let t = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self, let p = self.player else { return }
                guard p.isPlaying else { return }
                self.currentSec = p.currentTime
                if p.currentTime >= self.endSec {
                    if self.loop {
                        p.currentTime = self.startSec
                        self.currentSec = self.startSec
                    } else {
                        // 段末尾自然结束 — 暂停留在 endSec, 不销毁 player
                        p.pause()
                        self.paused = true
                        self.currentSec = self.endSec  // clamp, 不让 slider 越界
                    }
                }
            }
        }
        progressTimer = t
        // Tolerance 大点不影响 0.1s 精度
        t.tolerance = 0.02
    }
}
