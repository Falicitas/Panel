import SwiftUI

/// M4 占位实现: 仅显示当前音频元数据 + 各窗口状态 + 今日累计。
/// M5 接入 ASR + AVAudioPlayer + segment 列表 + ▶ / ☐ 交互。
struct EnglishView: View {
    @Environment(AppState.self) private var state
    @State private var showArchive = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                HStack {
                    Text("英文练习")
                        .font(.largeTitle.weight(.semibold))
                    Spacer()
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
    }

    @ViewBuilder
    private func headerStats(today: EnglishToday) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 12) {
                Text(String(format: "今日 %.1f / %d 分钟", today.totalMinutes, today.thresholdMin))
                    .font(.title2.weight(.semibold))
                if today.todayDone {
                    Text("完成 ✓")
                        .font(.headline)
                        .foregroundStyle(.green)
                }
            }
            ProgressView(
                value: min(today.totalMinutes, Double(today.thresholdMin)),
                total: Double(today.thresholdMin)
            )
            .tint(today.todayDone ? .green : .accentColor)
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
            Text("当前音频").font(.headline)
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

                segmentsList(audio: a, today: today)
            } else {
                Text("没有可用音频 (等待 backend fetch)")
                    .foregroundStyle(.secondary)
            }
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
