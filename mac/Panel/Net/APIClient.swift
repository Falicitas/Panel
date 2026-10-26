import Foundation

struct APIError: LocalizedError {
    let message: String
    /// HTTP 状态码 (拿得到时). 调用方靠它区分"真失败"和"语义上已经是想要的状态"
    /// — 例如复习项重复标完成回 409, 那条本来就已经复习过了, 刷新一下即可。
    var status: Int?
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

    func createTodolist(name: String) async throws -> Todolist {
        try await post("/api/todolists", body: ["name": name])
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

    func getSupplementsToday() async throws -> SupplementToday {
        try await get("/api/supplements/today")
    }

    func checkSupplement(id: Int64, date: String?, windowIndex: Int, checked: Bool) async throws {
        struct Body: Encodable {
            let date: String?
            let windowIndex: Int
            let checked: Bool
            enum CodingKeys: String, CodingKey {
                case date, checked
                case windowIndex = "window_index"
            }
        }
        var req = URLRequest(url: try url("/api/supplements/\(id)/check"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(Body(date: date, windowIndex: windowIndex, checked: checked))
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "check supplement \(id) failed")
        }
    }

    func pauseSupplement(id: Int64, until: Int64?) async throws -> [Supplement] {
        struct Body: Encodable { let until: Int64? }
        return try await post("/api/supplements/\(id)/pause", body: Body(until: until))
    }

    func enableSupplement(id: Int64, enabled: Bool) async throws -> [Supplement] {
        struct Body: Encodable { let enabled: Bool }
        return try await post("/api/supplements/\(id)/enable", body: Body(enabled: enabled))
    }

    func reorderSupplements(orderedIDs: [Int64]) async throws -> [Supplement] {
        struct Body: Encodable { let order: [Int64] }
        return try await post("/api/supplements/reorder", body: Body(order: orderedIDs))
    }

    func snoozeSupplement(id: Int64, date: String?, windowIndex: Int, count: Int = 1) async throws {
        struct Body: Encodable {
            let date: String?
            let windowIndex: Int
            let count: Int
            enum CodingKeys: String, CodingKey {
                case date, count
                case windowIndex = "window_index"
            }
        }
        var req = URLRequest(url: try url("/api/supplements/\(id)/snooze"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(Body(date: date, windowIndex: windowIndex, count: count))
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "snooze supplement \(id) failed")
        }
    }

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

    func setMeal(meal: String, time: String?, clear: Bool) async throws -> DailyState {
        struct Body: Encodable {
            let meal: String
            let time: String?
            let clear: Bool
        }
        return try await post("/api/state/meal", body: Body(meal: meal, time: time, clear: clear))
    }

    /// 暂停某餐: until=nil 取消暂停。跟 pauseSupplement 同构。
    /// 后端会立即跑 GenerateMealNotifications: 暂停时 dismiss 已 active 通知,
    /// 取消时 undismiss + 重新生成 (在合法时段内则立即 active)。
    func pauseMeal(meal: String, until: Int64?) async throws -> DailyState {
        struct Body: Encodable { let until: Int64? }
        return try await post("/api/state/meal/\(meal)/pause", body: Body(until: until))
    }

    /// 永久禁止 / 启用某餐 (跟 enableSupplement 同构, 跟 paused_until 两轴独立)。
    /// disable: 已 active 通知 dismiss; enable: undismiss + 立即 regen。
    func enableMeal(meal: String, enabled: Bool) async throws -> DailyState {
        struct Body: Encodable { let enabled: Bool }
        return try await post("/api/state/meal/\(meal)/enable", body: Body(enabled: enabled))
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

    // MARK: - English

    func getEnglishToday() async throws -> EnglishToday {
        try await get("/api/english/today")
    }

    func listEnglishAudios(archived: Bool) async throws -> [EnglishAudio] {
        let path = archived ? "/api/english/audios?archived=true" : "/api/english/audios"
        return try await get(path)
    }

    func getEnglishAudio(id: Int64) async throws -> EnglishAudioDetail {
        try await get("/api/english/audios/\(id)")
    }

    /// 永久删除一条英文 audio (历史档案 sheet 二次确认后调用)。
    /// 后端级联删 segments 和本地 mp3 文件; records 中的练习时长保留。
    func deleteEnglishAudio(id: Int64) async throws {
        try await delete("/api/english/audios/\(id)")
    }

    func putEnglishSegments(audioID: Int64, segments: [EnglishSegment]) async throws {
        var req = URLRequest(url: try url("/api/english/audios/\(audioID)/segments"))
        req.httpMethod = "PUT"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(segments)
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "put english segments \(audioID) failed")
        }
    }

    func checkEnglishSegment(audioID: Int64, segIdx: Int, checked: Bool, seconds: Double) async throws {
        struct Body: Encodable {
            let audioId: Int64
            let segIdx: Int
            let checked: Bool
            let seconds: Double
            enum CodingKeys: String, CodingKey {
                case checked, seconds
                case audioId = "audio_id"
                case segIdx = "seg_idx"
            }
        }
        var req = URLRequest(url: try url("/api/english/segments/check"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(Body(audioId: audioID, segIdx: segIdx, checked: checked, seconds: seconds))
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "check english segment failed")
        }
    }

    func snoozeEnglishWindow(idx: Int, date: String?, count: Int = 1) async throws {
        struct Body: Encodable {
            let date: String?
            let count: Int
        }
        var req = URLRequest(url: try url("/api/english/windows/\(idx)/snooze"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(Body(date: date, count: count))
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "snooze english window \(idx) failed")
        }
    }

    func getEnglishSettings() async throws -> EnglishSettings {
        try await get("/api/english/settings")
    }

    func putEnglishSettings(_ es: EnglishSettings) async throws -> EnglishSettings {
        try await put("/api/english/settings", body: es)
    }

    /// 模态框抽奖 — 在 topic 内取候选 (排除 currentID + excludeIDs)。
    /// 不归档当前 audio (归档动作走 skipEnglishAudio).
    /// audio == nil 表示候选已抽完。
    func candidateEnglishAudio(
        topic: String,
        excludeIDs: [Int64],
        currentID: Int64?
    ) async throws -> EnglishCandidateResp {
        var items: [URLQueryItem] = []
        let topicTrim = topic.trimmingCharacters(in: .whitespaces)
        if !topicTrim.isEmpty {
            items.append(URLQueryItem(name: "topic", value: topicTrim))
        }
        if !excludeIDs.isEmpty {
            items.append(URLQueryItem(
                name: "exclude",
                value: excludeIDs.map(String.init).joined(separator: ",")
            ))
        }
        if let cur = currentID, cur > 0 {
            items.append(URLQueryItem(name: "current", value: String(cur)))
        }
        var path = "/api/english/audios/candidate"
        if !items.isEmpty {
            var comp = URLComponents()
            comp.queryItems = items
            if let q = comp.percentEncodedQuery {
                path += "?" + q
            }
        }
        return try await get(path)
    }

    /// 跳过当前 audio + 切到 selected (用户在模态框已选好)。
    /// selected_audio_id 必填; 后端返回 selected 的详情。
    func skipEnglishAudio(
        audioID: Int64,
        selectedAudioID: Int64,
        topic: String
    ) async throws -> EnglishSkipResp {
        var req = URLRequest(url: try url("/api/english/audios/\(audioID)/skip"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(EnglishSkipReq(
            topic: topic,
            selectedAudioID: selectedAudioID
        ))
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "skip english audio \(audioID) failed (status \((resp as? HTTPURLResponse)?.statusCode ?? -1))")
        }
        return try decoder.decode(EnglishSkipResp.self, from: data)
    }

    // MARK: - English Review (issue-44)

    func getEnglishReviewToday() async throws -> EnglishReviewToday {
        try await get("/api/english/review/today")
    }

    func reviewMarkDone(itemID: Int64) async throws {
        try await postNoBody("/api/english/review/\(itemID)/done")
    }

    /// 一键把今日派发的待复习项推进 stage (后端按 daily_limit - 今日已完成 封顶)。
    func reviewMarkAllDone() async throws {
        try await postNoBody("/api/english/review/done-all")
    }

    func reviewPostpone(itemID: Int64, toDate: String? = nil) async throws {
        struct Body: Encodable {
            let toDate: String?
            enum CodingKeys: String, CodingKey { case toDate = "to_date" }
        }
        var req = URLRequest(url: try url("/api/english/review/\(itemID)/postpone"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(Body(toDate: toDate))
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "postpone review \(itemID) failed")
        }
    }

    func reviewFinish(itemID: Int64) async throws {
        try await postNoBody("/api/english/review/\(itemID)/finish")
    }

    private func postNoBody(_ path: String) async throws {
        var req = URLRequest(url: try url(path))
        req.httpMethod = "POST"
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let code = (resp as? HTTPURLResponse)?.statusCode ?? -1
            throw APIError(message: "POST \(path) failed (status \(code))", status: code)
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
            throw APIError(message: "HTTP \(http.statusCode): \(body)", status: http.statusCode)
        }
        return try decoder.decode(T.self, from: data)
    }
}

/// 双栏使用独立进程和连接池，其他模块的后端关闭时仍可读取和保存。
actor DualNotesClient {
    static let shared = DualNotesClient()

    private static let baseURL = URL(string: "http://127.0.0.1:8090")!
    private let session: URLSession
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    init(session: URLSession = URLSession(configuration: .ephemeral)) {
        self.session = session
    }

    func getDualNotes(end: String, days: Int) async throws -> DualNotesPage {
        var components = URLComponents(
            url: Self.baseURL.appendingPathComponent("api/dual-notes"),
            resolvingAgainstBaseURL: true
        )!
        components.queryItems = [
            URLQueryItem(name: "end", value: end),
            URLQueryItem(name: "days", value: String(days))
        ]
        guard let url = components.url else {
            throw APIError(message: "无效的双栏日期范围。")
        }
        return try await send(URLRequest(url: url))
    }

    func putDualNotes(_ day: DualNotesDay) async throws -> DualNotesDay {
        struct Payload: Encodable {
            let revision: Int
            let rows: [DualNotesRow]
        }
        let url = Self.baseURL.appendingPathComponent("api/dual-notes")
            .appendingPathComponent(day.date)
        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(Payload(revision: day.revision, rows: day.rows))
        return try await send(request)
    }

    func getShortTermWorkspace(month: String) async throws -> ShortTermWorkspace {
        let request = URLRequest(url: try workspaceURL(month: month))
        let workspace: ShortTermWorkspace = try await send(request)
        guard workspace.month == month else {
            throw APIError(message: "短期工作区返回的月份不匹配。")
        }
        return workspace
    }

    func putShortTermWorkspace(_ workspace: ShortTermWorkspace) async throws -> ShortTermWorkspace {
        struct Payload: Encodable {
            let revision: Int
            let tables: [ShortTermWorkspaceTable]
        }
        var request = URLRequest(url: try workspaceURL(month: workspace.month))
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(Payload(revision: workspace.revision, tables: workspace.tables))
        let saved: ShortTermWorkspace = try await send(request)
        guard saved.month == workspace.month else {
            throw APIError(message: "短期工作区返回的月份不匹配。")
        }
        return saved
    }

    private func workspaceURL(month: String) throws -> URL {
        guard month.utf8.count == 7,
              month.range(of: "^[0-9]{4}-(0[1-9]|1[0-2])$", options: .regularExpression) != nil,
              !month.hasPrefix("0000") else {
            throw APIError(message: "月份必须为有效的 YYYY-MM。")
        }
        return Self.baseURL.appendingPathComponent("api/short-term-workspaces")
            .appendingPathComponent(month)
    }

    private func send<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError(message: "双栏服务返回了无效响应。")
        }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8) ?? ""
            throw APIError(message: "HTTP \(http.statusCode): \(body)", status: http.statusCode)
        }
        return try decoder.decode(T.self, from: data)
    }
}
