import Foundation
import UserNotifications
import os.log

/// 紧急提醒: 在 supplements / english 两类 window 的 [end-15min, end) 区间内若仍未完成,
/// 通过 UNUserNotificationCenter 弹 macOS 系统通知 (Banner) 并附两个 action:
///   - "知道了" / 隐式 ✕  → dismiss (不再骚扰, 直到 doneToday/reached 翻转)
///   - "延后 30 分钟"      → snooze (window.end + 30·N min 处再弹一次, 可无限叠)
///
/// 两类 source 共享一套 fire / cancel / dismiss 通路, 通过 userInfo["kind"] 区分:
///   - "supplement": baseKey = "supplement:{itemID}:window:{idx}:date:{date}"
///   - "english":    baseKey = "english:window:{idx}:date:{date}"
/// baseKey 与 backend source_ref 完全一致, 双向消费状态来自同一字段。
///
/// done / undone 对称: 用户在 panel 勾完 → cancel 该 baseKey 全部 pending+delivered
/// (含 snooze 链) + 移除 firedKey; 取消勾 → 下次 tick 看到未达 + 已无 firedKey,
/// 若仍在 lead range 会重新 fire (一次新的 lead 提醒, snooze 链不复活)。
///
/// snooze 通知到时若已达成 → willPresent 返 [] 不显示。
@MainActor
final class UrgentNotifier: NSObject, @unchecked Sendable {
    static let shared = UrgentNotifier()
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "notify.urgent")

    private static let categoryID = "panel.urgent"
    private static let snoozeActionID = "snooze.30min"
    private static let dismissActionID = "dismiss"

    private let leadSeconds: TimeInterval = 15 * 60
    private let snoozeSeconds: TimeInterval = 30 * 60

    private var firedKeys: Set<String> = []
    private var firedDate: String = ""
    private var permissionRequested = false
    /// willPresent / snooze / dismiss 回调要查最新状态, 缓存最近 check 快照。
    private var lastSupplementsToday: SupplementToday?
    private var lastEnglishToday: EnglishToday?
    private var lastNotifications: [PanelNotification] = []

    /// userInfo["kind"] 区分两类 source。
    private enum NotifKind: String {
        case supplement
        case english
    }

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
    func check(
        supplementsToday: SupplementToday?,
        englishToday: EnglishToday?,
        notifications: [PanelNotification]
    ) {
        ensurePermission()
        lastSupplementsToday = supplementsToday
        lastEnglishToday = englishToday
        lastNotifications = notifications

        // 跨日清理: 任一 today 的 date 翻转就重置 firedKeys 并清掉 pending+delivered。
        let date = supplementsToday?.date ?? englishToday?.date ?? ""
        if !date.isEmpty, date != firedDate {
            firedDate = date
            firedKeys.removeAll()
            cancelAllPending()
        }

        checkSupplementsInner(today: supplementsToday, notifications: notifications)
        checkEnglishInner(today: englishToday, notifications: notifications)
    }

    // MARK: - supplements

    private func checkSupplementsInner(today: SupplementToday?, notifications: [PanelNotification]) {
        guard let today else { return }
        let now = Date()
        for item in today.items {
            for (idx, win) in item.windows.enumerated() {
                let key = supplementBaseKey(itemID: item.id, windowIndex: idx, date: today.date)
                let notif = notifications.first(where: { $0.sourceRef == key })

                let consumed = win.doneToday || (notif?.dismissedAt != nil)
                if consumed {
                    if firedKeys.contains(key) { firedKeys.remove(key) }
                    cancelPending(forBaseKey: key)
                    continue
                }

                guard item.enabled, !item.paused else { continue }
                guard win.requiresMeal.isEmpty else { continue }
                guard let notif else { continue }

                guard let endTime = parseTodayHM(win.endHM, base: now) else { continue }
                let leadStart = endTime.addingTimeInterval(-leadSeconds)
                guard now >= leadStart, now < endTime else { continue }
                guard !firedKeys.contains(key) else { continue }

                firedKeys.insert(key)
                let label = item.type == "supplement" ? "保健品" : "食物"
                fireGeneric(
                    key: key,
                    title: item.name,
                    body: "\(label) 时段 \(win.startHM)–\(win.endHM)",
                    endTime: endTime,
                    snoozeCount: 0,
                    notificationID: notif.id,
                    kind: .supplement,
                    extraUserInfo: [
                        "supplementID": item.id,
                        "windowIndex": findSupplementWindowIndex(in: item, matching: win),
                    ]
                )
            }
        }
    }

    // MARK: - english

    private func checkEnglishInner(today: EnglishToday?, notifications: [PanelNotification]) {
        guard let today else { return }
        let now = Date()
        for win in today.windows {
            let key = englishBaseKey(windowIndex: win.index, date: today.date)
            let notif = notifications.first(where: { $0.sourceRef == key })

            let consumed = win.reached || (notif?.dismissedAt != nil)
            if consumed {
                if firedKeys.contains(key) { firedKeys.remove(key) }
                cancelPending(forBaseKey: key)
                continue
            }

            guard let notif else { continue }
            // win.endHM 已经是 effective end (= original + snoozeCount * 30min) - 后端做的。
            // mac 端只需在 [endHM-15min, endHM) 内 fire 一次, 无需自行计算 snooze 偏移。
            guard let endTime = parseTodayHM(win.endHM, base: now) else { continue }
            let leadStart = endTime.addingTimeInterval(-leadSeconds)
            guard now >= leadStart, now < endTime else { continue }
            guard !firedKeys.contains(key) else { continue }

            firedKeys.insert(key)
            let remaining = max(0, Double(today.thresholdMin) - win.minutesDone)
            fireGeneric(
                key: key,
                title: "英文精听",
                body: String(format: "时段 %@–%@, 还差 %.1f 分钟", win.startHM, win.endHM, remaining),
                endTime: endTime,
                snoozeCount: win.snoozeCount,
                notificationID: notif.id,
                kind: .english,
                extraUserInfo: [
                    "englishWindowIndex": win.index,
                    "englishDate": today.date,
                ]
            )
        }
    }

    // MARK: - private fire / cancel

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

    private func supplementBaseKey(itemID: Int64, windowIndex: Int, date: String) -> String {
        "supplement:\(itemID):window:\(windowIndex):date:\(date)"
    }

    private func englishBaseKey(windowIndex: Int, date: String) -> String {
        "english:window:\(windowIndex):date:\(date)"
    }

    /// snoozeCount=0 立即 fire (trigger=nil); snoozeCount>0 按 endTime + N·30min schedule。
    /// notificationID 是 backend notification.id, dismiss action 调 backend dismiss API 用。
    private func fireGeneric(
        key: String,
        title: String,
        body: String,
        endTime: Date,
        snoozeCount: Int,
        notificationID: Int64,
        kind: NotifKind,
        extraUserInfo: [String: Any]
    ) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        content.categoryIdentifier = Self.categoryID
        content.interruptionLevel = .timeSensitive
        content.relevanceScore = 1.0
        var info: [String: Any] = [
            "kind": kind.rawValue,
            "baseKey": key,
            "windowEndUnix": endTime.timeIntervalSince1970,
            "snoozeCount": snoozeCount,
            "notificationID": notificationID,
        ]
        for (k, v) in extraUserInfo { info[k] = v }
        content.userInfo = info

        let trigger: UNNotificationTrigger?
        let identifier: String
        if snoozeCount == 0 {
            trigger = nil
            identifier = key
        } else {
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

    /// 在 item.windows 里找到 startHM/endHM 跟 win 一致的下标 — 防 mac 端数据异步刷掉时错位。
    private func findSupplementWindowIndex(in item: SupplementTodayItem, matching win: SupplementWindowStatus) -> Int {
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

    // MARK: - consumed checks (for willPresent / snooze)

    fileprivate func isSupplementWindowDone(itemID: Int64, windowIndex: Int) -> Bool {
        guard let today = lastSupplementsToday else { return false }
        guard let item = today.items.first(where: { $0.id == itemID }) else { return false }
        guard windowIndex >= 0, windowIndex < item.windows.count else { return false }
        return item.windows[windowIndex].doneToday
    }

    fileprivate func isEnglishWindowReached(windowIndex: Int) -> Bool {
        guard let today = lastEnglishToday else { return false }
        return today.windows.first(where: { $0.index == windowIndex })?.reached ?? false
    }

    // MARK: - response handlers (snooze / dismiss)

    fileprivate func snoozeFromResponse(_ response: UNNotificationResponse) {
        let info = response.notification.request.content.userInfo
        guard let kindRaw = info["kind"] as? String,
              let kind = NotifKind(rawValue: kindRaw) else { return }
        switch kind {
        case .supplement:
            snoozeSupplementResponse(info: info)
        case .english:
            snoozeEnglishResponse(info: info)
        }
    }

    private func snoozeSupplementResponse(info: [AnyHashable: Any]) {
        guard let baseKey = info["baseKey"] as? String,
              let itemID = info["supplementID"] as? Int64,
              let windowIndex = info["windowIndex"] as? Int,
              let endUnix = info["windowEndUnix"] as? TimeInterval,
              let snoozeCount = info["snoozeCount"] as? Int,
              let notificationID = info["notificationID"] as? Int64
        else { return }
        if isSupplementWindowDone(itemID: itemID, windowIndex: windowIndex) { return }
        guard let today = lastSupplementsToday,
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
                os_log("snooze supplement err: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }

        let label = item.type == "supplement" ? "保健品" : "食物"
        fireGeneric(
            key: baseKey,
            title: item.name,
            body: "\(label) 时段 \(win.startHM)–\(win.endHM)",
            endTime: endTime,
            snoozeCount: snoozeCount + 1,
            notificationID: notificationID,
            kind: .supplement,
            extraUserInfo: [
                "supplementID": itemID,
                "windowIndex": windowIndex,
            ]
        )
    }

    private func snoozeEnglishResponse(info: [AnyHashable: Any]) {
        guard let baseKey = info["baseKey"] as? String,
              let windowIndex = info["englishWindowIndex"] as? Int,
              let endUnix = info["windowEndUnix"] as? TimeInterval,
              let snoozeCount = info["snoozeCount"] as? Int,
              let notificationID = info["notificationID"] as? Int64
        else { return }
        if isEnglishWindowReached(windowIndex: windowIndex) { return }
        guard let today = lastEnglishToday,
              let win = today.windows.first(where: { $0.index == windowIndex })
        else { return }
        let endTime = Date(timeIntervalSince1970: endUnix)
        let date = today.date
        let remaining = max(0, Double(today.thresholdMin) - win.minutesDone)

        Task {
            do {
                try await APIClient.shared.snoozeEnglishWindow(idx: windowIndex, date: date)
            } catch {
                os_log("snooze english err: %{public}@", log: Self.log, type: .error, String(describing: error))
            }
        }

        fireGeneric(
            key: baseKey,
            title: "英文精听",
            body: String(format: "时段 %@–%@, 还差 %.1f 分钟", win.startHM, win.endHM, remaining),
            endTime: endTime,
            snoozeCount: snoozeCount + 1,
            notificationID: notificationID,
            kind: .english,
            extraUserInfo: [
                "englishWindowIndex": windowIndex,
                "englishDate": date,
            ]
        )
    }

    /// 用户在 mac banner 点 "知道了" 或隐式 ✕ 时调; 同步调 backend dismiss API,
    /// panel 内通知栏对应一条也消失 (订阅-消费双向)。两类 source 共用。
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
        // snooze trigger 到时若已达成 → 不打扰。
        let info = notification.request.content.userInfo
        let kindRaw = info["kind"] as? String
        Task { @MainActor in
            let consumed: Bool
            switch kindRaw {
            case NotifKind.supplement.rawValue:
                let id = info["supplementID"] as? Int64
                let idx = info["windowIndex"] as? Int
                if let id, let idx {
                    consumed = self.isSupplementWindowDone(itemID: id, windowIndex: idx)
                } else { consumed = false }
            case NotifKind.english.rawValue:
                if let idx = info["englishWindowIndex"] as? Int {
                    consumed = self.isEnglishWindowReached(windowIndex: idx)
                } else { consumed = false }
            default:
                consumed = false
            }
            completionHandler(consumed ? [] : [.banner, .sound])
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
                self.dismissFromResponse(response)
            default:
                break
            }
            completionHandler()
        }
    }
}
