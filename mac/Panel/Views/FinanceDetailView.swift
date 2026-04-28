import SwiftUI

struct FinanceDetailView: View {
    @Environment(AppState.self) private var state
    @State private var settingsOpen = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let s = state.financeSummary {
                    BalanceCard(s: s)
                    DebtIndicator(s: s)
                    MonthSummaryCard(s: s)
                    FreeCapBar(s: s)
                } else {
                    ProgressView().frame(maxWidth: .infinity)
                }
                Spacer(minLength: 8)
                SettingsRow { settingsOpen = true }
            }
            .padding(20)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .navigationTitle("当月汇总")
        .sheet(isPresented: $settingsOpen) {
            FinanceSettingsSheet().environment(state)
        }
    }
}

private struct BalanceCard: View {
    let s: FinanceSummary
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("当前余额")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
                Text(s.asOf)
                    .font(.caption.monospacedDigit()).foregroundStyle(.tertiary)
            }
            Text(Money.format(s.balance))
                .font(.system(size: 32, weight: .semibold).monospacedDigit())
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(20)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
    }
}

private struct DebtIndicator: View {
    let s: FinanceSummary
    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: s.debtInflated ? "exclamationmark.triangle.fill" : "checkmark.circle.fill")
                .foregroundStyle(s.debtInflated ? .orange : .green)
                .imageScale(.large)
            VStack(alignment: .leading, spacing: 4) {
                Text(s.debtInflated ? "债务膨胀触发" : "本月收支健康")
                    .font(.body.weight(.medium))
                if s.debtInflated {
                    Text("差额 \(Money.format(s.shortfall)) · 虚拟新借款利息估算 \(Money.format(s.estExtraInterest))")
                        .font(.caption).foregroundStyle(.secondary)
                    Text("(假设年化 \(rateLabel)% / 期 \(s.assumedNMonths) 月,可在设置改)")
                        .font(.caption2).foregroundStyle(.tertiary)
                } else {
                    Text("当月收入 ≥ 预期支出,无需新借款")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer()
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            (s.debtInflated ? Color.orange : Color.green).opacity(0.10),
            in: RoundedRectangle(cornerRadius: 10)
        )
    }

    private var rateLabel: String {
        String(format: "%g", s.assumedAnnualRate * 100)
    }
}

private struct MonthSummaryCard: View {
    let s: FinanceSummary
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(s.currentMonth) 当月")
                .font(.caption).foregroundStyle(.secondary)
            row("当月收入", Money.format(s.monthIncome), .green)
            Divider()
            row("当月预期支出", Money.format(s.monthExpected), .primary)
            row("├ 账单分摊", Money.format(s.monthBill), .secondary)
            row("├ 固定支出", Money.format(s.monthFixed), .secondary)
            row("└ 自由支出", Money.format(s.monthFree), .secondary)
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }

    private func row(_ k: String, _ v: String, _ color: Color) -> some View {
        HStack {
            Text(k).font(.body)
            Spacer()
            Text(v).font(.body.monospacedDigit()).foregroundStyle(color)
        }
    }
}

private struct FreeCapBar: View {
    let s: FinanceSummary
    var body: some View {
        let progress = s.monthFreeCap > 0 ? Double(s.monthFree) / Double(s.monthFreeCap) : 0
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("自由支出 月上限").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Text("\(Money.format(s.monthFree)) / \(Money.format(s.monthFreeCap))")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            ProgressView(value: min(progress, 1.0))
                .tint(progress > 1.0 ? .red : (progress > 0.8 ? .orange : .accentColor))
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct SettingsRow: View {
    let onTap: () -> Void
    var body: some View {
        Button(action: onTap) {
            HStack {
                Image(systemName: "gear")
                Text("财务设置").font(.body)
                Spacer()
                Image(systemName: "chevron.right")
                    .foregroundStyle(.tertiary)
            }
            .padding(12)
        }
        .buttonStyle(.plain)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
    }
}
