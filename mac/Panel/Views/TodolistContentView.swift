import SwiftUI

struct TodolistContentView: View {
    @Environment(AppState.self) private var state
    @State private var creating = false
    @State private var newName = ""

    var body: some View {
        @Bindable var bindable = state
        return List(selection: $bindable.selectedName) {
            if !state.active.isEmpty {
                Section("进行中") {
                    ForEach(state.active) { entry in
                        rowView(for: entry)
                    }
                }
            }
            if !state.done.isEmpty {
                Section("已完成") {
                    ForEach(state.done) { entry in
                        rowView(for: entry)
                    }
                }
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("Todolist")
        .onChange(of: state.selectedName) { _, newValue in
            Task { await state.selectList(newValue) }
        }
        .toolbar {
            ToolbarItem {
                Button {
                    newName = ""
                    creating = true
                } label: {
                    Image(systemName: "plus")
                }
                .help("新建 todolist")
            }
            ToolbarItem {
                Button {
                    Task { await state.loadAll() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("刷新列表")
                .disabled(state.isLoading)
            }
        }
        .alert("新建 todolist", isPresented: $creating) {
            TextField("名称 (.md 自动补)", text: $newName)
            Button("取消", role: .cancel) {}
            Button("创建") {
                let name = newName
                Task { await state.createTodolist(name: name) }
            }
            .disabled(newName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        } message: {
            Text("空文件,可在右栏直接编辑;创建后会自动锚定")
        }
    }

    @ViewBuilder
    private func rowView(for entry: ListEntry) -> some View {
        let isAnchored = entry.name == state.anchor
        TodolistRow(entry: entry, isAnchored: isAnchored)
            .tag(entry.name as String?)
            .listRowBackground(isAnchored ? Color.anchorAccent : Color.clear)
    }
}

private struct TodolistRow: View {
    let entry: ListEntry
    let isAnchored: Bool

    var body: some View {
        HStack(spacing: 6) {
            if isAnchored {
                Image(systemName: "pin.fill")
                    .font(.caption.weight(.bold))
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(entry.name).lineLimit(2)
                Text(progressString)
                    .font(.caption)
                    .foregroundStyle(isAnchored ? Color.white.opacity(0.85) : Color.secondary)
            }
        }
        .padding(.vertical, 2)
        .foregroundStyle(isAnchored ? Color.white : Color.primary)
    }

    private var progressString: String {
        var s = "\(entry.done)/\(entry.totalTodo)"
        if entry.skipped > 0 { s += " · skip \(entry.skipped)" }
        return s
    }
}
