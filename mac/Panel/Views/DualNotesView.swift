import SwiftUI
import WebKit

/// AppState owns the controller, so switching sidebar tabs preserves editor state
/// and in-flight saves. Each Markdown cell is managed by the bundled web editor.
struct DualNotesView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        DualNotesPane(controller: state.dualNotes, pageZoom: state.editorPageZoom)
    }
}

private struct DualNotesPane: View {
    @ObservedObject var controller: DualNotesController
    let pageZoom: Double

    var body: some View {
        ZStack {
            DualNotesWebView(controller: controller, pageZoom: pageZoom)
            if let error = controller.loadError {
                ContentUnavailableView {
                    Label("双栏页面加载失败", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(error)
                } actions: {
                    Button("重新加载") { controller.reload() }
                }
                .background(.background)
            }
        }
        .background {
            Button("查找双栏内容") { controller.openFind() }
                .keyboardShortcut("f", modifiers: .command)
                .frame(width: 0, height: 0)
                .hidden()
        }
        .onDisappear { controller.flush() }
        .onReceive(NotificationCenter.default.publisher(
            for: NSApplication.willResignActiveNotification
        )) { _ in controller.flush() }
    }
}

private struct DualNotesWebView: NSViewRepresentable {
    let controller: DualNotesController
    let pageZoom: Double

    func makeNSView(context: Context) -> WKWebView {
        controller.editorWebView()
    }

    func updateNSView(_ webView: WKWebView, context: Context) {
        if webView.pageZoom != CGFloat(pageZoom) {
            webView.pageZoom = CGFloat(pageZoom)
        }
    }
}

@MainActor
final class DualNotesController: NSObject, ObservableObject, WKScriptMessageHandler,
    WKNavigationDelegate {
    @Published private(set) var loadError: String?
    private var retainedWebView: WKWebView?
    private var isReady = false
    private var pendingFind = false

    func editorWebView() -> WKWebView {
        if let retainedWebView { return retainedWebView }
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.add(
            WeakDualNotesHandler(target: self), name: "dualNotes"
        )
        configuration.defaultWebpagePreferences.allowsContentJavaScript = true
        let webView = WKWebView(frame: .zero, configuration: configuration)
        webView.navigationDelegate = self
        webView.setValue(false, forKey: "drawsBackground")
        #if DEBUG
        if #available(macOS 13.3, *) { webView.isInspectable = true }
        #endif
        retainedWebView = webView
        loadPage(webView)
        return webView
    }

    private func loadPage(_ webView: WKWebView) {
        guard let url = Bundle.main.url(
            forResource: "index", withExtension: "html", subdirectory: "dual-notes"
        ) else {
            loadError = "缺少双栏编辑器资源，请重新构建应用。"
            return
        }
        loadError = nil
        isReady = false
        webView.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
    }

    func reload() {
        if let retainedWebView { loadPage(retainedWebView) }
    }

    func openFind() {
        guard isReady else {
            pendingFind = true
            return
        }
        retainedWebView?.evaluateJavaScript("window.panelDualNotes?.openFind()")
    }

    func flush() {
        retainedWebView?.evaluateJavaScript(
            "void window.panelDualNotes?.flush().catch(() => {})"
        )
    }

    nonisolated func userContentController(
        _ userContentController: WKUserContentController,
        didReceive message: WKScriptMessage
    ) {
        MainActor.assumeIsolated {
            guard message.frameInfo.isMainFrame,
                  let payload = message.body as? [String: Any],
                  let requestID = payload["requestId"] as? String else { return }
            do {
                let data = try JSONSerialization.data(withJSONObject: payload)
                let request = try JSONDecoder().decode(BridgeRequest.self, from: data)
                Task { await handle(request) }
            } catch {
                reply(requestID, error: APIError(message: "无法读取双栏操作，请重试。"))
            }
        }
    }

    private func handle(_ request: BridgeRequest) async {
        do {
            guard request.type == "request" else { return }
            switch request.method {
            case "load":
                guard let end = request.end, let days = request.days,
                      days == 1 || days == 10 else {
                    throw APIError(message: "无效的日期范围。")
                }
                let page = try await APIClient.shared.getDualNotes(end: end, days: days)
                reply(request.requestId, data: page)
            case "save":
                guard let day = request.day else {
                    throw APIError(message: "缺少要保存的双栏内容。")
                }
                let saved = try await APIClient.shared.putDualNotes(day)
                reply(request.requestId, data: saved)
            default:
                throw APIError(message: "不支持的双栏操作。")
            }
        } catch {
            reply(request.requestId, error: error)
        }
    }

    private func reply<T: Encodable>(_ requestID: String, data: T) {
        sendReply(BridgeSuccess(requestId: requestID, ok: true, data: data))
    }

    private func reply(_ requestID: String, error: Error) {
        let apiError = error as? APIError
        sendReply(BridgeFailure(
            requestId: requestID, ok: false, error: error.localizedDescription,
            status: apiError?.status
        ))
    }

    private func sendReply<T: Encodable>(_ reply: T) {
        guard let data = try? JSONEncoder().encode(reply),
              let json = String(data: data, encoding: .utf8) else { return }
        retainedWebView?.evaluateJavaScript("window.panelDualNotes.receive(\(json))")
    }

    nonisolated func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        MainActor.assumeIsolated {
            webView.evaluateJavaScript(
                "typeof window.panelDualNotes?.openFind === 'function'"
            ) { [weak self] result, _ in
                guard let self else { return }
                self.isReady = result as? Bool == true
                if !self.isReady {
                    self.loadError = "双栏编辑器未能初始化，请重新加载。"
                } else if self.pendingFind {
                    self.pendingFind = false
                    self.openFind()
                }
            }
        }
    }

    nonisolated func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        MainActor.assumeIsolated {
            isReady = false
            loadError = "编辑器意外停止。重新加载后会恢复本机保存的草稿。"
        }
    }

    nonisolated func webView(
        _ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!,
        withError error: Error
    ) {
        MainActor.assumeIsolated { loadError = error.localizedDescription }
    }

    nonisolated func webView(
        _ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error
    ) {
        MainActor.assumeIsolated { loadError = error.localizedDescription }
    }

    nonisolated func webView(
        _ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
    ) {
        MainActor.assumeIsolated {
            guard let url = navigationAction.request.url else {
                decisionHandler(.cancel)
                return
            }
            if navigationAction.navigationType == .linkActivated {
                if url.scheme == "https" || url.scheme == "http" {
                    NSWorkspace.shared.open(url)
                }
                decisionHandler(.cancel)
                return
            }
            decisionHandler(url.isFileURL ? .allow : .cancel)
        }
    }

    private struct BridgeRequest: Decodable {
        let type: String
        let requestId: String
        let method: String
        let end: String?
        let days: Int?
        let day: DualNotesDay?
    }

    private struct BridgeSuccess<T: Encodable>: Encodable {
        let requestId: String
        let ok: Bool
        let data: T
    }

    private struct BridgeFailure: Encodable {
        let requestId: String
        let ok: Bool
        let error: String
        let status: Int?
    }
}

/// WKUserContentController retains handlers; this proxy avoids a controller/webview cycle.
private final class WeakDualNotesHandler: NSObject, WKScriptMessageHandler {
    weak var target: (any WKScriptMessageHandler)?

    init(target: any WKScriptMessageHandler) { self.target = target }

    func userContentController(
        _ userContentController: WKUserContentController,
        didReceive message: WKScriptMessage
    ) {
        target?.userContentController(userContentController, didReceive: message)
    }
}
