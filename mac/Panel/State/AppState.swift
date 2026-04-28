import Foundation
import Observation
import SwiftUI

@MainActor
@Observable
final class AppState {
    var selectedTab: AppTab = .todolist
    var active: [ListEntry] = []
    var done: [ListEntry] = []
    var anchor: String?
    var selectedName: String?
    var focus: FocusResult?
    var detail: Todolist?
    var pendingScrollLine: Int?
    var microToday: Todolist?
    var recurring: [RecurringTodo] = []
    var financeSummary: FinanceSummary?
    var financeSettings: FinanceSettings?
    var bills: [Bill] = []
    var fixed: [FixedExpense] = []
    var frees: [FreeExpense] = []
    var incomes: [Income] = []
    var supplements: [Supplement] = []
    var dailyState: DailyState?
    var stateSettings: StateSettings?
    var notifications: [PanelNotification] = []
    var grip: GripSummary?
    var supplementMonthly: SupplementMonthly?
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
        case "focus":   return Color(red: 0.95, green: 0.65, blue: 0.35)
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
            async let recurResp = api.listRecurring()
            async let summaryResp = api.getFinanceSummary(at: nil)
            async let settingsResp = api.getFinanceSettings()
            async let billsResp = api.listBills()
            async let fixedResp = api.listFixed()
            async let freesResp = api.listFree()
            async let incomesResp = api.listIncomes()
            async let supplResp = api.listSupplements()
            async let dailyResp = api.getTodayState()
            async let stateSetResp = api.getStateSettings()
            async let notifResp = api.listNotifications(all: false)
            async let gripResp = api.getGripToday()
            async let suppMonthResp = api.getSupplementsMonth(month: nil)
            let (l, a, f, m, r) = try await (lists, anchorResp, focusResp, microResp, recurResp)
            let (sum, set, bs, fx, fr, inc) = try await (summaryResp, settingsResp, billsResp, fixedResp, freesResp, incomesResp)
            let (sp, ds, ss, ns) = try await (supplResp, dailyResp, stateSetResp, notifResp)
            let (gp, sm) = try await (gripResp, suppMonthResp)
            self.active = l.active
            self.done = l.done
            self.anchor = a.name
            self.focus = f
            self.microToday = m
            self.recurring = r
            self.financeSummary = sum
            self.financeSettings = set
            self.bills = bs
            self.fixed = fx
            self.frees = fr
            self.incomes = inc
            self.supplements = sp
            self.dailyState = ds
            self.stateSettings = ss
            self.notifications = ns
            self.grip = gp
            self.supplementMonthly = sm
            self.selectedName = a.name
            if let name = a.name {
                self.detail = try await api.getTodolist(name: name)
            } else {
                self.detail = nil
            }
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
            self.supplements = try await api.listSupplements()
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
            self.lastError = nil
        } catch {
            self.lastError = error.localizedDescription
        }
    }

    func addRecurring(content: String) async {
        let nextOrder = (recurring.last?.sortOrder ?? -1) + 1
        do {
            _ = try await api.createRecurring(content: content, sortOrder: nextOrder)
            self.recurring = try await api.listRecurring()
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

    /// 切换右栏显示的 todolist。不影响 anchor / focus。
    func selectList(_ name: String?) async {
        selectedName = name
        guard let name else {
            detail = nil
            return
        }
        do {
            detail = try await api.getTodolist(name: name)
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// 聚焦卡 L{n} 链接:先把 selection 切回锚定列表,再发 scroll trigger。
    /// 0.8s 延迟与具体滚动动画由 DetailView 监听 pendingScrollLine 实现。
    func jumpToAnchorLine(_ line: Int) async {
        guard let target = anchor else { return }
        if selectedName != target {
            await selectList(target)
        }
        pendingScrollLine = line
    }

    func saveDetail(rawMD: String) async {
        guard let name = detail?.name else { return }
        do {
            self.detail = try await api.putTodolist(name: name, rawMD: rawMD)
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

    /// 后台 polling: 处理外部编辑器(Typora 等)修改了文件的情况。
    /// 后端 fsnotify 已经失效缓存,这里只是周期性拉取看是否有新内容。
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
        do {
            if let name = selectedName {
                let d = try await api.getTodolist(name: name)
                if d != self.detail { self.detail = d }
            }
            if anchor != nil {
                let f = try await api.getFocus()
                if f != self.focus { self.focus = f }
            }
            async let micro = api.getMicroToday()
            async let notif = api.listNotifications(all: false)
            async let daily = api.getTodayState()
            let (m, ns, ds) = try await (micro, notif, daily)
            if m != self.microToday { self.microToday = m }
            if ns != self.notifications { self.notifications = ns }
            if ds != self.dailyState { self.dailyState = ds }
        } catch {
            // 静默,等下次
        }
    }
}
