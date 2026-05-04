import SwiftUI

/// 顶栏快捷按钮: 一键跳回 todolist 锚定 focus 卡片那一行 + hide sidebar。
/// 用于 SupplementsView / EnglishView 等 "完成杂事直奔关注" 场景。
/// focus 不存在 (无 anchor / 无 first-undone) → disabled。
/// state.focus 来自 todolist anchor (不是 micro 当日, micro 不进 state.focus)。
struct JumpToFocusButton: View {
    @Environment(AppState.self) private var state

    var body: some View {
        let f = state.focus
        let enabled = (f?.text ?? "").isEmpty == false
        Button {
            Task { await state.jumpToTodolistFocus() }
        } label: {
            Image(systemName: "scope")
        }
        .disabled(!enabled)
        .help(enabled
              ? "回到 Todolist 聚焦行 (隐藏 sidebar)"
              : "无聚焦行 (todolist 未锚定 / 无未完成项)")
    }
}
