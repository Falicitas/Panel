import SwiftUI

struct FocusCard: View {
    @Environment(AppState.self) private var state
    let focus: FocusResult
    /// 多 focus 时的当前显示索引。focus.group 变化 (id 用 group hash) 时会重置。
    @State private var groupIdx: Int = 0

    /// 当前显示的 (text, lineNo, breadcrumbs) — 多 focus 模式从 group[groupIdx] 取,
    /// 单条 / 无 group 模式从顶层 focus 取。封装一下避免 body 多处分支。
    private var displayed: (text: String?, lineNo: Int?, breadcrumbs: [String]?) {
        if let group = focus.group, !group.isEmpty {
            let idx = min(max(groupIdx, 0), group.count - 1)
            let item = group[idx]
            return (item.text, item.lineNo, item.breadcrumbs)
        }
        return (focus.text, focus.lineNo, focus.breadcrumbs)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            morningProgressRow
            if let group = focus.group, group.count >= 2 {
                groupNav(total: group.count)
            }
            if let crumbs = displayed.breadcrumbs, !crumbs.isEmpty {
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
            Text(displayed.text ?? "")
                .font(.title.weight(.semibold))
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
            if let n = displayed.lineNo {
                Button {
                    Task { await state.jumpToFocus(displayed.text) }
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
        // focus.group 变化时 (todolist 重 fetch / 锚定切换 / 用户改 [[[focus]]] 标记)
        // 重置到第一项, 防 idx 指到旧 group 不存在的位置。
        .onChange(of: focus.group) { _, _ in groupIdx = 0 }
    }

    /// 多 focus 切换控件 — chevron.left / 当前页码 (1/3) / chevron.right, 循环。
    /// 类似 cmd+F 找下一处的语义, 切换时同步 jumpToFocus 滚到该行 editor 上。
    @ViewBuilder
    private func groupNav(total: Int) -> some View {
        HStack(spacing: 12) {
            Button {
                groupIdx = (groupIdx - 1 + total) % total
                jumpToCurrentGroup()
            } label: {
                Image(systemName: "chevron.left")
                    .font(.body.weight(.medium))
                    .foregroundStyle(Color.anchorAccent)
                    .frame(width: 28, height: 28)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help("上一个 focus")

            Text("\(min(groupIdx, total - 1) + 1) / \(total)")
                .font(.callout.monospacedDigit())
                .foregroundStyle(.secondary)

            Button {
                groupIdx = (groupIdx + 1) % total
                jumpToCurrentGroup()
            } label: {
                Image(systemName: "chevron.right")
                    .font(.body.weight(.medium))
                    .foregroundStyle(Color.anchorAccent)
                    .frame(width: 28, height: 28)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help("下一个 focus")

            Spacer()
        }
    }

    private func jumpToCurrentGroup() {
        guard let group = focus.group, !group.isEmpty else { return }
        let idx = min(max(groupIdx, 0), group.count - 1)
        let text = group[idx].text
        Task { await state.jumpToFocus(text) }
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
