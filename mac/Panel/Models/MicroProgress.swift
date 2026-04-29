import Foundation

/// 当日 micro 晨间进度。Total/Done = morning kind recurring 在当日 micro 里的数量与勾选数量;
/// StartAt = 第一次勾选 morning 项的 unix 时间;EndAt = 全部勾完的 unix 时间。
/// 一旦 backend 写入,不会被清掉(避免来回勾选反复触发)。
struct MicroProgress: Codable, Hashable {
    let date: String
    let morningTotal: Int
    let morningDone: Int
    let morningStartAt: Int64?
    let morningEndAt: Int64?

    enum CodingKeys: String, CodingKey {
        case date
        case morningTotal = "morning_total"
        case morningDone = "morning_done"
        case morningStartAt = "morning_start_at"
        case morningEndAt = "morning_end_at"
    }

    var hasMorning: Bool { morningTotal > 0 }
    var isComplete: Bool { morningTotal > 0 && morningDone == morningTotal }
}
