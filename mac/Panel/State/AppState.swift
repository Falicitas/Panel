import Foundation
import Observation
import SwiftUI

enum ContentMode: String { case todolist, micro }

@MainActor
@Observable
final class AppState {
    var selectedTab: AppTab = .todolist
    var contentMode: ContentMode = .todolist
    var isDetailLoading: Bool = false
    /// 专注模式:折叠 sidebar + content 列,只显示 detail。
    var splitVisibility: NavigationSplitViewVisibility = .all
    var editorPageZoom: Double = AppState.loadZoom() {
        didSet { UserDefaults.standard.set(editorPageZoom, forKey: Self.zoomKey) }
    }
    private static let zoomKey = "editor.pageZoom"
    private static func loadZoom() -> Double {
        (UserDefaults.standard.object(forKey: zoomKey) as? Double) ?? 1.0
    }
    var active: [ListEntry] = []
    var done: [ListEntry] = []
    var anchor: String?
    var selectedName: String?
    var focus: FocusResult?
    var detail: Todolist?
    var editorBuffer: String = ""
    var microBuffer: String = ""
    var microLoadedFor: String?
    var pendingScrollText: String?
    var microToday: Todolist?
    var microProgress: MicroProgress?
    var recurring: [RecurringTodo] = []
    var financeSummary: FinanceSummary?
    var financeSettings: FinanceSettings?
    var bills: [Bill] = []
    var fixed: [FixedExpense] = []
    var frees: [FreeExpense] = []
    var incomes: [Income] = []
    var supplements: [Supplement] = []
    var supplementsToday: SupplementToday?
    var dailyState: DailyState?
    var stateSettings: StateSettings?
    var notifications: [PanelNotification] = []
    /// 通知点击跳转 trigger: SupplementsView 监听此值,非 nil 时滚到对应 item id 并清空。
    var pendingScrollSupplement: Int64?
    var grip: GripSummary?
    var supplementMonthly: SupplementMonthly?
    /// 英文练习 - 主响应 (含 currentAudio + segments + windows + 累计)
    var englishToday: EnglishToday?
    /// 英文练习 - settings (windows / threshold)
    var englishSettings: EnglishSettings?
    /// 英文练习 - 复习队列今日响应 (按 audio 分组). nil = 未加载, groups 空 = 今日无到期。
    var englishReviewToday: EnglishReviewToday?
    /// 英文练习 - 锚定段 (全局唯一). F5/F6 全局热键作用于此段; nil = 未锚定 (热键 beep 提示).
    /// source 区分锚定来自 learn / review view, 让播放进度只在对应 view 显示。
    /// 当前不持久化, 重启 Panel 丢失 (用户需重新点 📌)。
    var englishAnchor: EnglishAnchor?
    /// 锚定段的字幕显示开关. F10 全局热键 toggle 这个值, 锚定 row 用它决定 textArea 显示。
    /// 切换锚定段时 reset = false (新锚段默认隐藏字幕)。非锚定段仍用 row 本地 @State。
    var anchorShowText: Bool = false
    /// 跳过模态框: 是否显示 (绑给 .sheet)。主动点跳过 / 自动归档触发都走这条。
    var showSkipSheet: Bool = false
    /// 模态框当前候选 audio (含 segments / description preview)。fetch 中或抽完时为 nil。
    var skipSheetCandidate: EnglishCandidateResp?
    /// 模态框当前选中的 topic (空串 = "全部" 不过滤)。
    var skipSheetTopic: String = ""
    /// 已"再抽"过的 audio ID 列表; 累积传给 server 防重抽到同一条。
    var skipSheetExcludeIDs: [Int64] = []
    /// fetch candidate 中标识, UI 显示 spinner。
    var skipSheetIsLoading: Bool = false
    var recordsTab: RecordsTab = .grip
    var bashTranscript: [BashHistoryEntry] = []
    var bashHistory: [BashHistoryEntry] = []
    var bashIsRunning = false
    var timeTick: Date = Date()
    var isLoading = false
    var lastError: String?

    /// 当前有效 mode (考虑 sleep/focus 自动转换 + 优先级 sleep > outside > focus > normal)。
    /// effectiveMode 不直接修改 dailyState.mode (那个是用户显式设置的)。
    var effectiveMode: String {
        let _ = timeTick  // 触发 SwiftUI 跟踪,timeTick 变化时 view 重计算
        guard let ds = dailyState, let ss = stateSettings else { return "normal" }
        let nowMin = minutesOfDay(Date())
        let sleepMin = hmToMinutes(ss.sleepStart)
        let focusMin = hmToMinutes(ss.focusStart)
        if nowMin >= sleepMin { return "sleep" }
        if ds.mode == "outside" { return "outside" }
        if nowMin >= focusMin && nowMin < sleepMin { return "focus" }
        return ds.mode
    }

    /// 主题色: 普通无 / 专注柔和橙 / 睡眠淡紫 / 外出蓝
    var themeColor: Color? {
        switch effectiveMode {
        case "focus":   return Color.anchorAccent
        case "sleep":   return Color(red: 0.65, green: 0.55, blue: 0.85)
        case "outside": return Color(red: 0.40, green: 0.65, blue: 0.95)
        default:        return nil
        }
    }

    var modeLabel: String {
        switch effectiveMode {
        case "focus":   return "专注"
        case "sleep":   return "睡眠"
        case "outside": return "外出"
        default:        return "普通"
        }
    }

    private func minutesOfDay(_ d: Date) -> Int {
        let c = Calendar.current.dateComponents([.hour, .minute], from: d)
        return (c.hour ?? 0) * 60 + (c.minute ?? 0)
    }

    private func hmToMinutes(_ hm: String) -> Int {
        let parts = hm.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return 0 }
        return h * 60 + m
    }

    private let api: any BackendClient
    private var pollTask: Task<Void, Never>?

    init(api: any BackendClient = APIClient.shared) {
        self.api = api
    }

    func loadAll() async {
        isLoading = true
        defer { isLoading = false }
        do {
            async let lists = api.listTodolists()
            async let anchorResp = api.getAnchor()
            async let focusResp = api.getFocus()
            async let microResp = api.getMicroToday()
            async let microProgResp = api.getMicroProgress()
            async let recurResp = api.listRecurring()
            async let summaryResp = api.getFinanceSummary(at: nil)
            async let settingsResp = api.getFinanceSettings()
            async let billsResp = api.listBills()
            async let fixedResp = api.listFixed()
            async let freesResp = api.listFree()
            async let incomesResp = api.listIncomes()
            async let supplResp = api.listSupplements()
            async let supplTodayResp = api.getSupplementsToday()
            async let dailyResp = api.getTodayState()
            async let stateSetResp = api.getStateSettings()
            async let notifResp = api.listNotifications(all: false)
            async let gripResp = api.getGripToday()
            async let suppMonthResp = api.getSupplementsMonth(month: nil)
            async let engTodayResp = api.getEnglishToday()
            async let engSettingsResp = api.getEnglishSettings()
            async let engReviewResp = api.getEnglishReviewToday()
            let (l, a, f, m, mp, r) = try await (lists, anchorResp, focusResp, microResp, microProgResp, recurResp)
            let (sum, set, bs, fx, fr, inc) = try await (summaryResp, settingsResp, billsResp, fixedResp, freesResp, incomesResp)
            let (sp, st, ds, ss, ns) = try await (supplResp, supplTodayResp, dailyResp, stateSetResp, notifResp)
            let (gp, sm) = try await (gripResp, suppMonthResp)
            // english 用 try? 让旧 backend 缺 endpoint 时 fallback nil, 不阻断其他模块加载
            let engT = try? await engTodayResp
            let engS = try? await engSettingsResp
            let engR = try? await engReviewResp
            self.active = l.active
            self.done = l.done
            // anchor 校验: 文件可能在 Finder 里被改名/删除, backend 还存旧 anchor;
            // 用最新 lists 校验, 失效则当作无 anchor 并通知 backend 清掉, 避免后续 fetch 撞 404
            let validNames = Set(l.active.map(\.name)).union(l.done.map(\.name))
            let effectiveAnchor: String? = {
                guard let n = a.name else { return nil }
                return validNames.contains(n) ? n : nil
            }()
            if a.name != nil && effectiveAnchor == nil {
                _ = try? await api.setAnchor(name: nil)
            }
            self.anchor = effectiveAnchor
            self.focus = f
            self.microToday = m
            self.microProgress = mp
            self.recurring = r
            self.financeSummary = sum
            self.financeSettings = set
            self.bills = bs
            self.fixed = fx
            self.frees = fr
            self.incomes = inc
            self.supplements = sp
            self.supplementsToday = st
            self.dailyState = ds
            self.stateSettings = ss
            self.notifications = ns
            self.grip = gp
            self.supplementMonthly = sm
            self.englishToday = engT
            self.englishSettings = engS
            self.englishReviewToday = engR
            self.selectedName = effectiveAnchor
            if let name = effectiveAnchor {
                self.detail = try? await api.getTodolist(name: name)
            } else {
                self.detail = nil
            }
            self.editorBuffer = self.detail?.rawMD ?? ""
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - Finance ops

    func reloadFinance() async {
        do {
            async let sum = api.getFinanceSummary(at: nil)
            async let bs = api.listBills()
            async let fx = api.listFixed()
            async let fr = api.listFree()
            async let inc = api.listIncomes()
            async let set = api.getFinanceSettings()
            let (s, b, f, fre, i, st) = try await (sum, bs, fx, fr, inc, set)
            self.financeSummary = s
            self.bills = b
            self.fixed = f
            self.frees = fre
            self.incomes = i
            self.financeSettings = st
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveBill(_ b: Bill, isNew: Bool) async {
        do {
            if isNew {
                _ = try await api.createBill(b)
            } else {
                _ = try await api.updateBill(b)
            }
            await reloadFinance()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteBill(id: Int64) async {
        do {
            try await api.deleteBill(id: id)
            await reloadFinance()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveFixed(_ f: FixedExpense, isNew: Bool) async {
        do {
            if isNew { _ = try await api.createFixed(f) } else { _ = try await api.updateFixed(f) }
            await reloadFinance()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteFixed(id: Int64) async {
        do { try await api.deleteFixed(id: id); await reloadFinance() }
        catch { self.lastError = error.localizedDescription }
    }

    func saveFree(_ f: FreeExpense, isNew: Bool) async {
        do {
            if isNew { _ = try await api.createFree(f) } else { _ = try await api.updateFree(f) }
            await reloadFinance()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteFree(id: Int64) async {
        do { try await api.deleteFree(id: id); await reloadFinance() }
        catch { self.lastError = error.localizedDescription }
    }

    func saveIncome(_ i: Income, isNew: Bool) async {
        do {
            if isNew { _ = try await api.createIncome(i) } else { _ = try await api.updateIncome(i) }
            await reloadFinance()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteIncome(id: Int64) async {
        do { try await api.deleteIncome(id: id); await reloadFinance() }
        catch { self.lastError = error.localizedDescription }
    }

    func saveFinanceSettings(_ fs: FinanceSettings) async {
        do {
            self.financeSettings = try await api.putFinanceSettings(fs)
            self.financeSummary = try await api.getFinanceSummary(at: nil)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - Supplements / State / Notifications

    func reloadSupplements() async {
        do {
            async let listR = api.listSupplements()
            async let todayR = api.getSupplementsToday()
            // backend 在 supplements CRUD / enable / pause 后立即跑 GenerateNotifications,
            // 顺手把 notifications 也拉一次, 通知栏不必等 5s tick 才看到新通知。
            async let notifsR = api.listNotifications(all: false)
            self.supplements = try await listR
            self.supplementsToday = try await todayR
            if let ns = try? await notifsR { self.notifications = ns }
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 切换某项的某个 window 的今日"已吃"状态。
    func toggleSupplementWindowCheck(_ item: SupplementTodayItem, windowIndex: Int) async {
        guard windowIndex >= 0, windowIndex < item.windows.count else { return }
        let nowDone = item.windows[windowIndex].doneToday
        do {
            try await api.checkSupplement(
                id: item.id,
                date: nil,
                windowIndex: windowIndex,
                checked: !nowDone
            )
            await reloadSupplements()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 把某项的某个 window 当日 snooze count + 1 (30 分钟槽), effective end 同步推后 30min。
    /// UI 触发 — 跟 mac banner '延后 30 分钟' 同 backend 路径; 用户可点多次累加。
    /// 实际确认 (二次 confirmationDialog) 由 view 层处理, 这里只调 api。
    func snoozeSupplementWindow(_ item: SupplementTodayItem, windowIndex: Int) async {
        guard windowIndex >= 0, windowIndex < item.windows.count else { return }
        let date = supplementsToday?.date
        do {
            try await api.snoozeSupplement(
                id: item.id,
                date: date,
                windowIndex: windowIndex,
                count: 1
            )
            await reloadSupplements()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 暂停 N 天 (now + days*86400);days = 0 取消暂停。
    func pauseSupplement(_ sp: Supplement, days: Int) async {
        let until: Int64? = days > 0
            ? Int64(Date().timeIntervalSince1970) + Int64(days) * 86400
            : nil
        do {
            self.supplements = try await api.pauseSupplement(id: sp.id, until: until)
            self.supplementsToday = try await api.getSupplementsToday()
            await reloadNotifications()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 切换 enabled。
    func setSupplementEnabled(_ sp: Supplement, enabled: Bool) async {
        do {
            self.supplements = try await api.enableSupplement(id: sp.id, enabled: enabled)
            self.supplementsToday = try await api.getSupplementsToday()
            await reloadNotifications()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveSupplement(_ sp: Supplement, isNew: Bool) async {
        do {
            if isNew {
                _ = try await api.createSupplement(sp)
            } else {
                _ = try await api.updateSupplement(sp)
            }
            await reloadSupplements()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteSupplement(id: Int64) async {
        do {
            try await api.deleteSupplement(id: id)
            await reloadSupplements()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 把 sp 在当前列表里向 direction 方向移动一步, 然后调 reorder API 全列表保存。
    /// direction = -1 上移, +1 下移; 越界 (已是首/末) 时 no-op。
    func moveSupplement(_ sp: Supplement, direction: Int) async {
        guard direction == -1 || direction == 1 else { return }
        var items = self.supplements
        guard let idx = items.firstIndex(where: { $0.id == sp.id }) else { return }
        let target = idx + direction
        guard target >= 0, target < items.count else { return }
        items.swapAt(idx, target)
        let order = items.map(\.id)
        do {
            self.supplements = try await api.reorderSupplements(orderedIDs: order)
            self.supplementsToday = try await api.getSupplementsToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func setMode(_ mode: String, wakeTime: String? = nil) async {
        do {
            self.dailyState = try await api.setTodayState(mode: mode, wakeTime: wakeTime)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveStateSettings(_ ss: StateSettings) async {
        do {
            self.stateSettings = try await api.putStateSettings(ss)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 触发某餐:meal ∈ {"breakfast","lunch","dinner"};time=nil 用现在,clear=true 清空。
    func triggerMeal(_ meal: String, time: String? = nil, clear: Bool = false) async {
        do {
            self.dailyState = try await api.setMeal(meal: meal, time: time, clear: clear)
            self.supplementsToday = try await api.getSupplementsToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 暂停某餐 N 天 (now + days*86400);days = 0 取消暂停。跟 pauseSupplement 同构。
    /// 后端会 dismiss 当前 active 通知 (暂停场景) 或 undismiss + 重生成 (取消场景), 双向绑定。
    func pauseMeal(_ meal: String, days: Int) async {
        let until: Int64? = days > 0
            ? Int64(Date().timeIntervalSince1970) + Int64(days) * 86400
            : nil
        do {
            self.dailyState = try await api.pauseMeal(meal: meal, until: until)
            await reloadNotifications()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 永久禁止 / 启用某餐 (跟 setSupplementEnabled 同构, 跟 paused 两轴独立)。
    /// 禁用: dismiss 当前通知; 启用: undismiss + 重生成。
    func setMealEnabled(_ meal: String, enabled: Bool) async {
        do {
            self.dailyState = try await api.enableMeal(meal: meal, enabled: enabled)
            await reloadNotifications()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func dismiss(_ n: PanelNotification) async {
        do {
            try await api.dismissNotification(id: n.id)
            async let notifs = api.listNotifications(all: false)
            async let monthly = api.getSupplementsMonth(month: nil)
            self.notifications = try await notifs
            self.supplementMonthly = try? await monthly
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func reloadNotifications() async {
        do {
            self.notifications = try await api.listNotifications(all: false)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - English

    /// 同时只跑 1 个 audio 的 ASR; 防 tick / 启动重复触发同一 audio。
    private var asrInProgressIDs: Set<Int64> = []
    /// 本 session 内 ASR 失败的 audio.id, 不再自动重试 (避免烧钱循环)。
    /// (M3 起 backend.segmentsStatus=='failed' 也会拦, 跨重启也不重试。)
    private var asrFailedIDs: Set<Int64> = []

    func reloadEnglishToday() async {
        do {
            self.englishToday = try await api.getEnglishToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 拉取精听复习队列今日响应 (按 audio 分组). 失败 fallback nil + 记 lastError, 不抛。
    func reloadEnglishReview() async {
        do {
            self.englishReviewToday = try await api.getEnglishReviewToday()
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 复习项推进到下一阶段 (advance). 完成后 reload review + today (today 头部数字要更新)。
    func reviewMarkDone(itemID: Int64) async {
        do {
            try await api.reviewMarkDone(itemID: itemID)
            await reloadEnglishReview()
            await reloadEnglishToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 延后复习项 (默认 +1 天). 完成后 reload review (today 头部计数不变)。
    func reviewPostpone(itemID: Int64, toDate: String? = nil) async {
        do {
            try await api.reviewPostpone(itemID: itemID, toDate: toDate)
            await reloadEnglishReview()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 永久跳过复习项 (用户二次确认后调用). 完成后 reload review。
    func reviewFinish(itemID: Int64) async {
        do {
            try await api.reviewFinish(itemID: itemID)
            await reloadEnglishReview()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - English anchor (F5/F6 全局热键的目标段)

    /// toggle 锚定: 已锚定同段 → 取消; 否则锚定为新段 (替换之前的锚定)。
    /// 全局只有一个 anchor — 锚定别处自动取消旧锚。
    /// 切换锚定段 (含取消) 时 anchorShowText reset = false, 新锚段从隐藏字幕开始。
    func toggleEnglishAnchor(audioID: Int64, segIdx: Int, source: PlaySource) {
        let want = EnglishAnchor(audioID: audioID, segIdx: segIdx, source: source)
        if englishAnchor == want {
            englishAnchor = nil
        } else {
            englishAnchor = want
        }
        anchorShowText = false
    }

    /// set 锚定 (替换语义, 不 toggle). Shift+F7/F8 切换上下复习 audio 用。
    func setEnglishAnchor(audioID: Int64, segIdx: Int, source: PlaySource) {
        englishAnchor = EnglishAnchor(audioID: audioID, segIdx: segIdx, source: source)
        anchorShowText = false
    }

    func clearEnglishAnchor() {
        englishAnchor = nil
        anchorShowText = false
    }

    /// 当前 audio 若 status=pending 且 local 文件存在 + 本 session 没失败过, 后台启动转写,
    /// 完成后 PUT segments 回 backend + 刷新 today。失败把 audio.id 加进 asrFailedIDs,
    /// tick 后续不再重试 (防止无限失败循环烧钱)。
    /// 严格判断 status == "pending" — backend 已标 'failed' (静音占位 / ASR 失败) 或
    /// 'running' / 'ready' 都跳过, 重启后也不重试 ('failed' 跨 session 留住状态)。
    func ensureSegmentsForCurrentAudio() {
        guard let today = englishToday,
              let audio = today.currentAudio,
              audio.segmentsStatus == "pending",
              !audio.localPath.isEmpty,
              !asrInProgressIDs.contains(audio.id),
              !asrFailedIDs.contains(audio.id) else { return }
        asrInProgressIDs.insert(audio.id)
        Task {
            await transcribeAndUpload(audio)
            asrInProgressIDs.remove(audio.id)
        }
    }

    private func transcribeAndUpload(_ audio: EnglishAudio) async {
        let url = URL(fileURLWithPath: audio.localPath)
        do {
            let segs = try await SpeechSegmenter.transcribe(url: url)
            try await api.putEnglishSegments(audioID: audio.id, segments: segs)
            await reloadEnglishToday()
        } catch {
            asrFailedIDs.insert(audio.id)  // 本 session 不再重试
            self.lastError = "ASR 失败: \(error.localizedDescription)"
        }
    }

    /// 切换某 segment 的勾选状态。
    /// 勾上 → POST checked=true, seconds = (endMs - startMs) / 1000 (>= 1s 防 0)
    /// 取消 → POST checked=false (后端 DeleteRecord)
    /// 后端会自动: 重算 window minutes / dismiss-undismiss notif。
    /// (M2 起后端不再自动归档, 改成 mac 检测全勾完弹跳过模态框。)
    func toggleEnglishSegment(audioID: Int64, segment: EnglishSegment, currentlyDone: Bool) async {
        let secs = max(1.0, Double(segment.endMs - segment.startMs) / 1000.0)
        do {
            try await api.checkEnglishSegment(
                audioID: audioID,
                segIdx: segment.idx,
                checked: !currentlyDone,
                seconds: secs
            )
            await reloadEnglishToday()
            // 勾选/取消勾选副作用是入队/退队复习, 顺手 refresh 复习区。
            await reloadEnglishReview()
            self.lastError = nil
            // 全勾完 → 弹跳过模态框 (取代 backend 自动归档)
            if let today = englishToday,
               let audio = today.currentAudio,
               audio.id == audioID,
               !today.segments.isEmpty,
               today.doneSegIdx.count >= today.segments.count {
                openSkipSheet()
            }
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - English Skip Sheet

    /// 打开跳过模态框 (主动点跳过 / 自动归档触发都走这条)。
    /// 默认 topic = current audio 的 topic, 重置 exclude, 异步 fetch 第一个候选。
    func openSkipSheet() {
        skipSheetTopic = englishToday?.currentAudio?.topic ?? ""
        skipSheetCandidate = nil
        skipSheetExcludeIDs = []
        showSkipSheet = true
        Task { await fetchSkipCandidate() }
    }

    /// 用当前 sheet 状态 (topic + exclude + currentID) fetch 一个候选。
    /// 失败把 candidate 置 nil + 显示 lastError。
    func fetchSkipCandidate() async {
        skipSheetIsLoading = true
        defer { skipSheetIsLoading = false }
        let curID = englishToday?.currentAudio?.id
        do {
            let r = try await api.candidateEnglishAudio(
                topic: skipSheetTopic,
                excludeIDs: skipSheetExcludeIDs,
                currentID: curID
            )
            skipSheetCandidate = r
            self.lastError = nil
        } catch {
            skipSheetCandidate = nil
            self.lastError = error.localizedDescription
        }
    }

    /// 用户改 topic — push 到 state, 重置 exclude (新 topic 不背老 exclude), refetch。
    func setSkipSheetTopic(_ topic: String) async {
        skipSheetTopic = topic
        skipSheetExcludeIDs = []
        await fetchSkipCandidate()
    }

    /// "再抽" — 把当前候选加进 excludeIDs, refetch 同 topic 下次新一条。
    func rerollSkipCandidate() async {
        if let curID = skipSheetCandidate?.audio?.id {
            skipSheetExcludeIDs.append(curID)
        }
        await fetchSkipCandidate()
    }

    /// 用户确认切到 selected. POST /skip + 关 sheet + reload today。
    func confirmSkipToAudio(selectedAudioID: Int64) async {
        guard let curID = englishToday?.currentAudio?.id else { return }
        do {
            _ = try await api.skipEnglishAudio(
                audioID: curID,
                selectedAudioID: selectedAudioID,
                topic: skipSheetTopic
            )
            showSkipSheet = false
            skipSheetCandidate = nil
            skipSheetExcludeIDs = []
            await reloadEnglishToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 取消模态框 — 不归档, 重置 sheet state。
    func cancelSkipSheet() {
        showSkipSheet = false
        skipSheetCandidate = nil
        skipSheetExcludeIDs = []
    }

    // MARK: - Records

    func saveGrip(date: String, left: Double, right: Double, note: String = "") async {
        do {
            self.grip = try await api.postGrip(date: date, left: left, right: right, note: note)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func reloadGrip() async {
        do {
            self.grip = try await api.getGripToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func reloadSupplementMonthly(month: String? = nil) async {
        do {
            self.supplementMonthly = try await api.getSupplementsMonth(month: month)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func saveMicroToday(rawMD: String) async {
        do {
            self.microToday = try await api.putMicroToday(rawMD: rawMD)
            self.microProgress = try await api.getMicroProgress()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func addRecurring(content: String, kind: RecurringKind) async {
        do {
            _ = try await api.createRecurring(content: content, kind: kind)
            self.recurring = try await api.listRecurring()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 切换某条 recurring 的 kind (会自动放到目标段末尾)。
    func setRecurringKind(_ item: RecurringTodo, _ kind: RecurringKind) async {
        guard item.kind != kind else { return }
        do {
            self.recurring = try await api.updateRecurring(id: item.id, content: item.content, kind: kind)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 重排某 kind 段内的 recurring 顺序。ids 应仅含目标 kind 的项。
    func reorderRecurring(kind: RecurringKind, ids: [Int64]) async {
        do {
            self.recurring = try await api.reorderRecurring(kind: kind, ids: ids)
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func deleteRecurring(_ item: RecurringTodo) async {
        do {
            try await api.deleteRecurring(id: item.id)
            self.recurring = try await api.listRecurring()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func setAnchor(name: String?) async {
        do {
            _ = try await api.setAnchor(name: name)
            self.anchor = name
            self.focus = try? await api.getFocus()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 新建 todolist:dirty buffer 先存,后端创建空 .md,刷新列表 + 选中并锚定新文件。
    func createTodolist(name: String) async {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        if let cur = detail, editorBuffer != cur.rawMD {
            await saveDetail()
        }
        do {
            let tl = try await api.createTodolist(name: trimmed)
            let lists = try await api.listTodolists()
            self.active = lists.active
            self.done = lists.done
            self.detail = tl
            self.editorBuffer = tl.rawMD
            self.selectedName = tl.name
            self.contentMode = .todolist
            // 自动锚定新建的 todolist
            _ = try? await api.setAnchor(name: tl.name)
            self.anchor = tl.name
            self.focus = try? await api.getFocus()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 切换右栏显示的 todolist。不影响 anchor / focus。
    /// 切换前若当前 editorBuffer 比 detail.rawMD 新,先保存,避免丢失。
    /// 副作用:把 contentMode 切回 .todolist (用户点 sidebar 项 = 离开 micro 模式)。
    func selectList(_ name: String?) async {
        if let cur = detail, editorBuffer != cur.rawMD {
            await saveDetail()
        }
        contentMode = .todolist
        selectedName = name
        guard let name else {
            detail = nil
            editorBuffer = ""
            return
        }
        isDetailLoading = true
        defer { isDetailLoading = false }
        do {
            detail = try await api.getTodolist(name: name)
            editorBuffer = detail?.rawMD ?? ""
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// 聚焦卡 L{n} 链接:
    /// - todolist 模式:若不在锚定 todolist 上,先切回 anchor,再发 scroll trigger
    /// - micro 模式:直接发 scroll trigger (micro 隐含锚定为当日文件)
    /// 用 focus 自带的 text 作 scroll 目标 (parser 已剥 markdown 标记的干净文本),
    /// 不再走 lineNo→raw md 反推。MilkDownEditor 监听 pendingScrollText 触发。
    func jumpToFocus(_ text: String?) async {
        guard let text, !text.isEmpty else { return }
        if contentMode == .micro {
            pendingScrollText = text
            return
        }
        guard let target = anchor else { return }
        if selectedName != target {
            await selectList(target)
        }
        pendingScrollText = text
    }

    /// 通知点击 → 切到 supplements tab + 让 SupplementsView 滚到对应 item id。
    /// 不自动 dismiss 通知;用户决定何时清掉。
    func jumpToSupplement(id: Int64) {
        selectedTab = .supplements
        // 同 id 重复点击需要先重置,否则 onChange 不触发
        if pendingScrollSupplement == id {
            pendingScrollSupplement = nil
        }
        pendingScrollSupplement = id
    }

    /// 通知点击 → 切到 english tab。english 是 window-based 不是 item-based,
    /// 不需要 scroll 到具体 item, 切 tab 就让用户看到 today / current audio / segments。
    func jumpToEnglish() {
        selectedTab = .english
    }

    /// 三餐通知点击 → 切到 supplements tab。三餐 card 在 SupplementsView 顶部 MealsSection,
    /// 不需要 scroll, 切 tab 用户就能看到。
    func jumpToMeals() {
        selectedTab = .supplements
    }

    /// 从其它 tab (保健品 / 英文) 一键回到 todolist 当前 focus 行 — 完成杂事直奔关注。
    /// 三步:
    ///   1) selectedTab = .todolist + contentMode = .todolist (不是 micro)
    ///   2) splitVisibility = .doubleColumn — 系统 'Hide Sidebar' 按钮同款 state,
    ///      MainLayout 现有 collapseContent 逻辑会把 content 列宽也一并收为 0,
    ///      视觉等价 .detailOnly (sidebar+content 都掩盖, 只剩 editor)。
    ///   3) jumpToFocus(focus.text) — 切到 anchor todolist + 滚到该行
    ///      MilkDownEditor 在 webView ready (handleDidFinish) 时若 scrollText 仍 pending
    ///      会自动消费一次, 解决首次切到 todolist tab 时 editor 还没 ready 跳不到行的问题。
    /// focus 不存在或文本空 → no-op (按钮也会 disabled)。
    func jumpToTodolistFocus() async {
        guard let text = focus?.text, !text.isEmpty else { return }
        selectedTab = .todolist
        contentMode = .todolist
        splitVisibility = .doubleColumn
        await jumpToFocus(text)
    }

    /// 当 todolist 未锚定 / anchor 无 focus 时, JumpToFocusButton 的 fallback:
    /// 跳到当日 micro 第一个未完成项。流程同 jumpToTodolistFocus, 区别在 contentMode = .micro,
    /// jumpToFocus 内部走 micro 分支 (隐含锚定为当日, 不需 selectList)。
    /// micro 无未完成项 → no-op (按钮 disabled)。
    func jumpToMicroFocus() async {
        guard let text = microFocusText, !text.isEmpty else { return }
        selectedTab = .todolist
        contentMode = .micro
        splitVisibility = .doubleColumn
        await jumpToFocus(text)
    }

    /// 当日 micro 第一个未完成项的 text (跳过 checked 和 skip), 用于 JumpToFocusButton
    /// 在 todolist 无 focus 时 fallback 到 micro。语义对齐 DetailView.microFocus(today:)。
    var microFocusText: String? {
        guard let today = microToday else { return nil }
        guard let first = today.items.first(where: { !$0.checked && !$0.skip }) else { return nil }
        return first.text
    }

    /// 改今日精听阈值 (分钟)。复用 PUT /api/english/settings, windows / enabled 保留。
    func setEnglishThresholdMin(_ min: Int) async {
        guard min > 0, min <= 240 else { return }
        guard var es = englishSettings else { return }
        guard es.thresholdMin != min else { return }
        es.thresholdMin = min
        do {
            self.englishSettings = try await api.putEnglishSettings(es)
            await reloadEnglishToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 整体保存英文 settings (windows / threshold / enabled), 用于 EnglishSettingsSheet。
    /// backend 已 kick GenerateNotifications, 通知 body 会同步到新 windows 边界。
    func saveEnglishSettings(_ es: EnglishSettings) async {
        do {
            self.englishSettings = try await api.putEnglishSettings(es)
            await reloadEnglishToday()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    /// 保存当前 detail。若 rawMD 不传,默认保存 editorBuffer 当前内容。
    func saveDetail(rawMD: String? = nil) async {
        guard let name = detail?.name else { return }
        let content = rawMD ?? editorBuffer
        guard content != detail?.rawMD else { return }  // 无变化跳过
        do {
            self.detail = try await api.putTodolist(name: name, rawMD: content)
            self.editorBuffer = self.detail?.rawMD ?? content
            self.focus = try await api.getFocus()
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    // MARK: - Bash

    func execCommand(_ cmd: String, timeoutMs: Int? = nil) async {
        let trimmed = cmd.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        bashIsRunning = true
        do {
            let result = try await api.execCommand(trimmed, timeoutMs: timeoutMs)
            bashTranscript.append(result.asHistoryEntry)
            bashHistory = try await api.listBashHistory(limit: 100)
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
        bashIsRunning = false
    }

    func loadBashHistory(limit: Int = 100) async {
        do {
            bashHistory = try await api.listBashHistory(limit: limit)
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func loadBashEntryToTranscript(_ entry: BashHistoryEntry) {
        bashTranscript.append(entry)
    }

    /// 5s tick: 1) 把 editorBuffer / microBuffer 的 dirty 内容写回后端
    /// 2) 拉外部修改 (Typora 等改了 .md 文件) 同步到 panel
    func startPolling() async {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(3))
                await self?.tick()
            }
        }
        await pollTask?.value
    }

    private func tick() async {
        self.timeTick = Date()

        // 1. autosave dirty buffers (先 save 再 fetch,避免 fetch 把 dirty buffer 覆盖)
        if let cur = detail, editorBuffer != cur.rawMD {
            await saveDetail()
        }
        if let mt = microToday, microLoadedFor == mt.name, microBuffer != mt.rawMD {
            do {
                self.microToday = try await api.putMicroToday(rawMD: microBuffer)
                self.microBuffer = self.microToday?.rawMD ?? microBuffer
                self.microProgress = try await api.getMicroProgress()
            } catch {
                self.lastError = error.localizedDescription
            }
        }

        // 2. poll 外部修改
        do {
            if let name = selectedName {
                let d = try await api.getTodolist(name: name)
                if d != self.detail {
                    self.detail = d
                    if editorBuffer != (d.rawMD) {
                        editorBuffer = d.rawMD
                    }
                }
            }
            if anchor != nil {
                let f = try await api.getFocus()
                if f != self.focus { self.focus = f }
            }
            async let micro = api.getMicroToday()
            async let microProg = api.getMicroProgress()
            async let notif = api.listNotifications(all: false)
            async let daily = api.getTodayState()
            async let supplToday = api.getSupplementsToday()
            async let listsResp = api.listTodolists()
            async let engToday = api.getEnglishToday()
            async let engReview = api.getEnglishReviewToday()
            let (m, mp, ns, ds, st, lists) = try await (micro, microProg, notif, daily, supplToday, listsResp)
            let engT = try? await engToday
            let engR = try? await engReview
            if st != self.supplementsToday { self.supplementsToday = st }
            if let engT, engT != self.englishToday { self.englishToday = engT }
            if let engR, engR != self.englishReviewToday { self.englishReviewToday = engR }
            // tick 也尝试触发 ASR — 用户首次启动后切到英文 tab 才看到 audio,
            // 此时 ensureSegmentsForCurrentAudio 把它推上 ASR 队列
            ensureSegmentsForCurrentAudio()
            // sidebar 进度 + 外部新建/删除的 .md 实时反映 (3s)
            if lists.active != self.active { self.active = lists.active }
            if lists.done != self.done { self.done = lists.done }
            if m != self.microToday {
                self.microToday = m
                if microLoadedFor == m.name && microBuffer != m.rawMD {
                    microBuffer = m.rawMD
                }
            }
            if mp != self.microProgress { self.microProgress = mp }
            if ns != self.notifications { self.notifications = ns }
            if ds != self.dailyState { self.dailyState = ds }
        } catch {
            // 静默,等下次
        }
        // 紧急提醒: 订阅-消费模型, backend notification 是状态 source of truth;
        // mac 系统通知按 lead range fire / done / dismiss / snooze 联动 panel 通知栏。
        // 两类 source 共用一套机制, baseKey 与 backend source_ref 完全一致。
        UrgentNotifier.shared.check(
            supplementsToday: self.supplementsToday,
            englishToday: self.englishToday,
            notifications: self.notifications
        )
    }
}
