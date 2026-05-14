import SwiftUI

/// 顶栏快捷按钮: 一键跳回 todolist 锚定 focus 卡片那一行 + hide sidebar。
/// 用于 SupplementsView / EnglishView 等 "完成杂事直奔关注" 场景。
/// 优先 todolist anchor focus; todolist 无锚定 / 无未完成项时 fallback 跳到当日 micro
/// 第一个未完成项 (state.microFocusText), 两者都没有才 disabled。
struct JumpToFocusButton: View {
    @Environment(AppState.self) private var state

    var body: some View {
        let hasTodolist = (state.focus?.text ?? "").isEmpty == false
        let hasMicro = (state.microFocusText ?? "").isEmpty == false
        let enabled = hasTodolist || hasMicro
        Button {
            Task {
                if hasTodolist {
                    await state.jumpToTodolistFocus()
                } else if hasMicro {
                    await state.jumpToMicroFocus()
                }
            }
        } label: {
            Image(systemName: "scope")
        }
        .disabled(!enabled)
        .help(helpText(hasTodolist: hasTodolist, hasMicro: hasMicro))
    }

    private func helpText(hasTodolist: Bool, hasMicro: Bool) -> String {
        if hasTodolist {
            return "回到 Todolist 聚焦行 (隐藏 sidebar)"
        }
        if hasMicro {
            return "Todolist 未锚定 — 跳到当日 micro 首个未完成项"
        }
        return "无聚焦行 (todolist 未锚定 + micro 无未完成项)"
    }
}
