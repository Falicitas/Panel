import SwiftUI

struct RecurringSheet: View {
    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var newContent = ""

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("每日复发 todo")
                    .font(.headline)
                Spacer()
                Button("完成") { dismiss() }
                    .keyboardShortcut(.defaultAction)
            }
            .padding()

            Divider()

            HStack(spacing: 8) {
                TextField("添加一条复发 todo (例:喝水 ≥ 2L)", text: $newContent)
                    .textFieldStyle(.roundedBorder)
                    .onSubmit { addCurrent() }
                Button {
                    addCurrent()
                } label: {
                    Image(systemName: "plus.circle.fill")
                        .imageScale(.large)
                }
                .buttonStyle(.borderless)
                .disabled(trimmed.isEmpty)
            }
            .padding(.horizontal)
            .padding(.top, 12)

            if state.recurring.isEmpty {
                Spacer()
                ContentUnavailableView {
                    Label("还没有任何复发 todo", systemImage: "repeat")
                } description: {
                    Text("Phase 5 蒙版选\"普通模式\"时,这里的项会注入到当日 micro 顶部;选\"外出\"时不注入")
                }
                Spacer()
            } else {
                List {
                    ForEach(state.recurring) { item in
                        HStack {
                            Text(item.content)
                                .lineLimit(2)
                            Spacer()
                            Button {
                                Task { await state.deleteRecurring(item) }
                            } label: {
                                Image(systemName: "minus.circle")
                                    .foregroundStyle(.red)
                            }
                            .buttonStyle(.borderless)
                            .help("删除")
                        }
                    }
                }
                .listStyle(.inset)

                Text("Phase 5 蒙版选\"普通模式\"时,这里的项会自动注入到当日 micro 顶部;选\"外出\"时不注入。当日 micro 文件中的复发项不能直接删,只能勾选 / 加 [[[skip]]];永久删除走这里。")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .padding(.horizontal)
                    .padding(.bottom, 12)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .frame(minWidth: 480, minHeight: 380)
    }

    private var trimmed: String {
        newContent.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func addCurrent() {
        let t = trimmed
        guard !t.isEmpty else { return }
        Task {
            await state.addRecurring(content: t)
            newContent = ""
        }
    }
}
