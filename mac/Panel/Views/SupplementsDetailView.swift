import SwiftUI

struct SupplementsDetailView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                StateCard()
                NotificationsCard()
                Stats()
                Spacer(minLength: 8)
                Footer()
            }
            .padding(20)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .navigationTitle("今日上下文")
    }
}

private struct StateCard: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(dateLabel)
                .font(.title3.weight(.semibold))
            HStack(spacing: 16) {
                LabeledValue(label: "状态", value: modeLabel(state.dailyState?.mode ?? "normal"))
                LabeledValue(
                    label: "wake_time",
                    value: state.dailyState?.wakeTime.isEmpty == false
                        ? state.dailyState!.wakeTime
                        : "未设 (Phase 5.C 蒙版)"
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }

    private var dateLabel: String {
        let f = DateFormatter()
        f.dateFormat = "yyyy 年 M 月 d 日 EEEE"
        f.locale = Locale(identifier: "zh_CN")
        return f.string(from: Date())
    }

    private func modeLabel(_ m: String) -> String {
        switch m {
        case "focus":   "专注"
        case "sleep":   "睡眠"
        case "outside": "外出"
        default:        "普通"
        }
    }
}

private struct NotificationsCard: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("今日通知")
                .font(.caption).foregroundStyle(.secondary)
            HStack(spacing: 16) {
                LabeledValue(label: "未 dismiss", value: "\(state.notifications.count) 条")
                LabeledValue(
                    label: "提示",
                    value: state.notifications.isEmpty ? "无" : "顶部铃铛展开看"
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct Stats: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("当前在册")
                .font(.caption).foregroundStyle(.secondary)
            HStack(spacing: 16) {
                LabeledValue(
                    label: "食物",
                    value: "\(state.supplements.filter { $0.type == "food" }.count) 项"
                )
                LabeledValue(
                    label: "保健品",
                    value: "\(state.supplements.filter { $0.type == "supplement" }.count) 项"
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct Footer: View {
    var body: some View {
        Text("当月每项的 \"完成天数 / 应完成\" 借记录模块展示位 (Phase 6 接入)。Phase 5.C 加蒙版 + 4 状态主题色 + 状态切换。")
            .font(.caption2)
            .foregroundStyle(.tertiary)
    }
}

private struct LabeledValue: View {
    let label: String
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.body)
        }
    }
}
