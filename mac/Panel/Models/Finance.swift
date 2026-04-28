import Foundation

// 金额单位: 分 (Int64)。日期: YYYY-MM-DD;月份: YYYY-MM。

struct Bill: Codable, Identifiable, Hashable {
    let id: Int64
    var name: String
    var monthlyAmount: Int64
    var totalMonths: Int
    var startedMonth: String

    enum CodingKeys: String, CodingKey {
        case id, name
        case monthlyAmount = "monthly_amount"
        case totalMonths = "total_months"
        case startedMonth = "started_month"
    }
}

struct FixedExpense: Codable, Identifiable, Hashable {
    let id: Int64
    var name: String
    var monthlyAmount: Int64
    var startedMonth: String

    enum CodingKeys: String, CodingKey {
        case id, name
        case monthlyAmount = "monthly_amount"
        case startedMonth = "started_month"
    }
}

struct FreeExpense: Codable, Identifiable, Hashable {
    let id: Int64
    var date: String
    var amount: Int64
    var note: String
}

struct Income: Codable, Identifiable, Hashable {
    let id: Int64
    var date: String
    var amount: Int64
    var note: String
}

struct FinanceSettings: Codable, Hashable {
    var initialBalance: Int64
    var freeCapMonthly: Int64
    var assumedNMonths: Int
    var assumedAnnualRate: Double

    enum CodingKeys: String, CodingKey {
        case initialBalance = "initial_balance"
        case freeCapMonthly = "free_cap_monthly"
        case assumedNMonths = "assumed_n_months"
        case assumedAnnualRate = "assumed_annual_rate"
    }
}

struct FinanceSummary: Codable, Hashable {
    let asOf: String
    let currentMonth: String
    let balance: Int64
    let initialBalance: Int64
    let totalIncome: Int64
    let totalBillSpent: Int64
    let totalFixedSpent: Int64
    let totalFreeSpent: Int64
    let monthExpected: Int64
    let monthBill: Int64
    let monthFixed: Int64
    let monthFree: Int64
    let monthFreeCap: Int64
    let monthIncome: Int64
    let shortfall: Int64
    let estExtraInterest: Int64
    let assumedNMonths: Int
    let assumedAnnualRate: Double
    let debtInflated: Bool

    enum CodingKeys: String, CodingKey {
        case balance, shortfall
        case asOf = "as_of"
        case currentMonth = "current_month"
        case initialBalance = "initial_balance"
        case totalIncome = "total_income"
        case totalBillSpent = "total_bill_spent"
        case totalFixedSpent = "total_fixed_spent"
        case totalFreeSpent = "total_free_spent"
        case monthExpected = "month_expected"
        case monthBill = "month_bill"
        case monthFixed = "month_fixed"
        case monthFree = "month_free"
        case monthFreeCap = "month_free_cap"
        case monthIncome = "month_income"
        case estExtraInterest = "est_extra_interest"
        case assumedNMonths = "assumed_n_months"
        case assumedAnnualRate = "assumed_annual_rate"
        case debtInflated = "debt_inflated"
    }
}

enum Money {
    /// 分 → "¥1,234.56" 显示用
    static func format(_ cents: Int64) -> String {
        let yuan = Double(cents) / 100.0
        let f = NumberFormatter()
        f.numberStyle = .currency
        f.currencyCode = "CNY"
        f.currencySymbol = "¥"
        f.maximumFractionDigits = 2
        f.minimumFractionDigits = (cents % 100 == 0) ? 0 : 2
        return f.string(from: NSNumber(value: yuan)) ?? "¥\(yuan)"
    }

    /// "1234.56" 元 → 分 (Int64);失败返回 nil
    static func parseYuanToCents(_ s: String) -> Int64? {
        let t = s.trimmingCharacters(in: .whitespaces)
        guard let v = Double(t) else { return nil }
        return Int64((v * 100).rounded())
    }
}
