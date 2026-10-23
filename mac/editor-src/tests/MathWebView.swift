import AppKit
import WebKit

final class Runner: NSObject, WKScriptMessageHandler {
    var view: WKWebView!
    func start() {
        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "result")
        view = WKWebView(frame: NSRect(x: 0, y: 0, width: 900, height: 700), configuration: config)
        let url = URL(fileURLWithPath: CommandLine.arguments[1])
        view.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        DispatchQueue.main.asyncAfter(deadline: .now() + 30) {
            print("FAIL: WebKit test timed out")
            exit(1)
        }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        let result = String(describing: message.body)
        print(result)
        exit(result.hasPrefix("PASS:") ? 0 : 1)
    }
}
let application = NSApplication.shared
application.setActivationPolicy(.accessory)
let runner = Runner()
runner.start()
application.run()
