import SwiftUI

struct MicroDetailView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 24) {
            VStack(alignment: .leading, spacing: 4) {
                Text(dateLabel)
                    .font(.title3.weight(.semibold))
                Text("当日 Micro 上下文")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            VStack(alignment: .leading, spacing: 14) {
                StatRow(
                    icon: "sun.and.horizon",
                    title: "wake_time",
                    value: "Phase 5 接入"
                )
                StatRow(
                    icon: "checkmark.square",
                    title: "今日 micro 任务",
                    value: microStatsValue
                )
                StatRow(
                    icon: "repeat",
                    title: "复发 todo 总数",
                    value: "\(state.recurring.count) 条"
                )
            }

            Spacer()

            Text("当日 micro 在左侧编辑;复发 todo 通过左侧\"管理复发\"按钮维护。Phase 5 蒙版选\"普通模式\"后,复发 todo 会自动注入到当日 micro 顶部。")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var dateLabel: String {
        let f = DateFormatter()
        f.dateFormat = "yyyy 年 M 月 d 日 EEEE"
        f.locale = Locale(identifier: "zh_CN")
        return f.string(from: Date())
    }

    private var microStatsValue: String {
        let items = state.microToday?.items ?? []
        if items.isEmpty { return "—" }
        var done = 0, skip = 0
        for it in items {
            if it.checked { done += 1 } else if it.skip { skip += 1 }
        }
        var s = "\(done)/\(items.count) 完成"
        if skip > 0 { s += " · skip \(skip)" }
        return s
    }
}

private struct StatRow: View {
    let icon: String
    let title: String
    let value: String

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: icon)
                .frame(width: 20)
                .foregroundStyle(.secondary)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(value)
                    .font(.body)
            }
            Spacer()
        }
    }
}
