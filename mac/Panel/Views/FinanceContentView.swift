import SwiftUI

private enum FinanceSection: String, CaseIterable, Identifiable {
    case bills, fixed, free, incomes
    var id: String { rawValue }
    var label: String {
        switch self {
        case .bills:   "账单"
        case .fixed:   "固定"
        case .free:    "自由"
        case .incomes: "收入"
        }
    }
}

struct FinanceContentView: View {
    @Environment(AppState.self) private var state
    @State private var selected: FinanceSection = .bills
    @State private var creating = false
    @State private var editingBill: Bill?
    @State private var editingFixed: FixedExpense?
    @State private var editingFree: FreeExpense?
    @State private var editingIncome: Income?

    var body: some View {
        VStack(spacing: 0) {
            Picker("", selection: $selected) {
                ForEach(FinanceSection.allCases) { t in
                    Text(t.label).tag(t)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            Divider()
            sectionList
        }
        .navigationTitle("财务")
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
        .sheet(isPresented: $creating) { newSheet }
        .sheet(item: $editingBill) { b in
            BillFormSheet(initial: b, isNew: false).environment(state)
        }
        .sheet(item: $editingFixed) { f in
            FixedFormSheet(initial: f, isNew: false).environment(state)
        }
        .sheet(item: $editingFree) { f in
            FreeFormSheet(initial: f, isNew: false).environment(state)
        }
        .sheet(item: $editingIncome) { i in
            IncomeFormSheet(initial: i, isNew: false).environment(state)
        }
    }

    @ViewBuilder
    private var newSheet: some View {
        switch selected {
        case .bills:   BillFormSheet(initial: .blank, isNew: true).environment(state)
        case .fixed:   FixedFormSheet(initial: .blank, isNew: true).environment(state)
        case .free:    FreeFormSheet(initial: .blank, isNew: true).environment(state)
        case .incomes: IncomeFormSheet(initial: .blank, isNew: true).environment(state)
        }
    }

    @ViewBuilder
    private var sectionList: some View {
        switch selected {
        case .bills:   BillsList(onEdit: { editingBill = $0 })
        case .fixed:   FixedList(onEdit: { editingFixed = $0 })
        case .free:    FreeList(onEdit: { editingFree = $0 })
        case .incomes: IncomesList(onEdit: { editingIncome = $0 })
        }
    }
}

private struct BillsList: View {
    @Environment(AppState.self) private var state
    let onEdit: (Bill) -> Void
    var body: some View {
        if state.bills.isEmpty {
            EmptyHint(label: "还没有账单", systemImage: "creditcard", hint: "右上 + 添加")
        } else {
            List {
                ForEach(state.bills) { b in
                    BillRow(b: b)
                        .contentShape(Rectangle())
                        .onTapGesture { onEdit(b) }
                }
            }
            .listStyle(.inset)
        }
    }
}

private struct FixedList: View {
    @Environment(AppState.self) private var state
    let onEdit: (FixedExpense) -> Void
    var body: some View {
        if state.fixed.isEmpty {
            EmptyHint(label: "还没有固定支出", systemImage: "calendar.badge.clock", hint: "右上 + 添加")
        } else {
            List {
                ForEach(state.fixed) { f in
                    FixedRow(f: f)
                        .contentShape(Rectangle())
                        .onTapGesture { onEdit(f) }
                }
            }
            .listStyle(.inset)
        }
    }
}

private struct FreeList: View {
    @Environment(AppState.self) private var state
    let onEdit: (FreeExpense) -> Void
    var body: some View {
        if state.frees.isEmpty {
            EmptyHint(label: "还没有自由支出", systemImage: "cart", hint: "右上 + 添加")
        } else {
            List {
                ForEach(state.frees) { f in
                    FreeRow(f: f)
                        .contentShape(Rectangle())
                        .onTapGesture { onEdit(f) }
                }
            }
            .listStyle(.inset)
        }
    }
}

private struct IncomesList: View {
    @Environment(AppState.self) private var state
    let onEdit: (Income) -> Void
    var body: some View {
        if state.incomes.isEmpty {
            EmptyHint(label: "还没有收入", systemImage: "arrow.down.circle", hint: "右上 + 添加")
        } else {
            List {
                ForEach(state.incomes) { i in
                    IncomeRow(i: i)
                        .contentShape(Rectangle())
                        .onTapGesture { onEdit(i) }
                }
            }
            .listStyle(.inset)
        }
    }
}

private struct BillRow: View {
    let b: Bill
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(b.name).font(.body)
                Spacer()
                Text(Money.format(b.monthlyAmount)).font(.body.monospacedDigit())
            }
            Text("\(b.startedMonth) 起 · \(b.totalMonths) 期")
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
    }
}

private struct FixedRow: View {
    let f: FixedExpense
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(f.name).font(.body)
                Spacer()
                Text(Money.format(f.monthlyAmount)).font(.body.monospacedDigit())
            }
            Text("\(f.startedMonth) 起 · 无止期")
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
    }
}

private struct FreeRow: View {
    let f: FreeExpense
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(f.note.isEmpty ? f.date : f.note).font(.body)
                Spacer()
                Text(Money.format(f.amount)).font(.body.monospacedDigit())
            }
            if !f.note.isEmpty {
                Text(f.date).font(.caption).foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}

private struct IncomeRow: View {
    let i: Income
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(i.note.isEmpty ? i.date : i.note).font(.body)
                Spacer()
                Text(Money.format(i.amount))
                    .font(.body.monospacedDigit())
                    .foregroundStyle(.green)
            }
            if !i.note.isEmpty {
                Text(i.date).font(.caption).foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}

private struct EmptyHint: View {
    let label: String
    let systemImage: String
    let hint: String
    var body: some View {
        ContentUnavailableView {
            Label(label, systemImage: systemImage)
        } description: {
            Text(hint)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
