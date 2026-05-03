import SwiftUI

/// 单条 segment 行: ▶/■ 按钮 + 序号 + 文本 + ☐ checkbox。
/// M5 接区间播放 (▶);M6 才接 ☐ 勾选。
struct EnglishSegmentRow: View {
    let audio: EnglishAudio
    let segment: EnglishSegment
    let isDone: Bool

    @Environment(SegmentPlayer.self) private var player

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            playButton
            Text("\(segment.idx + 1).")
                .font(.body.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(width: 32, alignment: .trailing)
            Text(segment.text)
                .font(.body)
                .frame(maxWidth: .infinity, alignment: .leading)
                .textSelection(.enabled)
            Image(systemName: isDone ? "checkmark.square.fill" : "square")
                .font(.system(size: 16))
                .foregroundStyle(isDone ? Color.accentColor : Color.secondary)
                .help("M6 才能勾选")
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
    }

    @ViewBuilder
    private var playButton: some View {
        let playing = player.isPlaying(audioID: audio.id, segIdx: segment.idx)
        Button {
            togglePlay(playing: playing)
        } label: {
            Image(systemName: playing ? "stop.circle.fill" : "play.circle.fill")
                .font(.system(size: 20))
                .foregroundStyle(playing ? Color.orange : Color.accentColor)
        }
        .buttonStyle(.plain)
        .disabled(audio.localPath.isEmpty)
        .help(audio.localPath.isEmpty ? "本地 mp3 不存在" : "播放此句")
    }

    private func togglePlay(playing: Bool) {
        if playing {
            player.stop()
            return
        }
        let url = URL(fileURLWithPath: audio.localPath)
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: segment.idx,
                startMs: Int(segment.startMs),
                endMs: Int(segment.endMs)
            )
        } catch {
            // 用户能看到 player 状态没变 (按钮不变 stop), 故意 silent
        }
    }
}
