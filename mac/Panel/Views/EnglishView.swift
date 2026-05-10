import SwiftUI

/// M4 占位实现: 仅显示当前音频元数据 + 各窗口状态 + 今日累计。
/// M5 接入 ASR + AVAudioPlayer + segment 列表 + ▶ / ☐ 交互。
struct EnglishView: View {
    @Environment(AppState.self) private var state
    @Environment(SegmentPlayer.self) private var player
    @State private var showArchive = false
    @State private var showSettings = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                HStack {
                    Text("英文练习")
                        .font(.largeTitle.weight(.semibold))
                    Spacer()
                    Button {
                        showSettings = true
                    } label: {
                        Label("设置", systemImage: "gearshape")
                    }
                    .buttonStyle(.bordered)
                    .disabled(state.englishSettings == nil)
                    Button {
                        showArchive = true
                    } label: {
                        Label("历史档案", systemImage: "archivebox")
                    }
                    .buttonStyle(.bordered)
                }

                if let today = state.englishToday {
                    headerStats(today: today)
                    Divider()
                    windowsSection(today: today)
                    Divider()
                    if let review = state.englishReviewToday {
                        EnglishReviewSection(review: review)
                            .environment(state)
                            .environment(player)
                        Divider()
                    }
                    audioSection(today: today)
                } else {
                    ProgressView("加载中…")
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding(.top, 40)
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .sheet(isPresented: $showArchive) {
            EnglishArchiveSheet(isPresented: $showArchive)
        }
        .sheet(isPresented: $showSettings) {
            if let es = state.englishSettings {
                EnglishSettingsSheet(initial: es).environment(state)
            }
        }
        .sheet(isPresented: Binding(
            get: { state.showSkipSheet },
            set: { newValue in
                if !newValue { state.cancelSkipSheet() }
            }
        )) {
            EnglishSkipSheet().environment(state)
        }
        .toolbar {
            ToolbarItem { JumpToFocusButton() }
        }
    }

    @ViewBuilder
    private func headerStats(today: EnglishToday) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 12) {
                Text(String(format: "今日新学 %.1f /", today.totalMinutes))
                    .font(.title2.weight(.semibold))
                ThresholdEditor(currentMin: today.thresholdMin)
                Text("分钟")
                    .font(.title2.weight(.semibold))
                if today.todayDone {
                    Text("完成 ✓")
                        .font(.headline)
                        .foregroundStyle(.green)
                }
                Spacer()
                reviewSummaryBadge(today: today)
            }
            ProgressView(
                value: min(today.totalMinutes, Double(today.thresholdMin)),
                total: Double(today.thresholdMin)
            )
            .tint(today.todayDone ? .green : .accentColor)
        }
    }

    /// 复习摘要 (跟新学时长并列). 后端 issue-44 起返回; 老 backend 缺字段时这些值都是 0,
    /// 仍渲染 "复习 0/0" 不刺眼 (不专门 hide 简化判断)。
    @ViewBuilder
    private func reviewSummaryBadge(today: EnglishToday) -> some View {
        let limit = max(today.reviewDailyLimit, 1)
        let done = today.reviewDoneToday
        let due = today.reviewDueTotal
        HStack(spacing: 6) {
            Image(systemName: "arrow.triangle.2.circlepath")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text("复习 \(done) / \(limit) 条")
                .font(.body.monospacedDigit())
                .foregroundStyle(.secondary)
            if due > 0 {
                Text("(\(due) 待)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.orange)
            }
        }
    }

    @ViewBuilder
    private func windowsSection(today: EnglishToday) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("时段窗口").font(.headline)
            if today.windows.isEmpty {
                Text("(未配置窗口)").foregroundStyle(.secondary)
            } else {
                ForEach(today.windows) { w in
                    HStack(spacing: 12) {
                        Text("\(w.startHM)–\(w.endHM)")
                            .font(.body.monospacedDigit())
                            .frame(width: 110, alignment: .leading)
                        statusChip(status: w.status)
                        Text(String(format: "%.1f 分钟", w.minutesDone))
                            .font(.body.monospacedDigit())
                            .foregroundStyle(.secondary)
                        if w.reached {
                            Text("达到阈值 ✓").font(.body).foregroundStyle(.green)
                        }
                        if w.snoozeCount > 0 {
                            Text("snooze ×\(w.snoozeCount)")
                                .font(.caption).foregroundStyle(.orange)
                        }
                        Spacer()
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func audioSection(today: EnglishToday) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("当前音频").font(.headline)
                Spacer()
                if today.currentAudio != nil {
                    Button {
                        state.openSkipSheet()
                    } label: {
                        Label("跳过", systemImage: "forward.fill")
                    }
                    .buttonStyle(.bordered)
                    .help("归档当前音频, 选下一个 (按主题抽奖)")
                }
            }
            if let a = today.currentAudio {
                VStack(alignment: .leading, spacing: 6) {
                    Text(a.title).font(.title3.weight(.medium)).lineLimit(2)
                    HStack(spacing: 12) {
                        Text("[\(a.source.uppercased())]")
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                        Text("时长 \(formatMs(a.durationMs))")
                            .font(.caption).foregroundStyle(.secondary)
                        Text("段数 \(today.segments.count)")
                            .font(.caption).foregroundStyle(.secondary)
                        statusChip(status: a.segmentsStatus)
                    }
                    if !a.localPath.isEmpty {
                        Text(a.localPath)
                            .font(.caption2.monospaced())
                            .foregroundStyle(.tertiary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    } else {
                        Text("(本地未下载, 等待 backend fetch)")
                            .font(.caption).foregroundStyle(.orange)
                    }
                    if !a.segmentsFailedReason.isEmpty {
                        Text("ASR 失败: \(a.segmentsFailedReason)")
                            .font(.caption).foregroundStyle(.red)
                    }
                }

                wholeAudioPlayer(audio: a)
                segmentsList(audio: a, today: today)
            } else {
                Text("没有可用音频 (等待 backend fetch)")
                    .foregroundStyle(.secondary)
            }
        }
    }

    /// 整段音频播放器 (T4) — 跟 segment 播放共用 SegmentPlayer 单例。
    /// 用 segIdx = SegmentPlayer.wholeAudioSegIdx 区分整段播。
    @ViewBuilder
    private func wholeAudioPlayer(audio: EnglishAudio) -> some View {
        if !audio.localPath.isEmpty, audio.durationMs > 0 {
            let bound = player.isPlaying(audioID: audio.id, segIdx: SegmentPlayer.wholeAudioSegIdx)
            let playing = bound && !player.paused
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 12) {
                    Button {
                        toggleWholePlay(audio: audio, bound: bound, playing: playing)
                    } label: {
                        Image(systemName: playing ? "stop.circle.fill" : "play.circle.fill")
                            .font(.system(size: 28))
                            .foregroundStyle(playing ? Color.orange : Color.accentColor)
                    }
                    .buttonStyle(.plain)
                    .help(playing ? "暂停" : (bound ? "继续" : "整段播放 (无字幕)"))
                    Text("整段播放")
                        .font(.body.weight(.medium))
                        .foregroundStyle(.secondary)
                    Spacer()
                }
                if bound {
                    MiniPlayerControls()
                }
            }
            .padding(10)
            .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 8))
        }
    }

    /// pause / resume 三态: 在播 → pause; 已绑定但暂停 → resume; 未绑定 → play 从 0 开始。
    private func toggleWholePlay(audio: EnglishAudio, bound: Bool, playing: Bool) {
        if playing {
            player.pause()
            return
        }
        if bound {
            player.resume()
            return
        }
        let url = URL(fileURLWithPath: audio.localPath)
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: SegmentPlayer.wholeAudioSegIdx,
                startMs: 0,
                endMs: Int(audio.durationMs)
            )
        } catch {
            // silent
        }
    }

    @ViewBuilder
    private func segmentsList(audio: EnglishAudio, today: EnglishToday) -> some View {
        if today.segments.isEmpty {
            switch audio.segmentsStatus {
            case "ready":
                Text("无段落数据").foregroundStyle(.secondary).font(.caption)
            case "running":
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("正在分析音频…").foregroundStyle(.secondary).font(.caption)
                }
            case "failed":
                Text("分析失败,见上行 reason").foregroundStyle(.red).font(.caption)
            default:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("等待 ASR 启动…").foregroundStyle(.secondary).font(.caption)
                }
            }
        } else {
            let doneSet = Set(today.doneSegIdx)
            VStack(alignment: .leading, spacing: 0) {
                Divider().padding(.vertical, 4)
                Text("段落 (\(today.segments.count))").font(.headline)
                ForEach(today.segments) { seg in
                    EnglishSegmentRow(audio: audio, segment: seg, isDone: doneSet.contains(seg.idx))
                }
            }
        }
    }

    @ViewBuilder
    private func statusChip(status: String) -> some View {
        let (label, color) = statusChipStyle(status)
        Text(label)
            .font(.caption.weight(.medium))
            .padding(.horizontal, 8)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }

    private func statusChipStyle(_ status: String) -> (String, Color) {
        switch status {
        case "active":   return ("进行中", .green)
        case "upcoming": return ("待开始", .blue)
        case "passed":   return ("已过", .gray)
        case "ready":    return ("就绪", .green)
        case "pending":  return ("待解析", .orange)
        case "running":  return ("分析中", .blue)
        case "failed":   return ("失败", .red)
        default:         return (status, .gray)
        }
    }

    private func formatMs(_ ms: Int64) -> String {
        let total = Int(ms / 1000)
        let m = total / 60
        let s = total % 60
        return String(format: "%d:%02d", m, s)
    }
}

/// inline editor: 显示阈值数字, 点击切到 TextField, 失焦/回车提交。
private struct ThresholdEditor: View {
    let currentMin: Int
    @Environment(AppState.self) private var state

    @State private var editing = false
    @State private var draft: String = ""
    @FocusState private var focused: Bool

    var body: some View {
        if editing {
            TextField("", text: $draft)
                .font(.title2.weight(.semibold).monospacedDigit())
                .frame(width: 50)
                .multilineTextAlignment(.trailing)
                .textFieldStyle(.roundedBorder)
                .focused($focused)
                .onAppear {
                    draft = String(currentMin)
                    focused = true
                }
                .onSubmit { commit() }
                .onChange(of: focused) { _, new in
                    if !new { commit() }
                }
        } else {
            Text("\(currentMin)")
                .font(.title2.weight(.semibold).monospacedDigit())
                .underline(true, color: .secondary.opacity(0.4))
                .onTapGesture {
                    draft = String(currentMin)
                    editing = true
                }
                .help("点击修改今日阈值")
        }
    }

    private func commit() {
        defer { editing = false }
        guard let v = Int(draft.trimmingCharacters(in: .whitespaces)),
              v > 0, v <= 240,
              v != currentMin else { return }
        Task { await state.setEnglishThresholdMin(v) }
    }
}
