import SwiftUI
import MarkdownUI

struct MicroContentView: View {
    @Environment(AppState.self) private var state
    @State private var isEditing = false
    @State private var editorBuffer = ""
    @State private var isShowingRecurring = false

    var body: some View {
        Group {
            if let today = state.microToday {
                content(for: today)
                    .navigationTitle(today.name)
                    .toolbar {
                        ToolbarItemGroup {
                            Button {
                                isShowingRecurring = true
                            } label: {
                                Label("管理复发", systemImage: "repeat")
                            }
                            .help("管理每日复发 todo")
                            if !isEditing {
                                Button {
                                    editorBuffer = today.rawMD
                                    isEditing = true
                                } label: {
                                    Image(systemName: "pencil")
                                }
                                .help("编辑当日 micro")
                            }
                        }
                    }
                    .sheet(isPresented: $isShowingRecurring) {
                        RecurringSheet()
                            .environment(state)
                    }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    @ViewBuilder
    private func content(for today: Todolist) -> some View {
        if isEditing {
            EditorView(
                text: $editorBuffer,
                onSave: {
                    Task {
                        await state.saveMicroToday(rawMD: editorBuffer)
                        isEditing = false
                    }
                },
                onCancel: { isEditing = false }
            )
        } else if today.rawMD.isEmpty {
            EmptyMicro {
                editorBuffer = ""
                isEditing = true
            }
        } else {
            ScrollView {
                Markdown(today.rawMD)
                    .markdownTheme(.gitHub)
                    .padding(.horizontal)
                    .padding(.vertical, 12)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }
}

private struct EmptyMicro: View {
    let onEdit: () -> Void
    var body: some View {
        VStack(spacing: 16) {
            Spacer()
            ContentUnavailableView {
                Label("今日 Micro 还没有任何条目", systemImage: "calendar.badge.plus")
            } description: {
                Text("点上方铅笔开始添加今天的临时事务")
            }
            Button {
                onEdit()
            } label: {
                Label("编辑当日", systemImage: "pencil")
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            Spacer()
        }
        .padding()
    }
}
