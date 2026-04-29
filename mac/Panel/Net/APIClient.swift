import Foundation

struct APIError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}

private struct RecurringPayload: Encodable {
    let content: String
    let kind: String
}

private struct ReorderPayload: Encodable {
    let kind: String
    let ids: [Int64]
}

private struct GripPayload: Encodable {
    let date: String
    let left: Double
    let right: Double
    let note: String
}

private struct ExecPayload: Encodable {
    let cmd: String
    let timeoutMs: Int?

    enum CodingKeys: String, CodingKey {
        case cmd
        case timeoutMs = "timeout_ms"
    }
}

actor APIClient: BackendClient {
    static let shared = APIClient()

    /// 字面量 URL 解析失败 = 编程错误,fatalError 即可。
    private static let baseURL: URL = {
        guard let u = URL(string: "http://127.0.0.1:8089") else {
            fatalError("invalid baseURL literal")
        }
        return u
    }()

    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    func listTodolists() async throws -> TodolistListResponse {
        try await get("/api/todolists")
    }

    func getTodolist(name: String) async throws -> Todolist {
        try await get("/api/todolists/\(encode(name))")
    }

    func putTodolist(name: String, rawMD: String) async throws -> Todolist {
        try await put("/api/todolists/\(encode(name))", body: ["raw_md": rawMD])
    }

    func getAnchor() async throws -> AnchorResponse {
        try await get("/api/anchor")
    }

    func setAnchor(name: String?) async throws -> AnchorResponse {
        let body: [String: String?] = ["name": name]
        return try await post("/api/anchor", body: body)
    }

    func getFocus() async throws -> FocusResult {
        try await get("/api/focus")
    }

    func getMicroToday() async throws -> Todolist {
        try await get("/api/micro/today")
    }

    func putMicroToday(rawMD: String) async throws -> Todolist {
        try await put("/api/micro/today", body: ["raw_md": rawMD])
    }

    func getMicroProgress() async throws -> MicroProgress {
        try await get("/api/micro/today/progress")
    }

    func listRecurring() async throws -> [RecurringTodo] {
        try await get("/api/recurring")
    }

    func createRecurring(content: String, kind: RecurringKind) async throws -> RecurringTodo {
        try await post("/api/recurring", body: RecurringPayload(content: content, kind: kind.rawValue))
    }

    func updateRecurring(id: Int64, content: String, kind: RecurringKind) async throws -> [RecurringTodo] {
        try await put("/api/recurring/\(id)", body: RecurringPayload(content: content, kind: kind.rawValue))
    }

    func reorderRecurring(kind: RecurringKind, ids: [Int64]) async throws -> [RecurringTodo] {
        try await post("/api/recurring/reorder", body: ReorderPayload(kind: kind.rawValue, ids: ids))
    }

    func deleteRecurring(id: Int64) async throws {
        try await delete("/api/recurring/\(id)")
    }

    // MARK: - Finance

    func getFinanceSummary(at: String? = nil) async throws -> FinanceSummary {
        var path = "/api/finance/summary"
        if let at = at { path += "?at=\(at)" }
        return try await get(path)
    }

    func listBills() async throws -> [Bill] { try await get("/api/finance/bills") }
    func createBill(_ b: Bill) async throws -> Bill { try await post("/api/finance/bills", body: b) }
    func updateBill(_ b: Bill) async throws -> [Bill] { try await put("/api/finance/bills/\(b.id)", body: b) }
    func deleteBill(id: Int64) async throws { try await delete("/api/finance/bills/\(id)") }

    func listFixed() async throws -> [FixedExpense] { try await get("/api/finance/fixed") }
    func createFixed(_ f: FixedExpense) async throws -> FixedExpense { try await post("/api/finance/fixed", body: f) }
    func updateFixed(_ f: FixedExpense) async throws -> [FixedExpense] { try await put("/api/finance/fixed/\(f.id)", body: f) }
    func deleteFixed(id: Int64) async throws { try await delete("/api/finance/fixed/\(id)") }

    func listFree() async throws -> [FreeExpense] { try await get("/api/finance/free") }
    func createFree(_ f: FreeExpense) async throws -> FreeExpense { try await post("/api/finance/free", body: f) }
    func updateFree(_ f: FreeExpense) async throws -> [FreeExpense] { try await put("/api/finance/free/\(f.id)", body: f) }
    func deleteFree(id: Int64) async throws { try await delete("/api/finance/free/\(id)") }

    func listIncomes() async throws -> [Income] { try await get("/api/finance/incomes") }
    func createIncome(_ i: Income) async throws -> Income { try await post("/api/finance/incomes", body: i) }
    func updateIncome(_ i: Income) async throws -> [Income] { try await put("/api/finance/incomes/\(i.id)", body: i) }
    func deleteIncome(id: Int64) async throws { try await delete("/api/finance/incomes/\(id)") }

    func getFinanceSettings() async throws -> FinanceSettings { try await get("/api/finance/settings") }
    func putFinanceSettings(_ fs: FinanceSettings) async throws -> FinanceSettings {
        try await put("/api/finance/settings", body: fs)
    }

    // MARK: - Supplements

    func listSupplements() async throws -> [Supplement] { try await get("/api/supplements") }
    func createSupplement(_ s: Supplement) async throws -> Supplement {
        try await post("/api/supplements", body: s)
    }
    func updateSupplement(_ s: Supplement) async throws -> [Supplement] {
        try await put("/api/supplements/\(s.id)", body: s)
    }
    func deleteSupplement(id: Int64) async throws { try await delete("/api/supplements/\(id)") }

    // MARK: - State

    func getTodayState() async throws -> DailyState { try await get("/api/state/today") }
    func setTodayState(mode: String, wakeTime: String? = nil) async throws -> DailyState {
        var body: [String: String] = ["mode": mode]
        if let w = wakeTime { body["wake_time"] = w }
        return try await post("/api/state/today", body: body)
    }
    func getStateSettings() async throws -> StateSettings { try await get("/api/state/settings") }
    func putStateSettings(_ ss: StateSettings) async throws -> StateSettings {
        try await put("/api/state/settings", body: ss)
    }

    // MARK: - Notifications

    func listNotifications(all: Bool = false) async throws -> [PanelNotification] {
        try await get(all ? "/api/notifications?all=1" : "/api/notifications")
    }
    // MARK: - Records (grip + supplement monthly)

    func getGripToday() async throws -> GripSummary {
        try await get("/api/records/grip/today")
    }

    func postGrip(date: String, left: Double, right: Double, note: String = "") async throws -> GripSummary {
        try await post("/api/records/grip", body: GripPayload(date: date, left: left, right: right, note: note))
    }

    func getSupplementsMonth(month: String? = nil) async throws -> SupplementMonthly {
        var path = "/api/records/supplements/month"
        if let m = month { path += "?month=\(m)" }
        return try await get(path)
    }

    // MARK: - Bash

    func execCommand(_ cmd: String, timeoutMs: Int? = nil) async throws -> BashExecResult {
        try await post("/api/exec", body: ExecPayload(cmd: cmd, timeoutMs: timeoutMs))
    }

    func listBashHistory(limit: Int = 100) async throws -> [BashHistoryEntry] {
        try await get("/api/exec/history?limit=\(limit)")
    }

    func dismissNotification(id: Int64) async throws {
        var req = URLRequest(url: try url("/api/notifications/\(id)/dismiss"))
        req.httpMethod = "POST"
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "dismiss notification \(id) failed")
        }
    }

    // MARK: - HTTP helpers

    private func encode(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? s
    }

    private func url(_ path: String) throws -> URL {
        guard let u = URL(string: path, relativeTo: APIClient.baseURL) else {
            throw APIError(message: "invalid URL path: \(path)")
        }
        return u
    }

    private func get<T: Decodable>(_ path: String) async throws -> T {
        var req = URLRequest(url: try url(path))
        req.httpMethod = "GET"
        return try await send(req)
    }

    private func put<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        var req = URLRequest(url: try url(path))
        req.httpMethod = "PUT"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(body)
        return try await send(req)
    }

    private func post<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        var req = URLRequest(url: try url(path))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(body)
        return try await send(req)
    }

    private func delete(_ path: String) async throws {
        var req = URLRequest(url: try url(path))
        req.httpMethod = "DELETE"
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "DELETE \(path) failed")
        }
    }

    private func send<T: Decodable>(_ req: URLRequest) async throws -> T {
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse else {
            throw APIError(message: "non-HTTP response")
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw APIError(message: "HTTP \(http.statusCode): \(body)")
        }
        return try decoder.decode(T.self, from: data)
    }
}
