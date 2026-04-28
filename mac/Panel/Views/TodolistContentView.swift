import SwiftUI

struct TodolistContentView: View {
    @Environment(AppState.self) private var state
    @State private var selectedName: String?

    var body: some View {
        List(selection: $selectedName) {
            if !state.active.isEmpty {
                Section("进行中") {
                    ForEach(state.active) { entry in
                        TodolistRow(entry: entry).tag(entry.name as String?)
                    }
                }
            }
            if !state.done.isEmpty {
                Section("已完成") {
                    ForEach(state.done) { entry in
                        TodolistRow(entry: entry).tag(entry.name as String?)
                    }
                }
            }
        }
        .listStyle(.sidebar)
        .navigationSplitViewColumnWidth(min: 280, ideal: 320, max: 400)
        .navigationTitle("Todolist")
        .onChange(of: state.anchor, initial: true) { _, newValue in
            selectedName = newValue
        }
        .onChange(of: selectedName) { _, newValue in
            guard newValue != state.anchor else { return }
            Task { await state.setAnchor(name: newValue) }
        }
        .toolbar {
            ToolbarItem {
                Button {
                    Task { await state.loadAll() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("刷新列表")
                .disabled(state.isLoading)
            }
            if state.anchor != nil {
                ToolbarItem {
                    Button {
                        selectedName = nil
                    } label: {
                        Image(systemName: "xmark.circle")
                    }
                    .help("取消锚定")
                }
            }
        }
    }
}

private struct TodolistRow: View {
    let entry: ListEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(entry.name).lineLimit(2)
            Text(progressString)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
    }

    private var progressString: String {
        var s = "\(entry.done)/\(entry.totalTodo)"
        if entry.skipped > 0 { s += " · skip \(entry.skipped)" }
        return s
    }
}
