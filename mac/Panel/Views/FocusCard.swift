import SwiftUI

struct FocusCard: View {
    @Environment(AppState.self) private var state
    let focus: FocusResult
    /// 多 focus 时的当前显示索引, 持久化一份 (不 per-file) — 重启 panel 保留, 切换锚定
    /// 重置回 0。UserDefaults key = "focusCard.groupIdx", @State 初值读 UserDefaults。
    @State private var groupIdx: Int = UserDefaults.standard.integer(forKey: Self.groupIdxKey)
    private static let groupIdxKey = "focusCard.groupIdx"

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
        // 锚定切换 (focus.anchored 变化) → 重置 idx 到第一项 + 写 UserDefaults。
        // 这是用户原话 "重新锚定到其他文件上的时候,可持久化的下标就初始化为 1"。
        .onChange(of: focus.anchored) { _, _ in setGroupIdx(0) }
        // focus.group 内容变化 (同锚定下用户加/删 [[[focus]]] 标记) → 不重置, 但 clamp
        // 防 idx 指到不存在位置 (group 缩短后)。
        .onChange(of: focus.group) { _, new in
            if let g = new, !g.isEmpty, groupIdx >= g.count {
                setGroupIdx(g.count - 1)
            }
        }
    }

    /// 写 UserDefaults + 更新 @State, 持久化 groupIdx 一份全局。panel 重启后 @State
    /// init 会读这个 key 恢复;锚定切换时由调用方传 0 重置。
    private func setGroupIdx(_ idx: Int) {
        groupIdx = idx
        UserDefaults.standard.set(idx, forKey: Self.groupIdxKey)
    }

    /// 多 focus 切换控件 — chevron.left / 当前页码 (1/3) / chevron.right, 循环。
    /// 类似 cmd+F 找下一处的语义, 切换时同步 jumpToFocus 滚到该行 editor 上。
    @ViewBuilder
    private func groupNav(total: Int) -> some View {
        HStack(spacing: 12) {
            Button {
                setGroupIdx((groupIdx - 1 + total) % total)
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
                setGroupIdx((groupIdx + 1) % total)
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

}
