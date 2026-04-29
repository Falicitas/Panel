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
    /// - K/N (0<K<N): "晨间 K/N · 已用 mm:ss" (从 startAt 起活计时)
    /// - N/N: "晨间 N/N · 用时 mm:ss" (定格 endAt - startAt)
    @ViewBuilder
    private var morningProgressRow: some View {
        if state.contentMode == .micro,
           let p = state.microProgress, p.hasMorning {
            let _ = state.timeTick  // 让 view 跟着 tick 重算计时
            HStack(spacing: 8) {
                Image(systemName: p.isComplete ? "checkmark.circle.fill" : "sun.max.fill")
                    .foregroundStyle(p.isComplete ? Color.green : Color.anchorAccent)
                Text("晨间")
                    .font(.title3.weight(.semibold))
                Text("\(p.morningDone)/\(p.morningTotal)")
                    .font(.title3.monospacedDigit())
                    .foregroundStyle(p.isComplete ? .green : .primary)
                if let timer = morningTimerText(p) {
                    Text("·")
                        .foregroundStyle(.tertiary)
                    Text(timer)
                        .font(.body.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    /// 时间文案。
    /// - 已完成:"用时 X" (end - start)
    /// - 进行中(start 已写):"已用 X" (now - start)
    /// - 完全没动:nil
    private func morningTimerText(_ p: MicroProgress) -> String? {
        guard let start = p.morningStartAt else { return nil }
        if let end = p.morningEndAt {
            return "用时 " + formatDuration(end - start)
        }
        let now = Int64(Date().timeIntervalSince1970)
        return "已用 " + formatDuration(max(0, now - start))
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
