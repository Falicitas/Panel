import AppKit
import WebKit

/// Exercise the production page without a backend or persistent browser storage.
final class Runner: NSObject, WKScriptMessageHandler {
    private var view: WKWebView!
    private var window: NSWindow!

    func start() throws {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        for name in ["result", "dualNotes", "layout"] {
            config.userContentController.add(self, name: name)
        }
        let script = try String(contentsOfFile: CommandLine.arguments[2], encoding: .utf8)
        config.userContentController.addUserScript(WKUserScript(
            source: script, injectionTime: .atDocumentEnd, forMainFrameOnly: true
        ))
        view = WKWebView(frame: NSRect(x: 0, y: 0, width: 900, height: 700), configuration: config)
        view.setValue(false, forKey: "drawsBackground")
        window = NSWindow(contentRect: view.frame, styleMask: [.titled, .resizable], backing: .buffered, defer: false)
        window.contentView = view
        let url = URL(fileURLWithPath: CommandLine.arguments[1])
        view.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        DispatchQueue.main.asyncAfter(deadline: .now() + 45) {
            self.finish("FAIL: layout test timed out")
        }
    }

    private func finish(_ result: String) {
        print(result)
        exit(result.hasPrefix("PASS:") ? 0 : 1)
    }

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        if message.name == "result" {
            finish(String(describing: message.body))
            return
        }
        guard let request = message.body as? [String: Any] else { return }
        if message.name == "layout",
           let width = request["width"] as? Double,
           let zoom = request["zoom"] as? Double {
            window.setContentSize(NSSize(width: width, height: 700))
            // Assigning even an unchanged pageZoom forces a fresh WebKit layout and
            // masks the stale grid-row heights this test must catch.
            if view.pageZoom != CGFloat(zoom) { view.pageZoom = CGFloat(zoom) }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
                self.view.evaluateJavaScript("window.layoutDone()")
            }
            return
        }
        guard message.name == "dualNotes", let id = request["requestId"] as? String else { return }
        guard request["method"] as? String == "load", let date = request["end"] as? String else {
            finish("FAIL: resizing unexpectedly requested a note mutation")
            return
        }
        let paragraph = "测试长段落，调整窗口宽度后需要重新排列和计算行高。文字不应该与其他段落重叠，也不应该溢出下一行。Mixed English words wrap to fit the available space. "
        let markdown = [
            String(repeating: paragraph, count: 3),
            "$$\nO_{n+1} = \\operatorname{softmax}(q K^T / \\sqrt{d})V\n$$",
            String(repeating: paragraph, count: 2),
            "```shell\nblock 0 1 2 3 4 5 6 7 8 9\n\(paragraph)\n```",
            String(repeating: paragraph, count: 4),
        ].joined(separator: "\n\n")
        let rows: [[String: Any]] = (0..<4).map {
            ["id": "row-\($0)", "left": markdown, "right": $0 % 2 == 0 ? "" : paragraph]
        }
        let response: [String: Any] = ["requestId": id, "ok": true, "data": ["days": [
            ["date": date, "revision": 1, "rows": rows]
        ]]]
        do {
            let json = String(data: try JSONSerialization.data(withJSONObject: response), encoding: .utf8)!
            view.evaluateJavaScript("window.panelDualNotes.receive(\(json))")
        } catch {
            finish("FAIL: fixture encoding failed: \(error)")
        }
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.accessory)
let runner = Runner()
do { try runner.start() }
catch { print("FAIL: \(error)"); exit(1) }
application.run()
