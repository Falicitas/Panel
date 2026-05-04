import SwiftUI
import WebKit

/// cmd+F 搜索请求 — id 每次变化都触发新一次 webView.find, 即使 query 相同 (用户连按下一个)。
struct EditorFindCommand: Equatable {
    let id: UUID
    let query: String
    let backwards: Bool
}

struct MilkDownEditor: NSViewRepresentable {
    @Binding var text: String
    var pageZoom: Double = 1.0
    @Binding var scrollText: String?
    var readOnly: Bool = false
    @Binding var findCommand: EditorFindCommand?

    init(
        text: Binding<String>,
        pageZoom: Double = 1.0,
        scrollText: Binding<String?> = .constant(nil),
        readOnly: Bool = false,
        findCommand: Binding<EditorFindCommand?> = .constant(nil)
    ) {
        self._text = text
        self.pageZoom = pageZoom
        self._scrollText = scrollText
        self.readOnly = readOnly
        self._findCommand = findCommand
    }

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
        let zoom = CGFloat(pageZoom)
        if webView.pageZoom != zoom { webView.pageZoom = zoom }
        guard coord.isReady else { return }
        if coord.lastSyncedMarkdown != text {
            coord.lastSyncedMarkdown = text
            coord.evalSetRaw(text)
        }
        if coord.lastReadOnly != readOnly {
            coord.lastReadOnly = readOnly
            coord.evalSetReadonly(readOnly)
        }
        if let text = scrollText, !text.isEmpty {
            coord.evalScrollToText(text)
            DispatchQueue.main.async { self.scrollText = nil }
        }
        if let cmd = findCommand, coord.lastFindCommandID != cmd.id {
            coord.lastFindCommandID = cmd.id
            coord.runFind(query: cmd.query, backwards: cmd.backwards)
        }
    }

    @MainActor
    final class Coordinator: NSObject, WKScriptMessageHandler, WKNavigationDelegate {
        var parent: MilkDownEditor
        weak var webView: WKWebView?
        var isReady = false
        var lastSyncedMarkdown: String?
        var lastReadOnly: Bool = false
        var lastFindCommandID: UUID?

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
            lastReadOnly = parent.readOnly
            let escaped = Self.jsEscape(initial)
            let readonlyArg = parent.readOnly ? "true" : "false"
            webView.evaluateJavaScript("window.panelEditor.init(\(escaped), \(readonlyArg))") { [weak self] _, _ in
                self?.isReady = true
            }
        }

        func evalSetReadonly(_ readonly: Bool) {
            guard let webView else { return }
            let arg = readonly ? "true" : "false"
            webView.evaluateJavaScript("window.panelEditor.setReadonly(\(arg))", completionHandler: nil)
        }

        func evalSetRaw(_ md: String) {
            guard let webView else { return }
            let escaped = Self.jsEscape(md)
            webView.evaluateJavaScript("window.panelEditor.setRaw(\(escaped))", completionHandler: nil)
        }

        /// 用 WKWebView 内置 find API (macOS 12+) 找文本, 找到自动滚到 + 高亮。
        /// query 空 → 清高亮 (clearFindResults)。
        func runFind(query: String, backwards: Bool) {
            guard let webView else { return }
            let trimmed = query.trimmingCharacters(in: .whitespaces)
            if trimmed.isEmpty {
                if #available(macOS 12.0, *) {
                    Task { try? await webView.find("", configuration: WKFindConfiguration()) }
                }
                return
            }
            if #available(macOS 12.0, *) {
                let cfg = WKFindConfiguration()
                cfg.backwards = backwards
                cfg.caseSensitive = false
                cfg.wraps = true
                webView.find(trimmed, configuration: cfg) { _ in }
            }
        }

        func evalScrollToText(_ text: String) {
            guard let webView else { return }
            let escaped = Self.jsEscape(text)
            // 等内容渲染完(setRaw 是 destroy+recreate)再 scroll;短延迟保险
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) { [weak webView] in
                webView?.evaluateJavaScript("window.panelEditor.scrollToText(\(escaped))", completionHandler: nil)
            }
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
