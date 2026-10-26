import AppKit
import WebKit

/// Real WebKit editing against an in-memory bridge. Never opens Panel's database
/// or persistent web storage, and never activates or closes the user's app.
final class WorkspaceRunner: NSObject, WKScriptMessageHandler {
    private var view: WKWebView!
    private var window: NSWindow!
    private var months: [String: [String: Any]] = [:]
    private var days: [String: [String: Any]] = [:]
    private var workspaceSaves = 0
    private var copyRequests: [[String: Any]] = []
    private var copyResults: [String: [String: Any]] = [:]
    private var copyCommands: [String: [String: Any]] = [:]
    private var copySources: [String: [String: Any]] = [:]
    private var copyTargetsBefore: [String: [String: Any]] = [:]
    private var dropNextCopyResponse = false
    private var failNextCopy = false
    private var blockedSaveMonths = Set<String>()

    func start() throws {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        for name in ["result", "dualNotes", "layout", "fixture"] {
            config.userContentController.add(self, name: name)
        }
        let script = try String(contentsOfFile: CommandLine.arguments[2], encoding: .utf8)
        config.userContentController.addUserScript(WKUserScript(
            source: script, injectionTime: .atDocumentEnd, forMainFrameOnly: true
        ))
        view = WKWebView(frame: NSRect(x: 0, y: 0, width: 1100, height: 800), configuration: config)
        window = NSWindow(contentRect: view.frame, styleMask: [.titled, .resizable], backing: .buffered, defer: false)
        window.contentView = view
        let url = URL(fileURLWithPath: CommandLine.arguments[1])
        view.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        DispatchQueue.main.asyncAfter(deadline: .now() + 60) {
            self.view.evaluateJavaScript("JSON.stringify({stage: window.workspaceTestStage, error: document.getElementById('workspace-migrate-error')?.textContent, open: document.getElementById('workspace-migrate-dialog')?.open})") { value, _ in
                self.finish("FAIL: workspace test timed out; copies=\(self.copyRequests.count)/\(self.copyResults.count); \(String(describing: value))")
            }
        }
    }

    private func finish(_ result: String) {
        print(result)
        exit(result.hasPrefix("PASS:") ? 0 : 1)
    }

    private func emit(_ function: String, _ value: Any) {
        do {
            let data = try JSONSerialization.data(withJSONObject: value, options: [.fragmentsAllowed])
            let json = String(data: data, encoding: .utf8)!
            view.evaluateJavaScript("\(function)(\(json))")
        } catch { finish("FAIL: fixture serialization: \(error)") }
    }

    private func reply(_ id: String, data: Any) {
        emit("window.panelDualNotes.receive", ["requestId": id, "ok": true, "data": data])
    }

    private func fail(_ id: String, status: Int, message: String) {
        emit("window.panelDualNotes.receive", ["requestId": id, "ok": false, "status": status, "error": message])
    }

    private func copyTables(_ id: String, command: [String: Any]) {
        copyRequests.append(command)
        guard let operation = command["operationId"] as? String,
              let sourceMonth = command["sourceMonth"] as? String,
              let targetMonth = command["targetMonth"] as? String,
              let sourceRevision = command["sourceRevision"] as? Int,
              let targetRevision = command["targetRevision"] as? Int,
              let selected = command["tableIds"] as? [String], !selected.isEmpty else {
            finish("FAIL: malformed copy request"); return
        }
        if let result = copyResults[operation] {
            guard NSDictionary(dictionary: copyCommands[operation]!).isEqual(to: command) else {
                fail(id, status: 409, message: "operation ID was reused with different inputs"); return
            }
            reply(id, data: result)
            return
        }
        if failNextCopy {
            failNextCopy = false
            fail(id, status: 503, message: "fixture copy temporarily unavailable")
            return
        }
        guard sourceMonth != targetMonth,
              let source = months[sourceMonth],
              let sourceTables = source["tables"] as? [[String: Any]] else {
            fail(id, status: 400, message: "invalid source month"); return
        }
        var target = months[targetMonth] ?? ["month": targetMonth, "revision": 0, "tables": [[String: Any]]()]
        guard sourceRevision == source["revision"] as? Int,
              targetRevision == target["revision"] as? Int else {
            fail(id, status: 409, message: "fixture copy revision conflict"); return
        }
        let selectedSet = Set(selected)
        let matching = sourceTables.filter { selectedSet.contains($0["id"] as? String ?? "") }
        guard matching.count == selected.count, selectedSet.count == selected.count else {
            fail(id, status: 400, message: "invalid selected tables"); return
        }
        copyCommands[operation] = command
        copySources[operation] = source
        copyTargetsBefore[operation] = target
        let copied: [[String: Any]] = matching.map { table in
            var next = table
            next["id"] = UUID().uuidString
            next["rows"] = (table["rows"] as! [[String: Any]]).map { row in
                var nextRow = row
                nextRow["id"] = UUID().uuidString
                return nextRow
            }
            return next
        }
        target["tables"] = (target["tables"] as! [[String: Any]]) + copied
        target["revision"] = targetRevision + 1
        months[targetMonth] = target
        let result: [String: Any] = ["operationId": operation, "target": target, "copiedTableIds": copied.map { $0["id"] as! String }]
        copyResults[operation] = result
        if dropNextCopyResponse {
            dropNextCopyResponse = false
            // The write succeeded, but its successful response was lost. The
            // native transport reports an unknown outcome without HTTP status,
            // matching the production timeout path without hidden-page timers.
            emit("window.panelDualNotes.receive", ["requestId": id, "ok": false, "error": "fixture connection interrupted after commit"])
            return
        }
        reply(id, data: result)
    }

    private func dayRange(end: String, count: Int) -> [[String: Any]] {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        formatter.dateFormat = "yyyy-MM-dd"
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = formatter.timeZone
        guard let date = formatter.date(from: end) else { return [] }
        return (0..<count).reversed().map { offset in
            let key = formatter.string(from: calendar.date(byAdding: .day, value: -offset, to: date)!)
            return days[key] ?? ["date": key, "revision": 1, "rows": [
                ["id": "daily-\(key)", "left": "独立的每日记录", "right": ""]
            ]]
        }
    }

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        if message.name == "result" { finish(String(describing: message.body)); return }
        guard let request = message.body as? [String: Any] else { return }
        if message.name == "layout", let width = request["width"] as? Double, let zoom = request["zoom"] as? Double {
            window.setContentSize(NSSize(width: width, height: 800))
            if view.pageZoom != CGFloat(zoom) { view.pageZoom = CGFloat(zoom) }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                self.view.evaluateJavaScript("window.layoutDone()")
            }
            return
        }
        if message.name == "fixture", let id = request["id"] as? String {
            switch request["action"] as? String {
            case "dropNextCopyResponse": dropNextCopyResponse = true
            case "failNextCopy": failNextCopy = true
            case "blockWorkspaceSaves":
                if let month = request["month"] as? String { blockedSaveMonths.insert(month) }
            case "unblockWorkspaceSaves":
                if let month = request["month"] as? String { blockedSaveMonths.remove(month) }
            case nil: break
            default: finish("FAIL: unknown fixture action"); return
            }
            let result: [String: Any] = [
                "id": id, "months": months, "workspaceSaves": workspaceSaves, "days": days,
                "copyRequests": copyRequests, "copyCommits": copyResults.count,
                "copySources": copySources, "copyTargetsBefore": copyTargetsBefore,
                "copyResults": copyResults
            ]
            if let name = request["snapshot"] as? String,
               ["desktop", "narrow", "order-dialog", "migration"].contains(name),
               let directory = ProcessInfo.processInfo.environment["PANEL_WORKSPACE_SCREENSHOTS"] {
                view.takeSnapshot(with: nil) { image, error in
                    guard let image, let tiff = image.tiffRepresentation,
                          let bitmap = NSBitmapImageRep(data: tiff),
                          let png = bitmap.representation(using: .png, properties: [:]) else {
                        self.finish("FAIL: snapshot failed: \(String(describing: error))"); return
                    }
                    do {
                        let folder = URL(fileURLWithPath: directory, isDirectory: true)
                        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
                        try png.write(to: folder.appendingPathComponent("\(name).png"))
                        self.emit("window.workspaceFixtureReply", result)
                    } catch { self.finish("FAIL: snapshot writing failed: \(error)") }
                }
            } else { emit("window.workspaceFixtureReply", result) }
            return
        }
        guard message.name == "dualNotes", let id = request["requestId"] as? String,
              let method = request["method"] as? String else { return }
        switch method {
        case "load":
            guard let end = request["end"] as? String, let count = request["days"] as? Int else {
                finish("FAIL: malformed day load"); return
            }
            reply(id, data: ["days": dayRange(end: end, count: count)])
        case "save":
            guard var day = request["day"] as? [String: Any], let date = day["date"] as? String else {
                finish("FAIL: malformed day save"); return
            }
            day["revision"] = (day["revision"] as? Int ?? 0) + 1
            days[date] = day
            reply(id, data: day)
        case "loadWorkspace":
            guard let month = request["month"] as? String else { finish("FAIL: missing month"); return }
            reply(id, data: months[month] ?? ["month": month, "revision": 0, "tables": []])
        case "saveWorkspace":
            guard var workspace = request["workspace"] as? [String: Any],
                  let month = workspace["month"] as? String,
                  let revision = workspace["revision"] as? Int,
                  workspace["tables"] is [[String: Any]] else {
                finish("FAIL: malformed workspace save"); return
            }
            if blockedSaveMonths.contains(month) {
                fail(id, status: 503, message: "fixture workspace save temporarily unavailable")
                return
            }
            guard revision == (months[month]?["revision"] as? Int ?? 0) else {
                emit("window.panelDualNotes.receive", ["requestId": id, "ok": false, "status": 409, "error": "fixture revision conflict"])
                return
            }
            workspace["revision"] = revision + 1
            months[month] = workspace
            workspaceSaves += 1
            reply(id, data: workspace)
        case "copyWorkspaceTables":
            guard let command = request["copy"] as? [String: Any] else {
                finish("FAIL: missing copy command"); return
            }
            copyTables(id, command: command)
        default:
            finish("FAIL: unexpected bridge method \(method)")
        }
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.accessory)
let runner = WorkspaceRunner()
do { try runner.start() }
catch { print("FAIL: \(error)"); exit(1) }
application.run()
