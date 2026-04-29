import SwiftUI
import WebKit

struct MilkDownEditor: NSViewRepresentable {
    @Binding var text: String

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        let controller = WKUserContentController()
        controller.add(context.coordinator, name: "markdown")
        config.userContentController = controller
        config.defaultWebpagePreferences.allowsContentJavaScript = true

        let webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = context.coordinator
        webView.setValue(false, forKey: "drawsBackground")
        #if DEBUG
        if #available(macOS 13.3, *) { webView.isInspectable = true }
        #endif

        if let url = Bundle.main.url(
            forResource: "index",
            withExtension: "html",
            subdirectory: "editor"
        ) {
            webView.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        } else {
            assertionFailure("editor/index.html missing from bundle resources")
        }

        context.coordinator.webView = webView
        return webView
    }

    func updateNSView(_ webView: WKWebView, context: Context) {
        let coord = context.coordinator
        coord.parent = self
        guard coord.isReady else { return }
        if coord.lastSyncedMarkdown != text {
            coord.lastSyncedMarkdown = text
            coord.evalSetRaw(text)
        }
    }

    @MainActor
    final class Coordinator: NSObject, WKScriptMessageHandler, WKNavigationDelegate {
        var parent: MilkDownEditor
        weak var webView: WKWebView?
        var isReady = false
        var lastSyncedMarkdown: String?

        init(_ parent: MilkDownEditor) {
            self.parent = parent
        }

        nonisolated func webView(
            _ webView: WKWebView,
            didFinish navigation: WKNavigation!
        ) {
            MainActor.assumeIsolated { self.handleDidFinish() }
        }

        private func handleDidFinish() {
            guard let webView else { return }
            let initial = parent.text
            lastSyncedMarkdown = initial
            let escaped = Self.jsEscape(initial)
            webView.evaluateJavaScript("window.panelEditor.init(\(escaped))") { [weak self] _, _ in
                self?.isReady = true
            }
        }

        func evalSetRaw(_ md: String) {
            guard let webView else { return }
            let escaped = Self.jsEscape(md)
            webView.evaluateJavaScript("window.panelEditor.setRaw(\(escaped))", completionHandler: nil)
        }

        nonisolated func userContentController(
            _ userContentController: WKUserContentController,
            didReceive message: WKScriptMessage
        ) {
            MainActor.assumeIsolated { self.handleScriptMessage(message.body) }
        }

        private func handleScriptMessage(_ payload: Any) {
            guard let dict = payload as? [String: Any],
                  let type = dict["type"] as? String else { return }
            switch type {
            case "change":
                if let md = dict["md"] as? String, md != lastSyncedMarkdown {
                    lastSyncedMarkdown = md
                    parent.text = md
                }
            case "ready", "loaded":
                break
            default:
                break
            }
        }

        private static func jsEscape(_ s: String) -> String {
            guard let data = try? JSONEncoder().encode(s),
                  let str = String(data: data, encoding: .utf8) else { return "\"\"" }
            return str
        }
    }
}
