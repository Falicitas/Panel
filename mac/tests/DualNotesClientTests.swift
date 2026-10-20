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
        print("Dual-notes service isolation, persistence payload, and conflict checks passed")
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
