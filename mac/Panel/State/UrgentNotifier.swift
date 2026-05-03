import Foundation
import UserNotifications
import os.log

/// 监听 supplements/today 数据, 在每个 window 的 [end-15min, end) 区间内若仍未完成,
/// 通过 UNUserNotificationCenter 弹 macOS 系统通知 (Banner) 并附两个 action:
///   - "知道了" / 隐式 ✕  → dismiss (不再骚扰, 直到 doneToday 翻转)
///   - "延后 30 分钟"      → snooze (window.end + 30·N min 处再弹一次, 可无限叠)
///
/// done / undone 对称: 用户在 panel 勾完 → cancel 该 baseKey 全部 pending+delivered
/// (含 snooze 链) + 移除 firedKey; 用户取消勾 → 下次 tick 看到 doneToday=false 且
/// 已无 firedKey, 若仍在 lead range 会重新 fire (一次新的 lead 提醒, snooze 链不复活)。
///
/// snooze 通知到时若 doneToday=true → willPresent 返 [] 不显示。
@MainActor
final class UrgentNotifier: NSObject, @unchecked Sendable {
    static let shared = UrgentNotifier()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "notify.urgent")

    private static let categoryID = "supplement.urgent"
    private static let snoozeActionID = "snooze.30min"
    private static let dismissActionID = "dismiss"

    private let leadSeconds: TimeInterval = 15 * 60
    private let snoozeSeconds: TimeInterval = 30 * 60

    private var firedKeys: Set<String> = []
    private var firedDate: String = ""
    private var permissionRequested = false
    /// willPresent / snooze / dismiss 回调要查 doneToday 状态 + notification ref, 缓存最近 check 快照。
    private var lastToday: SupplementToday?
    private var lastNotifications: [PanelNotification] = []

    private override init() {
        super.init()
    }

    /// PanelApp 启动时调一次: 设 delegate + 注册通知 category 含 snooze/dismiss 两个 action。
    func setUp() {
        UNUserNotificationCenter.current().delegate = self
        let snooze = UNNotificationAction(
            identifier: Self.snoozeActionID,
            title: "延后 30 分钟",
            options: []
        )
        let dismiss = UNNotificationAction(
            identifier: Self.dismissActionID,
            title: "知道了",
            options: [.destructive]
        )
        let category = UNNotificationCategory(
            identifier: Self.categoryID,
            actions: [snooze, dismiss],
            intentIdentifiers: [],
            options: []
        )
        UNUserNotificationCenter.current().setNotificationCategories([category])
    }

    /// AppState.tick 5s 调一次。订阅-消费模型: backend notification.dismissed_at
    /// 是 reminder 状态的 source of truth, 任何端 dismiss / done 都会反映到这里。
    func check(today: SupplementToday?, notifications: [PanelNotification]) {
        ensurePermission()
        guard let today else { return }
        lastToday = today
        lastNotifications = notifications

        if today.date != firedDate {
            firedDate = today.date
            firedKeys.removeAll()
            cancelAllPending()
        }

        let now = Date()
        for item in today.items {
            for (idx, win) in item.windows.enumerated() {
                let key = baseKey(itemID: item.id, windowIndex: idx, date: today.date)
                let ref = key  // backend source_ref 跟 mac baseKey 同一格式
                let notif = notifications.first(where: { $0.sourceRef == ref })

                // 已 done OR backend 通知已 dismissed → 双向消费, mac 也要 cancel
                let consumed = win.doneToday || (notif?.dismissedAt != nil)
                if consumed {
                    if firedKeys.contains(key) {
                        firedKeys.remove(key)
                    }
                    cancelPending(forBaseKey: key)
                    continue
                }

                // 项被禁用 / 暂停 / 等待餐 / window 时间不可解析 → 不主动 fire (但不动 firedKeys)
                guard item.enabled, !item.paused else { continue }
                guard win.requiresMeal.isEmpty else { continue }
                // backend 还没 generate 这条 notif (window 没 active 或被 stale 清) → 不自作主张 fire
                guard let notif else { continue }

                guard let endTime = parseTodayHM(win.endHM, base: now) else { continue }
                let leadStart = endTime.addingTimeInterval(-leadSeconds)
                guard now >= leadStart, now < endTime else { continue }
                guard !firedKeys.contains(key) else { continue }

                firedKeys.insert(key)
                fire(key: key, item: item, win: win, endTime: endTime, snoozeCount: 0, notificationID: notif.id)
            }
        }
    }

    // MARK: - private

    private func ensurePermission() {
        guard !permissionRequested else { return }
        permissionRequested = true
        Task {
            do {
                _ = try await UNUserNotificationCenter.current()
                    .requestAuthorization(options: [.alert, .sound])
            } catch {
                os_log("auth err: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }
    }

    private func baseKey(itemID: Int64, windowIndex: Int, date: String) -> String {
        "supplement:\(itemID):window:\(windowIndex):date:\(date)"
    }

    /// snoozeCount=0 立即 fire (trigger=nil); snoozeCount>0 按 endTime + N·30min schedule。
    /// notificationID 是 backend notification.id, dismiss action 调 backend dismiss API 用。
    private func fire(
        key: String,
        item: SupplementTodayItem,
        win: SupplementWindowStatus,
        endTime: Date,
        snoozeCount: Int,
        notificationID: Int64
    ) {
        let content = UNMutableNotificationContent()
        let label = item.type == "supplement" ? "保健品" : "食物"
        content.title = item.name
        // body 跟 panel 内通知一致 (backend 已把 dinner+0m 等表达式解析为 HH:MM)
        content.body = "\(label) 时段 \(win.startHM)–\(win.endHM)"
        content.sound = .default
        content.categoryIdentifier = Self.categoryID
        // .timeSensitive: 突破 Focus 模式 + 系统更愿意持久显示, 比 .active 更显眼。
        // 真正"常驻直到操作"需要用户在 系统设置 → 通知 → Panel.app → 提醒样式 选
        // "提醒" (Alert), Apple 不允许 app 代码强制 alert style。
        content.interruptionLevel = .timeSensitive
        content.relevanceScore = 1.0
        content.userInfo = [
            "baseKey": key,
            "supplementID": item.id,
            "windowIndex": findWindowIndex(in: item, matching: win),
            "windowEndUnix": endTime.timeIntervalSince1970,
            "snoozeCount": snoozeCount,
            "notificationID": notificationID,
        ]

        let trigger: UNNotificationTrigger?
        let identifier: String
        if snoozeCount == 0 {
            trigger = nil
            identifier = key
        } else {
            // 第 N 次 snooze 后下次 fire = window.end + N·30min (用户语义: window 右区间 +30·N)
            let nextFireAt = endTime.addingTimeInterval(Double(snoozeCount) * snoozeSeconds)
            let interval = max(1, nextFireAt.timeIntervalSinceNow)
            trigger = UNTimeIntervalNotificationTrigger(timeInterval: interval, repeats: false)
            identifier = "\(key).snooze.\(snoozeCount)"
        }

        let req = UNNotificationRequest(identifier: identifier, content: content, trigger: trigger)
        UNUserNotificationCenter.current().add(req) { err in
            if let err {
                os_log("notif add err: %{public}@", log: Self.log, type: .error, String(describing: err))
            }
        }
    }

    /// 在 item.windows 里找到 startHM/endHM 跟 win 一致的下标 — fire 调用方传的是 enumerate
    /// 出来的 idx, 大多数情况就直接对应; 这里 defensive 兜一层避免 mac 端数据异步刷掉时错位。
    private func findWindowIndex(in item: SupplementTodayItem, matching win: SupplementWindowStatus) -> Int {
        item.windows.firstIndex(where: { $0.startHM == win.startHM && $0.endHM == win.endHM }) ?? 0
    }

    /// 找出 identifier == baseKey 或以 "{baseKey}.snooze." 开头的 pending+delivered, 一并移除。
    private func cancelPending(forBaseKey baseKey: String) {
        let center = UNUserNotificationCenter.current()
        center.getPendingNotificationRequests { reqs in
            let ids = reqs
                .filter { $0.identifier == baseKey || $0.identifier.hasPrefix("\(baseKey).snooze.") }
                .map(\.identifier)
            if !ids.isEmpty {
                center.removePendingNotificationRequests(withIdentifiers: ids)
            }
        }
        center.getDeliveredNotifications { notifs in
            let ids = notifs
                .map(\.request.identifier)
                .filter { $0 == baseKey || $0.hasPrefix("\(baseKey).snooze.") }
            if !ids.isEmpty {
                center.removeDeliveredNotifications(withIdentifiers: ids)
            }
        }
    }

    private func cancelAllPending() {
        let center = UNUserNotificationCenter.current()
        center.removeAllPendingNotificationRequests()
        center.removeAllDeliveredNotifications()
    }

    private func parseTodayHM(_ hm: String, base: Date) -> Date? {
        let parts = hm.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return nil }
        return Calendar.current.date(bySettingHour: h, minute: m, second: 0, of: base)
    }

    fileprivate func isWindowDone(itemID: Int64, windowIndex: Int) -> Bool {
        guard let today = lastToday else { return false }
        guard let item = today.items.first(where: { $0.id == itemID }) else { return false }
        guard windowIndex >= 0, windowIndex < item.windows.count else { return false }
        return item.windows[windowIndex].doneToday
    }

    /// 用户在 mac 系统通知 banner 上点 "延后 30 分钟" 时调; 已 done 则不 schedule。
    /// 同时调 backend snooze API 持久化 count, SupplementsView 的 effective end
    /// 自动反映 (= original + count·30min)。
    fileprivate func snoozeFromResponse(_ response: UNNotificationResponse) {
        let info = response.notification.request.content.userInfo
        guard let baseKey = info["baseKey"] as? String,
              let itemID = info["supplementID"] as? Int64,
              let windowIndex = info["windowIndex"] as? Int,
              let endUnix = info["windowEndUnix"] as? TimeInterval,
              let snoozeCount = info["snoozeCount"] as? Int,
              let notificationID = info["notificationID"] as? Int64
        else { return }
        if isWindowDone(itemID: itemID, windowIndex: windowIndex) { return }
        guard let today = lastToday,
              let item = today.items.first(where: { $0.id == itemID }),
              windowIndex < item.windows.count
        else { return }
        let win = item.windows[windowIndex]
        let endTime = Date(timeIntervalSince1970: endUnix)

        let date = today.date
        Task {
            do {
                try await APIClient.shared.snoozeSupplement(id: itemID, date: date, windowIndex: windowIndex)
            } catch {
                os_log("snooze api err: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }

        fire(key: baseKey, item: item, win: win, endTime: endTime, snoozeCount: snoozeCount + 1, notificationID: notificationID)
    }

    /// 用户在 mac banner 点 "知道了" 或隐式 ✕ 时调; 同步调 backend dismiss API,
    /// panel 内通知栏对应一条也消失 (订阅-消费双向)。
    fileprivate func dismissFromResponse(_ response: UNNotificationResponse) {
        let info = response.notification.request.content.userInfo
        guard let notificationID = info["notificationID"] as? Int64 else { return }
        Task {
            do {
                try await APIClient.shared.dismissNotification(id: notificationID)
            } catch {
                os_log("dismiss api err: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }
    }
}

// MARK: - UNUserNotificationCenterDelegate

extension UrgentNotifier: UNUserNotificationCenterDelegate {
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        // 即使 panel 在前台也要弹 banner; 但 snooze 通知到时若 doneToday=true 就不打扰。
        let info = notification.request.content.userInfo
        let id = info["supplementID"] as? Int64
        let idx = info["windowIndex"] as? Int
        Task { @MainActor in
            if let id, let idx, self.isWindowDone(itemID: id, windowIndex: idx) {
                completionHandler([])
            } else {
                completionHandler([.banner, .sound])
            }
        }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let actionID = response.actionIdentifier
        Task { @MainActor in
            switch actionID {
            case Self.snoozeActionID:
                self.snoozeFromResponse(response)
            case Self.dismissActionID, UNNotificationDismissActionIdentifier:
                // 显式 "知道了" 或隐式 ✕ 都同步 backend dismiss
                self.dismissFromResponse(response)
            default:
                break  // default action (点击 banner 本身) 不做事
            }
            completionHandler()
        }
    }
}
