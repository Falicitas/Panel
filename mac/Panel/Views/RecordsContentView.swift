import SwiftUI
import Charts

enum RecordsTab: String, CaseIterable, Identifiable {
    case grip
    case supplement

    var id: String { rawValue }

    var label: String {
        switch self {
        case .grip:       "晨起握力"
        case .supplement: "保健品月度"
        }
    }
}

struct RecordsContentView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        @Bindable var state = state
        VStack(spacing: 0) {
            Picker("", selection: $state.recordsTab) {
                ForEach(RecordsTab.allCases) { t in
                    Text(t.label).tag(t)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            Divider()
            switch state.recordsTab {
            case .grip:       GripPanel()
            case .supplement: SupplementMonthlyPanel()
            }
        }
        .navigationTitle("记录")
    }
}

// MARK: - 握力

private struct GripPanel: View {
    @Environment(AppState.self) private var state

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                GripEntryCard()
                GripClassificationCard()
                GripChartCard()
            }
            .padding(16)
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }
}

private struct GripEntryCard: View {
    @Environment(AppState.self) private var state
    @State private var leftText = ""
    @State private var rightText = ""
    @State private var note = ""
    @State private var saving = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("今日录入").font(.headline)
                Spacer()
                if let t = state.grip?.today {
                    Text("已录: 左 \(format(t.left)) · 右 \(format(t.right)) · 平均 \(format(t.avg))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Text("今日未录").font(.caption).foregroundStyle(.tertiary)
                }
            }
            HStack(spacing: 12) {
                LabeledKgField(label: "左手", text: $leftText)
                LabeledKgField(label: "右手", text: $rightText)
            }
            TextField("备注 (选填)", text: $note)
                .textFieldStyle(.roundedBorder)
            HStack {
                if saving { ProgressView().scaleEffect(0.7) }
                Spacer()
                Button {
                    save()
                } label: {
                    Label("保存今日握力", systemImage: "checkmark.circle.fill")
                }
                .buttonStyle(.borderedProminent)
                .disabled(saving || !canSave)
            }
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
        .onAppear { primeFromToday() }
        .onChange(of: state.grip?.today) { _, _ in primeFromToday() }
    }

    private var canSave: Bool {
        let l = Double(leftText) ?? 0
        let r = Double(rightText) ?? 0
        return l > 0 || r > 0
    }

    private func primeFromToday() {
        guard let t = state.grip?.today else { return }
        if leftText.isEmpty { leftText = trimZero(t.left) }
        if rightText.isEmpty { rightText = trimZero(t.right) }
    }

    private func save() {
        let l = Double(leftText) ?? 0
        let r = Double(rightText) ?? 0
        guard l > 0 || r > 0 else { return }
        saving = true
        Task {
            await state.saveGrip(date: todayString(), left: l, right: r, note: note)
            saving = false
        }
    }

    private func todayString() -> String {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: Date())
    }

    private func format(_ v: Double) -> String { String(format: "%.1f", v) + " kg" }

    private func trimZero(_ v: Double) -> String {
        if v == v.rounded() { return String(Int(v)) }
        return String(format: "%.1f", v)
    }
}

private struct LabeledKgField: View {
    let label: String
    @Binding var text: String

    var body: some View {
        HStack(spacing: 6) {
            Text(label).font(.caption).foregroundStyle(.secondary)
                .frame(width: 36, alignment: .leading)
            TextField("kg", text: $text)
                .textFieldStyle(.roundedBorder)
                .multilineTextAlignment(.trailing)
                .frame(maxWidth: .infinity)
                .monospacedDigit()
            Text("kg").font(.caption).foregroundStyle(.tertiary)
        }
    }
}

private struct GripClassificationCard: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("分类").font(.headline)
            HStack(spacing: 12) {
                Image(systemName: icon)
                    .font(.system(size: 28))
                    .foregroundStyle(color)
                VStack(alignment: .leading, spacing: 2) {
                    Text(state.grip?.label ?? "—").font(.title3.weight(.medium))
                    Text(detailLine).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
        }
        .padding(16)
        .background(color.opacity(0.10), in: RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(color.opacity(0.35), lineWidth: 1)
        )
    }

    private var icon: String {
        switch state.grip?.classification {
        case "rest":         "bed.double.fill"
        case "reduce":       "arrow.down.circle.fill"
        case "normal":       "checkmark.seal.fill"
        case "no_data":      "questionmark.circle"
        default:             "hourglass"
        }
    }

    private var color: Color {
        switch state.grip?.classification {
        case "rest":   .red
        case "reduce": .orange
        case "normal": .green
        default:       .gray
        }
    }

    private var detailLine: String {
        guard let g = state.grip else { return "—" }
        if g.baselineDays < 3 {
            return "baseline 还需 \(max(0, 3 - g.baselineDays)) 天 (已 \(g.baselineDays))"
        }
        let m = String(format: "%.1f", g.baselineM)
        let sd = String(format: "%.2f", g.sd)
        let z = String(format: "%+.2f", g.deviation)
        return "M=\(m) · SD=\(sd) · z=\(z) · baseline \(g.baselineDays) 天"
    }
}

private struct GripChartCard: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("最近 14 天").font(.headline)
                Spacer()
                if let g = state.grip, g.history.isEmpty {
                    Text("无数据").font(.caption).foregroundStyle(.tertiary)
                }
            }
            chart
                .frame(height: 200)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }

    @ViewBuilder
    private var chart: some View {
        if let g = state.grip, !g.history.isEmpty {
            Chart {
                ForEach(g.history) { d in
                    if let date = parseDate(d.date) {
                        LineMark(
                            x: .value("日期", date),
                            y: .value("平均", d.avg)
                        )
                        .interpolationMethod(.catmullRom)
                        .foregroundStyle(Color.accentColor)
                        PointMark(
                            x: .value("日期", date),
                            y: .value("平均", d.avg)
                        )
                        .foregroundStyle(Color.accentColor)
                    }
                }
                if g.baselineM > 0 {
                    RuleMark(y: .value("基线", g.baselineM))
                        .foregroundStyle(.gray)
                        .lineStyle(StrokeStyle(lineWidth: 1, dash: [4, 4]))
                        .annotation(position: .top, alignment: .leading) {
                            Text("M=\(String(format: "%.1f", g.baselineM))")
                                .font(.caption2).foregroundStyle(.secondary)
                        }
                }
            }
            .chartXAxis {
                AxisMarks(values: .stride(by: .day, count: 2)) { value in
                    AxisGridLine()
                    AxisTick()
                    AxisValueLabel(format: .dateTime.month(.defaultDigits).day())
                }
            }
            .chartYAxis {
                AxisMarks(position: .leading)
            }
        } else {
            ContentUnavailableView {
                Label("还没有握力数据", systemImage: "chart.line.uptrend.xyaxis")
            } description: {
                Text("早起左右手各捏一次,坚持累积 baseline")
            }
        }
    }

    private func parseDate(_ s: String) -> Date? {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        return f.date(from: s)
    }
}

// MARK: - 保健品月度

private struct SupplementMonthlyPanel: View {
    @Environment(AppState.self) private var state

    var body: some View {
        Group {
            if let m = state.supplementMonthly, !m.entries.isEmpty {
                List {
                    Section {
                        ForEach(m.entries) { e in
                            SupplementMonthlyRow(entry: e)
                        }
                    } header: {
                        HStack {
                            Text(m.month + " 月度完成情况")
                            Spacer()
                            Text("基于通知 dismiss 派生")
                                .font(.caption2).foregroundStyle(.tertiary)
                        }
                    }
                }
                .listStyle(.inset)
            } else {
                ContentUnavailableView {
                    Label("本月暂无数据", systemImage: "calendar.badge.clock")
                } description: {
                    Text("当 supplement 通知触发并被 dismiss 后会出现在这里")
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

private struct SupplementMonthlyRow: View {
    let entry: SupplementMonthlyEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Image(systemName: entry.type == "supplement" ? "pills" : "leaf")
                    .foregroundStyle(entry.type == "supplement" ? Color.accentColor : Color.green)
                Text(entry.name).font(.body.weight(.medium))
                Spacer()
                Text("\(entry.completedDays) / \(entry.expectedDays)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            ProgressView(value: ratio)
                .tint(entry.type == "supplement" ? Color.accentColor : Color.green)
        }
        .padding(.vertical, 4)
    }

    private var ratio: Double {
        guard entry.expectedDays > 0 else { return 0 }
        let r = Double(entry.completedDays) / Double(entry.expectedDays)
        return min(1, max(0, r))
    }
}
