import SwiftUI

/// 单条 segment 行: ▶/■ 按钮 + 序号 + (隐藏的) 文本 + ☐/☑ checkbox。
/// 精听场景文本默认隐藏 ("[ 点击显示字幕 ]"), 听一段不懂时点击该段才看字幕,
/// 听懂了直接勾 checkbox 不必看字幕。勾选后 POST /api/english/segments/check。
struct EnglishSegmentRow: View {
    let audio: EnglishAudio
    let segment: EnglishSegment
    let isDone: Bool

    @Environment(SegmentPlayer.self) private var player
    @Environment(AppState.self) private var state
    @State private var showText: Bool = false

    /// 段播放时前后各扩 N ms 听感缓冲, 抵消 ASR timestamp 偏差。
    /// Groq 词级 timestamp 已经精确, 暂置 0 看裸精度; 若末尾常吞词再加回 200-500ms。
    /// 第一段开头 clamp 到 0, 最后一段末尾 clamp 到 audio.durationMs (若已知)。
    private static let playbackPaddingMs: Int = 0

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .top, spacing: 12) {
                playButton
                Text("\(segment.idx + 1).")
                    .font(.title3.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .frame(width: 36, alignment: .trailing)
                textArea
                checkBox
            }
            HStack(spacing: 0) {
                Spacer().frame(width: 36 + 12 + 24)  // 对齐到文本起始位置
                MiniPlayerControls(placeholder: .init(
                    audioID: audio.id,
                    segIdx: segment.idx,
                    lengthSec: Double(segment.endMs - segment.startMs) / 1000.0
                ))
            }
        }
        .padding(.vertical, 6)
        .contentShape(Rectangle())
    }

    @ViewBuilder
    private var textArea: some View {
        Group {
            if showText {
                Text(segment.text)
                    .font(.title3)
                    .lineSpacing(2)
                    .textSelection(.enabled)
            } else {
                Text("[ 点击显示字幕 ]")
                    .font(.title3)
                    .foregroundStyle(.tertiary)
                    .italic()
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .onTapGesture { showText.toggle() }
        .help(showText ? "点击隐藏字幕" : "点击显示字幕")
    }

    @ViewBuilder
    private var checkBox: some View {
        Button {
            Task {
                await state.toggleEnglishSegment(
                    audioID: audio.id,
                    segment: segment,
                    currentlyDone: isDone
                )
            }
        } label: {
            Image(systemName: isDone ? "checkmark.square.fill" : "square")
                .font(.system(size: 20))
                .foregroundStyle(isDone ? Color.accentColor : Color.secondary)
        }
        .buttonStyle(.plain)
        .help(isDone ? "取消勾选" : "标记完成")
    }

    @ViewBuilder
    private var playButton: some View {
        let bound = player.isPlaying(audioID: audio.id, segIdx: segment.idx)
        let playing = bound && !player.paused
        Button {
            togglePlay(bound: bound, playing: playing)
        } label: {
            Image(systemName: playing ? "stop.circle.fill" : "play.circle.fill")
                .font(.system(size: 24))
                .foregroundStyle(playing ? Color.orange : Color.accentColor)
        }
        .buttonStyle(.plain)
        .disabled(audio.localPath.isEmpty)
        .help(audio.localPath.isEmpty ? "本地 mp3 不存在" : (playing ? "暂停" : (bound ? "继续" : "播放此句")))
    }

    /// pause / resume 三态: 在播 → pause 保留位置; 已绑定但暂停 → resume 接着播;
    /// 未绑定 → play 从段头开始。切到别段 ▶ 由 player.play() 内部 stop 旧段。
    private func togglePlay(bound: Bool, playing: Bool) {
        if playing {
            player.pause()
            return
        }
        if bound {
            player.resume()
            return
        }
        let url = URL(fileURLWithPath: audio.localPath)
        let pad = Self.playbackPaddingMs
        let expandedStart = max(0, Int(segment.startMs) - pad)
        let rawEnd = Int(segment.endMs) + pad
        let expandedEnd = audio.durationMs > 0
            ? min(Int(audio.durationMs), rawEnd)
            : rawEnd  // durationMs 未知时 AVAudioPlayer 自动到末尾停
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: segment.idx,
                startMs: expandedStart,
                endMs: expandedEnd
            )
        } catch {
            // 用户能看到 player 状态没变 (按钮不变 stop), 故意 silent
        }
    }
}
