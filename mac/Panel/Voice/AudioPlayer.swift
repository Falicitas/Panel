import AVFoundation
import Foundation
import Observation

/// 单例的"段播放器"。同一时刻只放一个 segment。
/// 不切割实际 mp3 文件 — 用 AVAudioPlayer 设 currentTime 到 startMs, 然后定时器
/// 在 (endMs - startMs) 毫秒后 stop。整个文件被加载到内存或按需流读由 AVFoundation 决定。
@Observable
@MainActor
final class SegmentPlayer {
    static let shared = SegmentPlayer()

    private(set) var playingAudioID: Int64?
    private(set) var playingSegIdx: Int?

    private var player: AVAudioPlayer?
    private var stopWork: DispatchWorkItem?

    private init() {}

    /// 是否正在播放给定 audio 的给定 segment。
    func isPlaying(audioID: Int64, segIdx: Int) -> Bool {
        playingAudioID == audioID && playingSegIdx == segIdx
    }

    /// 开始播放区间 [startMs, endMs) of url。已在播则先 stop。
    func play(url: URL, audioID: Int64, segIdx: Int, startMs: Int, endMs: Int) throws {
        stop()
        guard endMs > startMs else { return }
        let p = try AVAudioPlayer(contentsOf: url)
        p.prepareToPlay()
        p.currentTime = Double(startMs) / 1000.0
        guard p.play() else {
            throw NSError(domain: "SegmentPlayer", code: 1,
                          userInfo: [NSLocalizedDescriptionKey: "AVAudioPlayer.play() returned false"])
        }
        let durationSec = Double(endMs - startMs) / 1000.0
        let work = DispatchWorkItem { [weak self] in
            guard let self else { return }
            self.player?.stop()
            self.player = nil
            self.playingAudioID = nil
            self.playingSegIdx = nil
            self.stopWork = nil
        }
        stopWork = work
        DispatchQueue.main.asyncAfter(deadline: .now() + durationSec, execute: work)
        player = p
        playingAudioID = audioID
        playingSegIdx = segIdx
    }

    /// 立刻停止当前播放。
    func stop() {
        stopWork?.cancel()
        stopWork = nil
        player?.stop()
        player = nil
        playingAudioID = nil
        playingSegIdx = nil
    }
}
