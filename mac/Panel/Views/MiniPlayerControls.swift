import SwiftUI

/// 段播放进度条 + 循环按钮 + 倍速 menu。挂在已 isPlaying(audioID, segIdx) 的位置下方。
/// 不持有 player 状态, 只读 SegmentPlayer 当前 currentSec / startSec / endSec。
/// seek / loop / rate 通过 SegmentPlayer 直接改 (单例)。
///
/// 传 placeholder 时支持「未播放也常驻显示」: player 不在播本段则 slider 静态显示
/// 段长 + 禁用, rate picker 始终可用 (rate 是单例全局态), 让用户进页面就能预设倍速。
struct MiniPlayerControls: View {
    @Environment(SegmentPlayer.self) private var player

    let placeholder: Placeholder?

    struct Placeholder {
        let audioID: Int64
        let segIdx: Int
        let source: PlaySource
        let lengthSec: Double
    }

    init(placeholder: Placeholder? = nil) {
        self.placeholder = placeholder
    }

    /// 拖 slider 时锁定 currentSec 不被 timer 反向覆盖。
    @State private var dragging: Bool = false
    @State private var dragValue: Double = 0

    private static let rateOptions: [Float] = [
        0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2, 1.3, 1.4, 1.5
    ]

    var body: some View {
        @Bindable var player = player
        let live = isLive
        let totalSec: Double = live
            ? max(0, player.endSec - player.startSec)
            : (placeholder?.lengthSec ?? 0)
        let curRel: Double = live ? max(0, displaySec - player.startSec) : 0

        HStack(spacing: 10) {
            Text(formatSec(curRel))
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(width: 40, alignment: .trailing)

            Slider(
                value: Binding(
                    get: {
                        guard live else { return 0 }
                        return dragging ? dragValue : displaySec
                    },
                    set: { newVal in
                        guard live else { return }
                        dragValue = newVal
                        if !dragging { player.seek(to: newVal) }
                    }
                ),
                in: live
                    ? player.startSec...max(player.endSec, player.startSec + 0.1)
                    : 0...max(0.1, totalSec),
                onEditingChanged: { editing in
                    guard live else { return }
                    if editing {
                        dragging = true
                        dragValue = displaySec
                    } else {
                        player.seek(to: dragValue)
                        dragging = false
                    }
                }
            )
            .disabled(!live)

            Text(formatSec(totalSec))
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

    private var isLive: Bool {
        guard let p = placeholder else { return true }
        return player.isPlaying(audioID: p.audioID, segIdx: p.segIdx, source: p.source)
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
