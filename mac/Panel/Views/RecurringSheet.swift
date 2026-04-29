import SwiftUI

struct RecurringSheet: View {
    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var newContent = ""
    @State private var newKind: RecurringKind = .morning

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
                Picker("", selection: $newKind) {
                    Label("晨间", systemImage: "sun.max").tag(RecurringKind.morning)
                    Label("自由", systemImage: "clock").tag(RecurringKind.free)
                }
                .pickerStyle(.segmented)
                .fixedSize()
                .labelsHidden()

                TextField("新增 (例:喝水 ≥ 2L)", text: $newContent)
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
                    Text("晨间 = 起床后一并做完(在面包屑位置显示完成进度);自由 = 不计时随时做")
                }
                Spacer()
            } else {
                List {
                    Section(header: sectionHeader(.morning)) {
                        ForEach(items(of: .morning)) { item in
                            row(item)
                        }
                        .onMove { src, dst in
                            move(in: .morning, from: src, to: dst)
                        }
                    }
                    Section(header: sectionHeader(.free)) {
                        ForEach(items(of: .free)) { item in
                            row(item)
                        }
                        .onMove { src, dst in
                            move(in: .free, from: src, to: dst)
                        }
                    }
                }
                .listStyle(.inset)
            }
        }
        .frame(minWidth: 520, minHeight: 420)
    }

    private func sectionHeader(_ kind: RecurringKind) -> some View {
        HStack {
            Image(systemName: kind == .morning ? "sun.max.fill" : "clock")
                .foregroundStyle(kind == .morning ? Color.anchorAccent : Color.secondary)
            Text(kind == .morning ? "晨间" : "自由")
                .font(.subheadline.weight(.semibold))
            Text("(\(items(of: kind).count))")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
    }

    private func row(_ item: RecurringTodo) -> some View {
        HStack {
            Text(item.content)
                .lineLimit(2)
            Spacer()
            Button {
                let target: RecurringKind = item.kind == .morning ? .free : .morning
                Task { await state.setRecurringKind(item, target) }
            } label: {
                Image(systemName: item.kind == .morning ? "arrow.down.circle" : "arrow.up.circle")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help(item.kind == .morning ? "移到自由段" : "移到晨间段")

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

    private func items(of kind: RecurringKind) -> [RecurringTodo] {
        state.recurring.filter { $0.kind == kind }
    }

    private func move(in kind: RecurringKind, from src: IndexSet, to dst: Int) {
        var ids = items(of: kind).map(\.id)
        ids.move(fromOffsets: src, toOffset: dst)
        Task { await state.reorderRecurring(kind: kind, ids: ids) }
    }

    private var trimmed: String {
        newContent.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func addCurrent() {
        let t = trimmed
        guard !t.isEmpty else { return }
        let kind = newKind
        Task {
            await state.addRecurring(content: t, kind: kind)
            newContent = ""
        }
    }
}
