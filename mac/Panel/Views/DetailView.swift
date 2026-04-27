import SwiftUI
import MarkdownUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isEditing = false
    @State private var editorBuffer = ""

    var body: some View {
        Group {
            if let detail = state.detail {
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
                        }
                    }
                    .padding(.vertical, 12)
                }
                .frame(minWidth: 520)
                .navigationTitle(detail.name)
                .toolbar {
                    if !isEditing {
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
                EmptyState(anchor: state.anchor)
            }
        }
    }
}

private struct EmptyState: View {
    let anchor: String?
    var body: some View {
        ContentUnavailableView {
            Label(
                anchor == nil ? "未锚定 todolist" : "锚定的 todolist 加载失败",
                systemImage: anchor == nil ? "list.bullet.rectangle" : "exclamationmark.triangle"
            )
        } description: {
            Text(anchor == nil ? "在左侧选一个开始" : "")
        }
    }
}
