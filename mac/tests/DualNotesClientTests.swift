import Foundation

@main
struct DualNotesClientTests {
    static func main() async throws {
        precondition(URLProtocol.registerClass(BackendStub.self))
        defer { URLProtocol.unregisterClass(BackendStub.self) }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [BackendStub.self]
        let session = URLSession(configuration: configuration)
        defer { session.invalidateAndCancel() }
        let dualNotes = DualNotesClient(session: session)
        let day = DualNotesDay(
            date: "2026-10-21", revision: 7,
            rows: [DualNotesRow(id: "row-1", left: "# 中文\n内容", right: "**right**")]
        )

        // The general backend fails, while dual-notes requests still complete.
        BackendStub.respond { request in
            expect(request.url?.host == "127.0.0.1", "Panel remains local")
            expect(request.url?.port == 8089, "Other modules keep their existing port")
            expect(request.url?.path == "/api/todolists", "Exercise a general backend request")
            throw URLError(.cannotConnectToHost)
        }
        do {
            _ = try await APIClient.shared.listTodolists()
            preconditionFailure("General backend should be unavailable")
        } catch {
            expect((error as NSError).code == URLError.cannotConnectToHost.rawValue,
                   "The simulated backend failure must reach its caller")
        }

        BackendStub.respond { request in
            expect(request.httpMethod == "GET", "Load uses GET")
            expect(request.url?.host == "127.0.0.1", "Dual notes remains local")
            expect(request.url?.port == 8090, "Load uses the independent service")
            expect(request.url?.path == "/api/dual-notes", "Keep the existing load endpoint")
            let query = URLComponents(url: request.url!, resolvingAgainstBaseURL: true)!.queryItems!
            expect(query.contains(URLQueryItem(name: "end", value: day.date)), "Preserve the end date")
            expect(query.contains(URLQueryItem(name: "days", value: "10")), "Preserve the requested range")
            return (200, try JSONEncoder().encode(DualNotesPage(days: [day])))
        }
        let loaded = try await dualNotes.getDualNotes(end: day.date, days: 10)
        expect(loaded.days.count == 1 && loaded.days[0].revision == 7, "Decode the loaded revision")
        expect(loaded.days[0].rows[0].left == day.rows[0].left, "Preserve Markdown and Unicode")

        BackendStub.respond { request in
            expect(request.httpMethod == "PUT", "Save uses PUT")
            expect(request.url?.port == 8090, "Save uses the independent service")
            expect(request.url?.path == "/api/dual-notes/2026-10-21", "Save selects the requested day")
            expect(request.value(forHTTPHeaderField: "Content-Type") == "application/json", "Save sends JSON")
            let payload = try JSONSerialization.jsonObject(with: body(of: request)) as! [String: Any]
            expect(payload["revision"] as? Int == 7, "Send the loaded revision for optimistic locking")
            let rows = payload["rows"] as! [[String: String]]
            expect(rows == [["id": "row-1", "left": "# 中文\n内容", "right": "**right**"]],
                   "Save must preserve row identities and both Markdown cells")
            let saved = DualNotesDay(date: day.date, revision: 8, rows: day.rows)
            return (200, try JSONEncoder().encode(saved))
        }
        let saved = try await dualNotes.putDualNotes(day)
        expect(saved.revision == 8, "Return the service's new revision")

        BackendStub.respond { _ in
            (409, Data("记录已更新，请重新加载。".utf8))
        }
        do {
            _ = try await dualNotes.putDualNotes(day)
            preconditionFailure("Stale writes must fail")
        } catch let error as APIError {
            expect(error.status == 409, "Keep the HTTP status required by draft conflict recovery")
            expect(error.localizedDescription.contains("记录已更新"), "Preserve the service error")
        }

        BackendStub.respond { request in
            expect(request.url?.port == 8090, "Do not fall back to panel-backend")
            throw URLError(.cannotConnectToHost)
        }
        do {
            _ = try await dualNotes.getDualNotes(end: day.date, days: 1)
            preconditionFailure("A stopped dual-notes service must surface a failure")
        } catch {
            expect((error as NSError).code == URLError.cannotConnectToHost.rawValue,
                   "Propagate connection failures for the editor's retry and draft recovery")
        }
        try await testWorkspaces(using: dualNotes)
        print("Dual-notes and monthly workspace client checks passed")
    }

    private static func testWorkspaces(using client: DualNotesClient) async throws {
        let workspace = ShortTermWorkspace(
            month: "2026-10", revision: 4,
            tables: [
                ShortTermWorkspaceTable(id: "table-a", header: "十月事项", rows: [
                    ShortTermWorkspaceRow(id: "row-a", content: "# 中文\n**内容**"),
                    ShortTermWorkspaceRow(id: "row-b", content: "")
                ]),
                ShortTermWorkspaceTable(id: "table-b", header: "随手记录", rows: [
                    ShortTermWorkspaceRow(id: "row-c", content: "```swift\nlet value = 1\n```")
                ])
            ]
        )
        BackendStub.respond { request in
            expect(request.httpMethod == "GET", "Workspace load uses GET")
            expect(request.url?.host == "127.0.0.1" && request.url?.port == 8090,
                   "Workspaces use the independent local service")
            expect(request.url?.path == "/api/short-term-workspaces/2026-10",
                   "Workspace load selects the month endpoint")
            expect(request.url?.query == nil, "Workspace month belongs in the path")
            return (200, Data(##"{"month":"2026-10","revision":4,"tables":[{"id":"table-a","header":"十月事项","rows":[{"id":"row-a","content":"# 中文\n**内容**"},{"id":"row-b","content":""}]},{"id":"table-b","header":"随手记录","rows":[{"id":"row-c","content":"```swift\nlet value = 1\n```"}]}]}"##.utf8))
        }
        let loaded = try await client.getShortTermWorkspace(month: workspace.month)
        expect(loaded.month == workspace.month && loaded.revision == 4, "Decode the requested month and revision")
        expect(loaded.tables.map(\.id) == ["table-a", "table-b"], "Preserve multiple tables and their order")
        expect(loaded.tables[0].header == "十月事项", "Decode editable column headers")
        expect(loaded.tables[0].rows[0].content == "# 中文\n**内容**", "Decode Unicode Markdown without changes")
        expect(loaded.tables[0].rows[1].content.isEmpty, "Preserve intentionally empty rows")

        BackendStub.respond { request in
            expect(request.httpMethod == "PUT", "Workspace save uses PUT")
            expect(request.url?.host == "127.0.0.1" && request.url?.port == 8090,
                   "Workspace save stays on the independent service")
            expect(request.url?.path == "/api/short-term-workspaces/2026-10", "Workspace save selects the month")
            expect(request.value(forHTTPHeaderField: "Content-Type") == "application/json", "Workspace save sends JSON")
            let payload = try JSONSerialization.jsonObject(with: body(of: request)) as! [String: Any]
            expect(Set(payload.keys) == ["revision", "tables"], "Workspace PUT only sends revision and tables")
            expect(payload["revision"] as? Int == 4, "Send the loaded month revision")
            let tables = payload["tables"] as! [[String: Any]]
            expect(tables.count == 2 && tables.map { $0["id"] as! String } == ["table-a", "table-b"],
                   "Preserve table identities and order when saving")
            expect(tables[0]["header"] as? String == "十月事项" && tables[1]["header"] as? String == "随手记录",
                   "Persist each editable column title")
            let rows = tables[0]["rows"] as! [[String: String]]
            expect(rows == [["id": "row-a", "content": "# 中文\n**内容**"], ["id": "row-b", "content": ""]],
                   "Persist ordered rows, Markdown, and empty cells")
            let saved = ShortTermWorkspace(month: workspace.month, revision: 5, tables: workspace.tables)
            return (200, try JSONEncoder().encode(saved))
        }
        let saved = try await client.putShortTermWorkspace(loaded)
        expect(saved.revision == 5 && saved.tables.count == 2, "Return the new revision and all tables")

        // Empty months are valid, including the supported year boundaries.
        for month in ["0001-01", "2024-02", "9999-12"] {
            BackendStub.respond { request in
                expect(request.url?.path == "/api/short-term-workspaces/\(month)", "Keep a valid month verbatim")
                return (200, try JSONEncoder().encode(ShortTermWorkspace(month: month, revision: 0, tables: [])))
            }
            let empty = try await client.getShortTermWorkspace(month: month)
            expect(empty.tables.isEmpty && empty.revision == 0, "Empty months do not fabricate a table")
        }

        BackendStub.respond { _ in
            preconditionFailure("Invalid months must be rejected before any HTTP request")
        }
        for month in ["", "2026-1", "026-01", "0000-01", "10000-01", "2026-00", "2026-13",
                      "2026-01-01", " 2026-01", "2026-01\n", "２０２６-０１", "2026/01", "../2026", "2026-01?x=1"] {
            do {
                _ = try await client.getShortTermWorkspace(month: month)
                preconditionFailure("Invalid GET month accepted: \(month)")
            } catch let error as APIError {
                expect(error.localizedDescription.contains("YYYY-MM"), "Invalid GET month has a useful error")
            }
            do {
                _ = try await client.putShortTermWorkspace(ShortTermWorkspace(month: month, revision: 0, tables: []))
                preconditionFailure("Invalid PUT month accepted: \(month)")
            } catch let error as APIError {
                expect(error.localizedDescription.contains("YYYY-MM"), "Invalid PUT month has a useful error")
            }
        }

        BackendStub.respond { _ in (409, Data("本月工作区已更新。".utf8)) }
        do {
            _ = try await client.putShortTermWorkspace(workspace)
            preconditionFailure("A stale workspace revision must fail")
        } catch let error as APIError {
            expect(error.status == 409, "Workspace conflicts retain their status for the JavaScript bridge")
            expect(error.localizedDescription.contains("本月工作区已更新"), "Keep workspace conflict details")
        }

        BackendStub.respond { _ in
            (200, try JSONEncoder().encode(ShortTermWorkspace(month: "2026-11", revision: 1, tables: [])))
        }
        do {
            _ = try await client.getShortTermWorkspace(month: workspace.month)
            preconditionFailure("A response for a different month must not replace the requested month")
        } catch let error as APIError {
            expect(error.localizedDescription.contains("月份不匹配"), "Reject mismatched loaded months")
        }
        do {
            _ = try await client.putShortTermWorkspace(workspace)
            preconditionFailure("A response for a different month must not acknowledge a save")
        } catch let error as APIError {
            expect(error.localizedDescription.contains("月份不匹配"), "Reject mismatched saved months")
        }

        BackendStub.respond { request in
            expect(request.url?.port == 8090, "Workspaces never fall back to panel-backend")
            throw URLError(.cannotConnectToHost)
        }
        do {
            _ = try await client.getShortTermWorkspace(month: workspace.month)
            preconditionFailure("An unavailable workspace service must surface a failure")
        } catch {
            expect((error as NSError).code == URLError.cannotConnectToHost.rawValue,
                   "Keep workspace connection failures available to draft recovery")
        }
    }

    private static func body(of request: URLRequest) throws -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var result = Data()
        var bytes = [UInt8](repeating: 0, count: 1024)
        while stream.hasBytesAvailable {
            let count = stream.read(&bytes, maxLength: bytes.count)
            if count < 0 { throw stream.streamError! }
            if count == 0 { break }
            result.append(contentsOf: bytes.prefix(count))
        }
        return result
    }

    private static func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
        precondition(condition(), message)
    }
}

private final class BackendStub: URLProtocol {
    private static let lock = NSLock()
    private static var handler: ((URLRequest) throws -> (Int, Data))?

    static func respond(_ handler: @escaping (URLRequest) throws -> (Int, Data)) {
        lock.lock()
        defer { lock.unlock() }
        Self.handler = handler
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.lock.lock()
        let handler = Self.handler!
        Self.lock.unlock()
        do {
            let (status, data) = try handler(request)
            let response = HTTPURLResponse(
                url: request.url!, statusCode: status, httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
