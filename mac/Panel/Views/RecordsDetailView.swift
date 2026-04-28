import SwiftUI

struct RecordsDetailView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                switch state.recordsTab {
                case .grip:       GripDetail()
                case .supplement: SupplementMonthlyDetail()
                }
                Spacer(minLength: 8)
                Footer()
            }
            .padding(20)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .navigationTitle(state.recordsTab.label)
    }
}

// MARK: - 握力侧栏

private struct GripDetail: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("历史 14 天").font(.caption).foregroundStyle(.secondary)
            if let g = state.grip, !g.history.isEmpty {
                HistoryTable(history: g.history.reversed())
                if g.baselineDays >= 3 {
                    BaselineCard(g: g)
                }
            } else {
                ContentUnavailableView("暂无历史", systemImage: "clock")
                    .frame(maxWidth: .infinity, minHeight: 120)
            }
        }
    }
}

private struct HistoryTable: View {
    let history: [GripDayValue]

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("日期").frame(width: 96, alignment: .leading)
                Text("左").frame(maxWidth: .infinity, alignment: .trailing)
                Text("右").frame(maxWidth: .infinity, alignment: .trailing)
                Text("均").frame(maxWidth: .infinity, alignment: .trailing)
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .padding(.vertical, 6)
            .padding(.horizontal, 12)
            .background(.quaternary.opacity(0.3))
            ForEach(Array(history.enumerated()), id: \.offset) { _, d in
                HStack {
                    Text(short(d.date)).frame(width: 96, alignment: .leading)
                        .foregroundStyle(.secondary)
                    Text(fmt(d.left)).frame(maxWidth: .infinity, alignment: .trailing)
                    Text(fmt(d.right)).frame(maxWidth: .infinity, alignment: .trailing)
                    Text(fmt(d.avg))
                        .frame(maxWidth: .infinity, alignment: .trailing)
                        .foregroundStyle(.primary).bold()
                }
                .font(.caption.monospacedDigit())
                .padding(.vertical, 4)
                .padding(.horizontal, 12)
                Divider()
            }
        }
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8))
    }

    private func short(_ s: String) -> String {
        // YYYY-MM-DD → MM-DD
        if s.count == 10 { return String(s.dropFirst(5)) }
        return s
    }

    private func fmt(_ v: Double) -> String { v == 0 ? "—" : String(format: "%.1f", v) }
}

private struct BaselineCard: View {
    let g: GripSummary

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Baseline (近 \(g.baselineDays) 天,不含今日)")
                .font(.caption).foregroundStyle(.secondary)
            HStack(spacing: 16) {
                LabeledValue(label: "M", value: String(format: "%.1f", g.baselineM))
                LabeledValue(label: "SD", value: String(format: "%.2f", g.sd))
                LabeledValue(label: "z (今日)", value: String(format: "%+.2f", g.deviation))
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8))
    }
}

// MARK: - 保健品月度侧栏

private struct SupplementMonthlyDetail: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            if let m = state.supplementMonthly {
                MonthSummary(monthly: m)
            } else {
                ContentUnavailableView("暂无月度数据", systemImage: "calendar")
                    .frame(maxWidth: .infinity, minHeight: 120)
            }
        }
    }
}

private struct MonthSummary: View {
    let monthly: SupplementMonthly

    private var totalCompleted: Int { monthly.entries.reduce(0) { $0 + $1.completedDays } }
    private var totalExpected: Int { monthly.entries.reduce(0) { $0 + $1.expectedDays } }
    private var ratio: Double {
        guard totalExpected > 0 else { return 0 }
        return Double(totalCompleted) / Double(totalExpected)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(monthly.month + " 总览").font(.headline)
            HStack(spacing: 16) {
                LabeledValue(label: "已完成", value: "\(totalCompleted) 次")
                LabeledValue(label: "应完成", value: "\(totalExpected) 次")
                LabeledValue(label: "完成率", value: String(format: "%.0f%%", ratio * 100))
            }
            if totalExpected > 0 {
                ProgressView(value: ratio)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct LabeledValue: View {
    let label: String
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.body.monospacedDigit())
        }
    }
}

private struct Footer: View {
    var body: some View {
        Text("握力 baseline = 不含今日的过去 14 天均值;|z|<=1 → 正常;-2~-1 → 降强度;<-2 → 改有氧/休息。供参考,不替代教练判断。")
            .font(.caption2)
            .foregroundStyle(.tertiary)
    }
}
