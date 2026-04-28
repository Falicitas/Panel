import Foundation

struct APIError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}

private struct RecurringPayload: Encodable {
    let content: String
    let sortOrder: Int

    enum CodingKeys: String, CodingKey {
        case content
        case sortOrder = "sort_order"
    }
}

actor APIClient {
    static let shared = APIClient()

    private let baseURL = URL(string: "http://127.0.0.1:8089")!
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

    func listRecurring() async throws -> [RecurringTodo] {
        try await get("/api/recurring")
    }

    func createRecurring(content: String, sortOrder: Int) async throws -> RecurringTodo {
        try await post("/api/recurring", body: RecurringPayload(content: content, sortOrder: sortOrder))
    }

    func updateRecurring(id: Int64, content: String, sortOrder: Int) async throws -> [RecurringTodo] {
        try await put("/api/recurring/\(id)", body: RecurringPayload(content: content, sortOrder: sortOrder))
    }

    func deleteRecurring(id: Int64) async throws {
        var req = URLRequest(url: url("/api/recurring/\(id)"))
        req.httpMethod = "DELETE"
        let (_, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError(message: "DELETE /api/recurring/\(id) failed")
        }
    }

    private func encode(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? s
    }

    private func url(_ path: String) -> URL {
        URL(string: path, relativeTo: baseURL)!
    }

    private func get<T: Decodable>(_ path: String) async throws -> T {
        var req = URLRequest(url: url(path))
        req.httpMethod = "GET"
        return try await send(req)
    }

    private func put<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        var req = URLRequest(url: url(path))
        req.httpMethod = "PUT"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(body)
        return try await send(req)
    }

    private func post<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        var req = URLRequest(url: url(path))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try encoder.encode(body)
        return try await send(req)
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
