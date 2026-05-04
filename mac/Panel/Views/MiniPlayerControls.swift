import SwiftUI

/// 段播放进度条 + 循环按钮 + 倍速 menu。挂在已 isPlaying(audioID, segIdx) 的位置下方。
/// 不持有 player 状态, 只读 SegmentPlayer 当前 currentSec / startSec / endSec。
/// seek / loop / rate 通过 SegmentPlayer 直接改 (单例)。
struct MiniPlayerControls: View {
    @Environment(SegmentPlayer.self) private var player

    /// 拖 slider 时锁定 currentSec 不被 timer 反向覆盖。
    @State private var dragging: Bool = false
    @State private var dragValue: Double = 0

    private static let rateOptions: [Float] = [0.5, 0.75, 1.0, 1.25, 1.5, 2.0]

    var body: some View {
        @Bindable var player = player
        HStack(spacing: 10) {
            Text(formatSec(displaySec - player.startSec))
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(width: 40, alignment: .trailing)

            Slider(
                value: Binding(
                    get: { dragging ? dragValue : displaySec },
                    set: { newVal in
                        dragValue = newVal
                        if !dragging { player.seek(to: newVal) }
                    }
                ),
                in: player.startSec...max(player.endSec, player.startSec + 0.1),
                onEditingChanged: { editing in
                    if editing {
                        dragging = true
                        dragValue = displaySec
                    } else {
                        player.seek(to: dragValue)
                        dragging = false
                    }
                }
            )

            Text(formatSec(player.endSec - player.startSec))
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(width: 40, alignment: .leading)

            Button {
                player.loop.toggle()
            } label: {
                Image(systemName: player.loop ? "repeat.circle.fill" : "repeat")
                    .font(.system(size: 16))
                    .foregroundStyle(player.loop ? Color.accentColor : Color.secondary)
            }
            .buttonStyle(.plain)
            .help(player.loop ? "循环已开" : "点击循环播放")

            Picker("", selection: $player.rate) {
                ForEach(Self.rateOptions, id: \.self) { r in
                    Text(String(format: "%.2gx", r)).tag(r)
                }
            }
            .pickerStyle(.menu)
            .frame(width: 70)
            .help("播放倍速")
        }
        .padding(.vertical, 4)
    }

    private var displaySec: Double {
        player.currentSec
    }

    private func formatSec(_ s: Double) -> String {
        let total = max(0, Int(s.rounded()))
        let m = total / 60
        let sec = total % 60
        return String(format: "%d:%02d", m, sec)
    }
}
