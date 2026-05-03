import Foundation
import UserNotifications
import os.log

/// 监听 supplements/today 数据, 在每个 window 的最后 5 分钟内若仍未完成,
/// 通过 UNUserNotificationCenter 弹 macOS 系统通知 (Banner)。
///
/// 防重复: in-memory firedKeys (key = supplement:{id}:window:{idx}:date:{date}),
/// 跨日按 today.date 自动清空。panel 重启会丢 firedKeys, 接受重启后罕见的单次重弹。
///
/// 权限 lazy 请求 (第一次 check 时), 用户拒绝则静默不弹。
@MainActor
final class UrgentNotifier {
    static let shared = UrgentNotifier()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "notify.urgent")

    /// window 结束前多少秒触发"最后提醒"。
    private let leadSeconds: TimeInterval = 5 * 60

    private var firedKeys: Set<String> = []
    private var firedDate: String = ""
    private var permissionRequested = false

    private init() {}

    /// AppState.tick 5s 调一次。
    func check(today: SupplementToday?) {
        ensurePermission()
        guard let today else { return }
        if today.date != firedDate {
            firedDate = today.date
            firedKeys.removeAll()
        }
        let now = Date()
        for item in today.items {
            guard item.enabled, !item.paused else { continue }
            for (idx, win) in item.windows.enumerated() {
                // 等待餐触发的 window — end 时刻不可计算, 跳过
                guard win.requiresMeal.isEmpty else { continue }
                // 已勾完 → 用户已经做了, 不必弹
                guard !win.doneToday else { continue }
                guard let endTime = parseTodayHM(win.endHM, base: now) else { continue }
                let leadStart = endTime.addingTimeInterval(-leadSeconds)
                // 不在 [end-5min, end) 区间内, 跳过
                guard now >= leadStart, now < endTime else { continue }
                let key = "supplement:\(item.id):window:\(idx):date:\(today.date)"
                guard !firedKeys.contains(key) else { continue }
                firedKeys.insert(key)
                fire(key: key, item: item, win: win)
            }
        }
    }

    private func ensurePermission() {
        guard !permissionRequested else { return }
        permissionRequested = true
        Task {
            do {
                let granted = try await UNUserNotificationCenter.current()
                    .requestAuthorization(options: [.alert, .sound])
                os_log("system notif auth granted=%{public}@", log: Self.log, type: .info, String(describing: granted))
            } catch {
                os_log("system notif auth error: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }
    }

    private func fire(key: String, item: SupplementTodayItem, win: SupplementWindowStatus) {
        let content = UNMutableNotificationContent()
        let label = item.type == "supplement" ? "保健品" : "食物"
        content.title = "⏰ 还有 5 分钟: \(item.name)"
        content.body = "\(label) 时段 \(win.startHM)–\(win.endHM) 即将结束,还没勾完成"
        content.sound = .default
        let req = UNNotificationRequest(identifier: key, content: content, trigger: nil)
        UNUserNotificationCenter.current().add(req) { err in
            if let err {
                os_log("notif add err: %{public}@", log: Self.log, type: .error, String(describing: err))
            }
        }
    }

    private func parseTodayHM(_ hm: String, base: Date) -> Date? {
        let parts = hm.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return nil }
        return Calendar.current.date(bySettingHour: h, minute: m, second: 0, of: base)
    }
}
