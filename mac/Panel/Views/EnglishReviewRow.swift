import SwiftUI

/// 单条复习行: ▶ 播放 / 序号 / 字幕区 (默认隐藏) / 完成 / 延后 / 跳过.
///
/// 跟 EnglishSegmentRow 的关键差别 - 这里不是切换 segment 勾选 (那会污染 records 表 +
/// 重置 stage), 而是对 review_queue 行的 stage 推进 / 延后 / 永久跳过.
///
/// 完成 / 跳过弹 confirmationDialog (默认行为 Enter 触发主按钮); 延后无确认 (高频低风险).
///
/// item.doneToday = 今天已复习完的行. 不从队列消失, 就地灰掉 + 收起三个操作按钮
/// 换成「已完成」标记; 播放 / 锚定 / 看字幕仍可用 (复习完还想再听一遍很常见).
struct EnglishReviewRow: View {
    let audio: EnglishAudio
    let item: EnglishReviewItem

    @Environment(SegmentPlayer.self) private var player
    @Environment(AppState.self) private var state
    @State private var showText = false
    @State private var showCompleteConfirm = false
    @State private var showFinishConfirm = false

    /// 段播放时前后扩 N ms (跟 EnglishSegmentRow 同字段). 0 = 用 ASR 裸 timestamp。
    private static let playbackPaddingMs: Int = 0

    var body: some View {
        let bound = player.isPlaying(audioID: audio.id, segIdx: item.segment.idx, source: .review)
        let playing = bound && !player.paused
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .top, spacing: 12) {
                playButton(bound: bound, playing: playing)
                anchorButton
                Text("\(item.segment.idx + 1).")
                    .font(.title3.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .frame(width: 36, alignment: .trailing)
                textArea
                    .opacity(item.doneToday ? 0.55 : 1)
                if item.doneToday {
                    doneBadge
                } else {
                    completeButton
                    postponeButton
                    finishButton
                }
            }
            stageMetaRow
            if bound {
                HStack(spacing: 0) {
                    Spacer().frame(width: 36 + 12 + 24)
                    MiniPlayerControls()
                }
            }
        }
        .padding(.vertical, 6)
        .contentShape(Rectangle())
        .confirmationDialog(
            "标记本段复习完成?", isPresented: $showCompleteConfirm
        ) {
            Button("完成") {
                Task { await state.reviewMarkDone(itemID: item.id) }
            }
            .keyboardShortcut(.defaultAction)
            Button("取消", role: .cancel) {}
        } message: {
            Text(stageProgressText)
        }
        .confirmationDialog(
            "永久跳过这一段?", isPresented: $showFinishConfirm
        ) {
            Button("跳过", role: .destructive) {
                Task { await state.reviewFinish(itemID: item.id) }
            }
            .keyboardShortcut(.defaultAction)
            Button("取消", role: .cancel) {}
        } message: {
            Text("这一段不会再出现在复习队列中.")
        }
    }

    /// 锚定段 = 用 state.anchorShowText (F10 全局切换 + 点击同步), 非锚定段 = 本地 @State。
    private var isAnchored: Bool {
        state.englishAnchor == EnglishAnchor(audioID: audio.id, segIdx: item.segment.idx, source: .review)
    }

    private var displayShowText: Bool {
        isAnchored ? state.anchorShowText : showText
    }

    private func toggleShowText() {
        if isAnchored {
            state.anchorShowText.toggle()
        } else {
            showText.toggle()
        }
    }

    @ViewBuilder
    private var textArea: some View {
        let shown = displayShowText
        Group {
            if shown {
                Text(item.segment.text)
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
        .onTapGesture { toggleShowText() }
        .help(shown ? "点击隐藏字幕" : "点击显示字幕")
    }

    @ViewBuilder
    private var anchorButton: some View {
        let isAnchored = state.englishAnchor == EnglishAnchor(
            audioID: audio.id, segIdx: item.segment.idx, source: .review
        )
        Button {
            state.toggleEnglishAnchor(audioID: audio.id, segIdx: item.segment.idx, source: .review)
        } label: {
            Image(systemName: isAnchored ? "pin.fill" : "pin")
                .font(.system(size: 16))
                .foregroundStyle(isAnchored ? Color.orange : Color.secondary.opacity(0.5))
                .rotationEffect(.degrees(isAnchored ? 0 : 30))
        }
        .buttonStyle(.plain)
        .help(isAnchored ? "已锚定 (F5/F6 作用此段) · 点击取消" : "锚定为 F5/F6 操作目标")
    }

    @ViewBuilder
    private func playButton(bound: Bool, playing: Bool) -> some View {
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

    /// 已完成行的右侧标记, 占掉原来三个按钮的位置。
    @ViewBuilder
    private var doneBadge: some View {
        Label("已完成", systemImage: "checkmark.circle.fill")
            .labelStyle(.titleAndIcon)
            .font(.subheadline)
            .foregroundStyle(.green)
            .help("今天已复习过这一段, 明天起才会再进队列")
    }

    @ViewBuilder
    private var completeButton: some View {
        Button {
            showCompleteConfirm = true
        } label: {
            Label("完成", systemImage: "checkmark.circle.fill")
                .labelStyle(.titleAndIcon)
        }
        .buttonStyle(.borderedProminent)
        .tint(.green)
        .help("已记住这一段, 推进到下一阶段")
    }

    @ViewBuilder
    private var postponeButton: some View {
        Button {
            Task { await state.reviewPostpone(itemID: item.id) }
        } label: {
            Image(systemName: "clock.arrow.circlepath")
                .font(.system(size: 16))
        }
        .buttonStyle(.bordered)
        .help("延后 1 天")
    }

    @ViewBuilder
    private var finishButton: some View {
        Button(role: .destructive) {
            showFinishConfirm = true
        } label: {
            Image(systemName: "forward.end.fill")
                .font(.system(size: 14))
        }
        .buttonStyle(.bordered)
        .help("永久跳过 (二次确认)")
    }

    @ViewBuilder
    private var stageMetaRow: some View {
        HStack(spacing: 8) {
            Spacer().frame(width: 36 + 12 + 24)
            Text(item.doneToday ? "已完成 · \(stageProgressText)" : stageProgressText)
                .font(.caption)
                .foregroundStyle(item.doneToday ? Color.green : stageColor)
            if item.overdueDays > 0 && !item.doneToday {
                Text("过去第 \(item.overdueDays) 天")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
            if item.postponedCount > 0 {
                Text("已延后 \(item.postponedCount) 次")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
        }
    }

    /// stage 0..4 对应 1/3/7/14/30 天复习
    private var stageProgressText: String {
        let intervals = [1, 3, 7, 14, 30]
        let nextStage = item.stage + 1
        if nextStage > intervals.count {
            return "已掌握"
        }
        let dayLabel = intervals[item.stage]
        return "stage \(nextStage)/5 · \(dayLabel) 天复习"
    }

    private var stageColor: Color {
        switch item.stage {
        case 0: return .blue
        case 1: return .teal
        case 2: return .green
        case 3: return .yellow
        case 4: return .orange
        default: return .secondary
        }
    }

    /// pause / resume 三态: 在播 → pause 保留位置; 已绑定但暂停 → resume; 未绑定 → play 段头。
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
        let expandedStart = max(0, Int(item.segment.startMs) - pad)
        let rawEnd = Int(item.segment.endMs) + pad
        let expandedEnd = audio.durationMs > 0
            ? min(Int(audio.durationMs), rawEnd)
            : rawEnd
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: item.segment.idx,
                source: .review,
                startMs: expandedStart,
                endMs: expandedEnd
            )
        } catch {
            // silent
        }
    }
}
