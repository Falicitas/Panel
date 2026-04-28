import Foundation

struct GripDayValue: Codable, Hashable, Identifiable {
    let date: String
    let left: Double
    let right: Double
    let avg: Double

    var id: String { date }
}

struct GripSummary: Codable, Hashable {
    let today: GripDayValue?
    let baselineM: Double
    let sd: Double
    let baselineDays: Int
    let deviation: Double
    let classification: String
    let label: String
    let history: [GripDayValue]

    enum CodingKeys: String, CodingKey {
        case today, sd, deviation, classification, label, history
        case baselineM = "baseline_m"
        case baselineDays = "baseline_days"
    }
}

struct SupplementMonthlyEntry: Codable, Hashable, Identifiable {
    let supplementId: Int64
    let name: String
    let type: String
    let completedDays: Int
    let expectedDays: Int

    var id: Int64 { supplementId }

    enum CodingKeys: String, CodingKey {
        case name, type
        case supplementId = "supplement_id"
        case completedDays = "completed_days"
        case expectedDays = "expected_days"
    }
}

struct SupplementMonthly: Codable, Hashable {
    let month: String
    let entries: [SupplementMonthlyEntry]
}
