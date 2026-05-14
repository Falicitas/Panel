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
                    Label("睡前", systemImage: "moon.stars").tag(RecurringKind.phone)
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
                    Text(
                        "晨间 = 起床蒙版的 checklist;睡前 = 22:00 后蒙版的 checklist;"
                        + "自由 = 不计时随时做,注入到当日 micro。"
                    )
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
                    Section(header: sectionHeader(.phone)) {
                        ForEach(items(of: .phone)) { item in
                            row(item)
                        }
                        .onMove { src, dst in
                            move(in: .phone, from: src, to: dst)
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
            Image(systemName: Self.iconName(for: kind))
                .foregroundStyle(Self.iconColor(for: kind))
            Text(Self.label(for: kind))
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
            // 循环切段: morning → phone → free → morning
            // icon: 向下走 morning/phone, 回顶 free 用向上 — 跟原来 2 态时 视觉一致
            Button {
                let target = Self.nextKind(item.kind)
                Task { await state.setRecurringKind(item, target) }
            } label: {
                Image(systemName: item.kind == .free ? "arrow.up.circle" : "arrow.down.circle")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("移到 \(Self.label(for: Self.nextKind(item.kind))) 段")

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

    private static func nextKind(_ k: RecurringKind) -> RecurringKind {
        switch k {
        case .morning: return .phone
        case .phone:   return .free
        case .free:    return .morning
        }
    }

    private static func label(for kind: RecurringKind) -> String {
        switch kind {
        case .morning: return "晨间"
        case .phone:   return "睡前"
        case .free:    return "自由"
        }
    }

    private static func iconName(for kind: RecurringKind) -> String {
        switch kind {
        case .morning: return "sun.max.fill"
        case .phone:   return "moon.stars.fill"
        case .free:    return "clock"
        }
    }

    private static func iconColor(for kind: RecurringKind) -> Color {
        switch kind {
        case .morning: return Color.anchorAccent
        case .phone:   return Color(red: 0.65, green: 0.55, blue: 0.85)
        case .free:    return Color.secondary
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
