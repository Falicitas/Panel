import SwiftUI

private enum SupplementsFilter: String, CaseIterable, Identifiable {
    case all, food, supplement
    var id: String { rawValue }
    var label: String {
        switch self {
        case .all:        "全部"
        case .food:       "食物"
        case .supplement: "保健品"
        }
    }
}

struct SupplementsContentView: View {
    @Environment(AppState.self) private var state
    @State private var filter: SupplementsFilter = .all
    @State private var creating = false
    @State private var editing: Supplement?

    var body: some View {
        VStack(spacing: 0) {
            Picker("", selection: $filter) {
                ForEach(SupplementsFilter.allCases) { t in
                    Text(t.label).tag(t)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            Divider()
            list
        }
        .navigationTitle("食物 & 保健品")
        .toolbar {
            ToolbarItem {
                Button {
                    creating = true
                } label: {
                    Image(systemName: "plus")
                }
                .help("添加")
            }
        }
        .sheet(isPresented: $creating) {
            SupplementFormSheet(initial: .blank, isNew: true).environment(state)
        }
        .sheet(item: $editing) { sp in
            SupplementFormSheet(initial: sp, isNew: false).environment(state)
        }
    }

    private var filtered: [Supplement] {
        switch filter {
        case .all:        state.supplements
        case .food:       state.supplements.filter { $0.type == "food" }
        case .supplement: state.supplements.filter { $0.type == "supplement" }
        }
    }

    @ViewBuilder
    private var list: some View {
        if filtered.isEmpty {
            ContentUnavailableView {
                Label("还没有食物 / 保健品", systemImage: "pills")
            } description: {
                Text("右上 + 添加,可设多个时段 (HH:MM 或 wake±Nh)")
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List {
                ForEach(filtered) { sp in
                    SupplementRow(sp: sp)
                        .contentShape(Rectangle())
                        .onTapGesture { editing = sp }
                }
            }
            .listStyle(.inset)
        }
    }
}

private struct SupplementRow: View {
    let sp: Supplement
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 8) {
                Image(systemName: sp.type == "supplement" ? "pills" : "leaf")
                    .foregroundStyle(sp.type == "supplement" ? Color.accentColor : Color.green)
                Text(sp.name).font(.body)
                Spacer()
                Text(sp.type == "supplement" ? "保健品" : "食物")
                    .font(.caption2)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(.tertiary.opacity(0.3), in: Capsule())
            }
            if !sp.windows.isEmpty {
                Text(windowsLabel)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            if !sp.notes.isEmpty {
                Text(sp.notes)
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .lineLimit(2)
            }
        }
        .padding(.vertical, 2)
    }

    private var windowsLabel: String {
        sp.windows.map { "\($0.start)–\($0.end)" }.joined(separator: " · ")
    }
}
