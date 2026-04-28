import SwiftUI

// MARK: - Blanks

extension Bill {
    static var blank: Bill {
        Bill(id: 0, name: "", monthlyAmount: 0, totalMonths: 12, startedMonth: nextMonthString())
    }
}

extension FixedExpense {
    static var blank: FixedExpense {
        FixedExpense(id: 0, name: "", monthlyAmount: 0, startedMonth: thisMonthString())
    }
}

extension FreeExpense {
    static var blank: FreeExpense {
        FreeExpense(id: 0, date: todayString(), amount: 0, note: "")
    }
}

extension Income {
    static var blank: Income {
        Income(id: 0, date: todayString(), amount: 0, note: "")
    }
}

private func todayString() -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM-dd"
    return f.string(from: Date())
}

private func thisMonthString() -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM"
    return f.string(from: Date())
}

private func nextMonthString() -> String {
    let f = DateFormatter()
    f.dateFormat = "yyyy-MM"
    let d = Calendar.current.date(byAdding: .month, value: 1, to: Date()) ?? Date()
    return f.string(from: d)
}

// MARK: - Reusable input rows

private struct LabelColumn<Content: View>: View {
    let label: String
    @ViewBuilder var content: Content
    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label).frame(width: 90, alignment: .leading)
            content
        }
    }
}

private struct YuanField: View {
    let label: String
    @Binding var cents: Int64
    @State private var text: String = ""

    var body: some View {
        LabelColumn(label: label) {
            HStack {
                Text("¥").foregroundStyle(.secondary)
                TextField("0.00", text: $text)
                    .textFieldStyle(.roundedBorder)
                    .onAppear {
                        text = cents > 0 ? String(format: "%.2f", Double(cents) / 100.0) : ""
                    }
                    .onChange(of: text) { _, new in
                        if new.isEmpty { cents = 0 }
                        else if let c = Money.parseYuanToCents(new) { cents = c }
                    }
            }
        }
    }
}

private struct TextRow: View {
    let label: String
    let placeholder: String
    @Binding var text: String
    var body: some View {
        LabelColumn(label: label) {
            TextField(placeholder, text: $text)
                .textFieldStyle(.roundedBorder)
        }
    }
}

private struct IntRow: View {
    let label: String
    @Binding var value: Int
    @State private var text: String = ""
    var body: some View {
        LabelColumn(label: label) {
            TextField("0", text: $text)
                .textFieldStyle(.roundedBorder)
                .onAppear { text = String(value) }
                .onChange(of: text) { _, new in
                    if new.isEmpty { value = 0 }
                    else if let v = Int(new) { value = v }
                }
        }
    }
}

// MARK: - Form scaffold

private struct FormScaffold<Content: View>: View {
    let title: String
    let canSave: Bool
    let onSave: () -> Void
    let onDelete: (() -> Void)?
    @ViewBuilder var content: Content

    @Environment(\.dismiss) private var dismiss
    @State private var confirmingDelete = false

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text(title).font(.headline)
                Spacer()
                Button("取消") { dismiss() }
                Button("保存") {
                    onSave()
                    dismiss()
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .disabled(!canSave)
            }
            .padding()
            Divider()
            VStack(spacing: 12) { content }
                .padding()
            Spacer(minLength: 20)
            if let onDelete = onDelete {
                Divider()
                HStack {
                    Spacer()
                    Button(role: .destructive) {
                        confirmingDelete = true
                    } label: {
                        Label("删除", systemImage: "trash")
                    }
                    .confirmationDialog("确认删除?", isPresented: $confirmingDelete) {
                        Button("删除", role: .destructive) {
                            onDelete()
                            dismiss()
                        }
                        Button("取消", role: .cancel) {}
                    }
                }
                .padding()
            }
        }
        .frame(minWidth: 460, minHeight: 340)
    }
}

// MARK: - Bill / Fixed / Free / Income sheets

struct BillFormSheet: View {
    let initial: Bill
    let isNew: Bool
    @Environment(AppState.self) private var state
    @State private var item: Bill = .blank

    var body: some View {
        FormScaffold(
            title: isNew ? "新增账单" : "编辑账单",
            canSave: !item.name.trimmingCharacters(in: .whitespaces).isEmpty
                && item.monthlyAmount > 0 && item.totalMonths > 0,
            onSave: { Task { await state.saveBill(item, isNew: isNew) } },
            onDelete: isNew ? nil : { Task { await state.deleteBill(id: initial.id) } }
        ) {
            TextRow(label: "名称", placeholder: "信用卡A / 房贷 / ...", text: $item.name)
            YuanField(label: "月供 (含息)", cents: $item.monthlyAmount)
            IntRow(label: "总月数", value: $item.totalMonths)
            TextRow(label: "起始月", placeholder: "YYYY-MM", text: $item.startedMonth)
        }
        .onAppear { item = initial }
    }
}

struct FixedFormSheet: View {
    let initial: FixedExpense
    let isNew: Bool
    @Environment(AppState.self) private var state
    @State private var item: FixedExpense = .blank

    var body: some View {
        FormScaffold(
            title: isNew ? "新增固定支出" : "编辑固定支出",
            canSave: !item.name.trimmingCharacters(in: .whitespaces).isEmpty && item.monthlyAmount > 0,
            onSave: { Task { await state.saveFixed(item, isNew: isNew) } },
            onDelete: isNew ? nil : { Task { await state.deleteFixed(id: initial.id) } }
        ) {
            TextRow(label: "名称", placeholder: "Spotify / 房租 / ...", text: $item.name)
            YuanField(label: "月供", cents: $item.monthlyAmount)
            TextRow(label: "起始月", placeholder: "YYYY-MM", text: $item.startedMonth)
        }
        .onAppear { item = initial }
    }
}

struct FreeFormSheet: View {
    let initial: FreeExpense
    let isNew: Bool
    @Environment(AppState.self) private var state
    @State private var item: FreeExpense = .blank

    var body: some View {
        FormScaffold(
            title: isNew ? "新增自由支出" : "编辑自由支出",
            canSave: item.amount > 0,
            onSave: { Task { await state.saveFree(item, isNew: isNew) } },
            onDelete: isNew ? nil : { Task { await state.deleteFree(id: initial.id) } }
        ) {
            TextRow(label: "日期", placeholder: "YYYY-MM-DD", text: $item.date)
            YuanField(label: "金额", cents: $item.amount)
            TextRow(label: "备注", placeholder: "(可空)", text: $item.note)
        }
        .onAppear { item = initial }
    }
}

struct IncomeFormSheet: View {
    let initial: Income
    let isNew: Bool
    @Environment(AppState.self) private var state
    @State private var item: Income = .blank

    var body: some View {
        FormScaffold(
            title: isNew ? "新增收入" : "编辑收入",
            canSave: item.amount > 0,
            onSave: { Task { await state.saveIncome(item, isNew: isNew) } },
            onDelete: isNew ? nil : { Task { await state.deleteIncome(id: initial.id) } }
        ) {
            TextRow(label: "日期", placeholder: "YYYY-MM-DD", text: $item.date)
            YuanField(label: "金额", cents: $item.amount)
            TextRow(label: "备注", placeholder: "工资 / 奖金 / ...", text: $item.note)
        }
        .onAppear { item = initial }
    }
}

// MARK: - Settings sheet

struct FinanceSettingsSheet: View {
    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var item = FinanceSettings(
        initialBalance: 0, freeCapMonthly: 100000,
        assumedNMonths: 12, assumedAnnualRate: 0.12
    )
    @State private var rateText: String = "0.12"

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("财务设置").font(.headline)
                Spacer()
                Button("取消") { dismiss() }
                Button("保存") {
                    Task {
                        await state.saveFinanceSettings(item)
                        dismiss()
                    }
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
            }
            .padding()
            Divider()
            VStack(spacing: 12) {
                YuanField(label: "初始余额", cents: $item.initialBalance)
                YuanField(label: "free 上限/月", cents: $item.freeCapMonthly)
                IntRow(label: "假设 N 月", value: $item.assumedNMonths)
                LabelColumn(label: "年利率") {
                    HStack {
                        TextField("0.12", text: $rateText)
                            .textFieldStyle(.roundedBorder)
                            .onChange(of: rateText) { _, new in
                                if let v = Double(new) { item.assumedAnnualRate = v }
                            }
                        Text("(0.12 = 12%)")
                            .font(.caption).foregroundStyle(.tertiary)
                    }
                }
            }
            .padding()
            Spacer(minLength: 20)
            Text("仅当债务膨胀触发时使用 — 用于估算\"如果差额 a 需要新借款 N 月\"对应的虚拟利息。已有 bill 的 monthly_amount 已含息,本利率不会重复计算。")
                .font(.caption2).foregroundStyle(.tertiary)
                .padding(.horizontal).padding(.bottom, 12)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(minWidth: 480, minHeight: 360)
        .onAppear {
            if let s = state.financeSettings {
                item = s
                rateText = String(format: "%g", s.assumedAnnualRate)
            }
        }
    }
}
