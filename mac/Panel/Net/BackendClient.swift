import Foundation

/// Abstracts the panel backend HTTP surface so AppState/UI can be tested with mocks.
/// Production binding is `APIClient.shared` (see `Net/APIClient.swift`).
protocol BackendClient: Sendable {
    // MARK: Todolist
    func listTodolists() async throws -> TodolistListResponse
    func getTodolist(name: String) async throws -> Todolist
    func putTodolist(name: String, rawMD: String) async throws -> Todolist
    func createTodolist(name: String) async throws -> Todolist

    // MARK: Anchor / Focus
    func getAnchor() async throws -> AnchorResponse
    func setAnchor(name: String?) async throws -> AnchorResponse
    func getFocus() async throws -> FocusResult

    // MARK: Micro
    func getMicroToday() async throws -> Todolist
    func putMicroToday(rawMD: String) async throws -> Todolist
    func getMicroProgress() async throws -> MicroProgress

    // MARK: Recurring
    func listRecurring() async throws -> [RecurringTodo]
    func createRecurring(content: String, kind: RecurringKind) async throws -> RecurringTodo
    func updateRecurring(id: Int64, content: String, kind: RecurringKind) async throws -> [RecurringTodo]
    func reorderRecurring(kind: RecurringKind, ids: [Int64]) async throws -> [RecurringTodo]
    func deleteRecurring(id: Int64) async throws

    // MARK: Finance
    func getFinanceSummary(at: String?) async throws -> FinanceSummary
    func listBills() async throws -> [Bill]
    func createBill(_ b: Bill) async throws -> Bill
    func updateBill(_ b: Bill) async throws -> [Bill]
    func deleteBill(id: Int64) async throws
    func listFixed() async throws -> [FixedExpense]
    func createFixed(_ f: FixedExpense) async throws -> FixedExpense
    func updateFixed(_ f: FixedExpense) async throws -> [FixedExpense]
    func deleteFixed(id: Int64) async throws
    func listFree() async throws -> [FreeExpense]
    func createFree(_ f: FreeExpense) async throws -> FreeExpense
    func updateFree(_ f: FreeExpense) async throws -> [FreeExpense]
    func deleteFree(id: Int64) async throws
    func listIncomes() async throws -> [Income]
    func createIncome(_ i: Income) async throws -> Income
    func updateIncome(_ i: Income) async throws -> [Income]
    func deleteIncome(id: Int64) async throws
    func getFinanceSettings() async throws -> FinanceSettings
    func putFinanceSettings(_ fs: FinanceSettings) async throws -> FinanceSettings

    // MARK: Supplements
    func listSupplements() async throws -> [Supplement]
    func createSupplement(_ s: Supplement) async throws -> Supplement
    func updateSupplement(_ s: Supplement) async throws -> [Supplement]
    func deleteSupplement(id: Int64) async throws
    func getSupplementsToday() async throws -> SupplementToday
    func checkSupplement(id: Int64, date: String?, windowIndex: Int, checked: Bool) async throws
    func pauseSupplement(id: Int64, until: Int64?) async throws -> [Supplement]
    func enableSupplement(id: Int64, enabled: Bool) async throws -> [Supplement]
    func snoozeSupplement(id: Int64, date: String?, windowIndex: Int) async throws

    // MARK: State
    func getTodayState() async throws -> DailyState
    func setTodayState(mode: String, wakeTime: String?) async throws -> DailyState
    func getStateSettings() async throws -> StateSettings
    func putStateSettings(_ ss: StateSettings) async throws -> StateSettings
    func setMeal(meal: String, time: String?, clear: Bool) async throws -> DailyState

    // MARK: Notifications
    func listNotifications(all: Bool) async throws -> [PanelNotification]
    func dismissNotification(id: Int64) async throws

    // MARK: Records
    func getGripToday() async throws -> GripSummary
    func postGrip(date: String, left: Double, right: Double, note: String) async throws -> GripSummary
    func getSupplementsMonth(month: String?) async throws -> SupplementMonthly

    // MARK: Bash
    func execCommand(_ cmd: String, timeoutMs: Int?) async throws -> BashExecResult
    func listBashHistory(limit: Int) async throws -> [BashHistoryEntry]
}
