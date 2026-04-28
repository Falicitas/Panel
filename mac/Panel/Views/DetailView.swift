import SwiftUI
import MarkdownUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isEditing = false
    @State private var editorBuffer = ""

    var body: some View {
        Group {
            if let detail = state.detail {
                ScrollViewReader { proxy in
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            if let focus = state.focus, focus.found {
                                FocusCard(focus: focus)
                            }
                            if isEditing {
                                EditorView(
                                    text: $editorBuffer,
                                    onSave: {
                                        Task {
                                            await state.saveDetail(rawMD: editorBuffer)
                                            isEditing = false
                                        }
                                    },
                                    onCancel: { isEditing = false }
                                )
                            } else {
                                Markdown(detail.rawMD)
                                    .markdownTheme(.gitHub)
                                    .padding(.horizontal)
                                    .textSelection(.enabled)
                                    .id("md-content")
                            }
                        }
                        .padding(.vertical, 12)
                    }
                    .onChange(of: state.pendingScrollLine) { _, newValue in
                        guard let line = newValue else { return }
                        Task { @MainActor in
                            try? await Task.sleep(for: .milliseconds(800))
                            // TODO(user): proxy.scrollTo("line-\(line)", anchor: .top)
                            // MarkdownUI 当前未按行打 .id;Phase 11 换 MilkDown 时再做精确 line→block 映射。
                            // 此处保留 proxy 以便用户实现滚动动画。
                            let _ = (proxy, line)
                            state.pendingScrollLine = nil
                        }
                    }
                }
                .frame(minWidth: 520)
                .navigationTitle(detail.name)
                .toolbar {
                    if !isEditing {
                        ToolbarItem {
                            anchorButton
                        }
                        ToolbarItem {
                            Button {
                                editorBuffer = detail.rawMD
                                isEditing = true
                            } label: {
                                Image(systemName: "pencil")
                            }
                            .help("编辑 Markdown 源码")
                        }
                    }
                }
            } else {
                EmptyState(selectedName: state.selectedName)
            }
        }
    }

    @ViewBuilder
    private var anchorButton: some View {
        let selected = state.selectedName
        let isAnchored = selected != nil && selected == state.anchor
        Button {
            guard let name = selected else { return }
            Task { await state.setAnchor(name: isAnchored ? nil : name) }
        } label: {
            Image(systemName: isAnchored ? "pin.fill" : "pin")
        }
        .help(isAnchored ? "取消锚定" : "锚定为主 todolist")
        .tint(isAnchored ? Color.anchorAccent : Color.secondary)
        .disabled(selected == nil)
    }
}

private struct EmptyState: View {
    let selectedName: String?
    var body: some View {
        ContentUnavailableView {
            Label(
                selectedName == nil ? "未选中 todolist" : "todolist 加载失败",
                systemImage: selectedName == nil ? "list.bullet.rectangle" : "exclamationmark.triangle"
            )
        } description: {
            Text(selectedName == nil ? "在左侧选一个开始" : "")
        }
    }
}
