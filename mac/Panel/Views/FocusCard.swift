import SwiftUI

struct FocusCard: View {
    @Environment(AppState.self) private var state
    let focus: FocusResult

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            morningProgressRow
            if let crumbs = focus.breadcrumbs, !crumbs.isEmpty {
                HStack(spacing: 6) {
                    ForEach(Array(crumbs.enumerated()), id: \.offset) { idx, crumb in
                        if idx > 0 {
                            Image(systemName: "chevron.right")
                                .font(.body)
                                .foregroundStyle(.tertiary)
                        }
                        Text(crumb)
                            .font(.title3)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Text(focus.text ?? "")
                .font(.title.weight(.semibold))
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
            if let n = focus.lineNo {
                Button {
                    Task { await state.jumpToFocus(focus.text) }
                } label: {
                    Text("L\(n)")
                        .font(.body.monospacedDigit())
                        .underline()
                }
                .buttonStyle(.plain)
                .foregroundStyle(Color.anchorAccent)
                .help("跳到锚定 todolist 第 \(n) 行")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(20)
        .background(Color.anchorAccent.opacity(0.08))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(Color.anchorAccent.opacity(0.3), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal)
    }

    /// 晨间进度行:仅 micro 模式且至少有 1 条 morning recurring 时显示。
    /// - 0/N: "晨间 0/N" (未开始,无计时)
    /// - K/N (0<K<N): "晨间 K/N · 已用 mm:ss" (从 startAt 起活计时, 1s 一跳)
    /// - N/N: "晨间 N/N · 用时 mm:ss" (定格 endAt - startAt)
    /// 进行中 timer 用 TimelineView(.periodic(by: 1)) 自己秒级刷新,
    /// 不依赖 state.timeTick (那个跟着 5s backend poll, 跳得太慢)。
    @ViewBuilder
    private var morningProgressRow: some View {
        if state.contentMode == .micro,
           let p = state.microProgress, p.hasMorning {
            HStack(spacing: 8) {
                Image(systemName: p.isComplete ? "checkmark.circle.fill" : "sun.max.fill")
                    .foregroundStyle(p.isComplete ? Color.green : Color.anchorAccent)
                Text("晨间")
                    .font(.title3.weight(.semibold))
                Text("\(p.morningDone)/\(p.morningTotal)")
                    .font(.title3.monospacedDigit())
                    .foregroundStyle(p.isComplete ? .green : .primary)
                morningTimerView(p)
            }
        }
    }

    /// 进行中: TimelineView 1s 一刷, ctx.date 算 elapsed → "已用 mm:ss" 秒级跳动。
    /// 已完成 / 未开始: 静态文本 / 空。
    @ViewBuilder
    private func morningTimerView(_ p: MicroProgress) -> some View {
        if let start = p.morningStartAt {
            if let end = p.morningEndAt {
                Text("·").foregroundStyle(.tertiary)
                Text("用时 " + formatDuration(end - start))
                    .font(.body.monospacedDigit())
                    .foregroundStyle(.secondary)
            } else {
                TimelineView(.periodic(from: .now, by: 1)) { ctx in
                    let elapsed = max(0, Int64(ctx.date.timeIntervalSince1970) - start)
                    HStack(spacing: 8) {
                        Text("·").foregroundStyle(.tertiary)
                        Text("已用 " + formatDuration(elapsed))
                            .font(.body.monospacedDigit())
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }

    private func formatDuration(_ secs: Int64) -> String {
        let s = Int(secs)
        let h = s / 3600
        let m = (s % 3600) / 60
        let sec = s % 60
        if h > 0 {
            return String(format: "%d:%02d:%02d", h, m, sec)
        }
        return String(format: "%d:%02d", m, sec)
    }
}
