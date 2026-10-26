import AppKit
import Combine
import Foundation
import WebKit

/// The embedded page always uses the local service as its top-level origin.
/// Keeping this policy independent of WebKit actions makes redirect and link
/// handling share the same rules.
enum AutoDLPage {
    static let url = URL(string: "http://127.0.0.1:8765/")!

    enum NavigationDecision: Equatable {
        case allow
        case openExternally
        case cancel
    }

    static func isLocalURL(_ url: URL) -> Bool {
        guard let components = safeHTTPComponents(url) else { return false }
        return components.scheme?.lowercased() == "http"
            && components.percentEncodedHost == "127.0.0.1"
            && components.port == 8765
    }

    static func navigationDecision(
        for url: URL?, userActivated: Bool, isMainFrame: Bool = true
    ) -> NavigationDecision {
        guard isMainFrame, let url, safeHTTPComponents(url) != nil else {
            return .cancel
        }
        if isLocalURL(url) { return .allow }
        return userActivated ? .openExternally : .cancel
    }

    private static func safeHTTPComponents(_ url: URL) -> URLComponents? {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let scheme = components.scheme?.lowercased(),
              scheme == "http" || scheme == "https",
              let host = components.host, !host.isEmpty,
              components.user == nil, components.password == nil else { return nil }
        return components
    }
}

/// Each window retains this controller and its WebView across tab changes.
/// Only an explicit refresh reconnects; no backend jobs are stopped by this UI.
@MainActor
final class AutoDLController: NSObject, ObservableObject, WKNavigationDelegate, WKUIDelegate {
    @Published private(set) var loadError: String?
    @Published private(set) var isLoading = false

    private let webViewFactory: () -> WKWebView
    private var retainedWebView: WKWebView?

    init(webViewFactory: (() -> WKWebView)? = nil) {
        self.webViewFactory = webViewFactory ?? Self.makeWebView
        super.init()
    }

    private static func makeWebView() -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.defaultWebpagePreferences.allowsContentJavaScript = true
        return WKWebView(frame: .zero, configuration: configuration)
    }

    func webView() -> WKWebView {
        if let retainedWebView { return retainedWebView }
        let webView = webViewFactory()
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.allowsBackForwardNavigationGestures = false
        retainedWebView = webView
        loadHome(in: webView)
        return webView
    }

    func reload() {
        guard let retainedWebView else {
            _ = webView()
            return
        }
        loadHome(in: retainedWebView)
    }

    func openInBrowser() {
        NSWorkspace.shared.open(AutoDLPage.url)
    }

    private func loadHome(in webView: WKWebView) {
        loadError = nil
        isLoading = true
        let request = URLRequest(
            url: AutoDLPage.url, cachePolicy: .reloadIgnoringLocalCacheData,
            timeoutInterval: 10
        )
        webView.load(request)
    }

    private func failed(_ error: Error) {
        let failure = error as NSError
        // Navigation cancellation is expected when refreshing or opening a link
        // externally; it does not mean the local service is unavailable.
        guard !(failure.domain == NSURLErrorDomain && failure.code == NSURLErrorCancelled)
        else { return }
        isLoading = false
        loadError = error.localizedDescription
    }

    nonisolated func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        MainActor.assumeIsolated {
            isLoading = false
            loadError = nil
        }
    }

    nonisolated func webView(
        _ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!,
        withError error: Error
    ) {
        MainActor.assumeIsolated { failed(error) }
    }

    nonisolated func webView(
        _ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error
    ) {
        MainActor.assumeIsolated { failed(error) }
    }

    nonisolated func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        MainActor.assumeIsolated {
            isLoading = false
            loadError = "页面进程已停止，请重新连接。正在运行的抢卡任务不受影响。"
        }
    }

    nonisolated func webView(
        _ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
    ) {
        MainActor.assumeIsolated {
            let isMainFrame = navigationAction.targetFrame?.isMainFrame
                ?? navigationAction.sourceFrame.isMainFrame
            let userActivated = navigationAction.navigationType == .linkActivated
                && navigationAction.sourceFrame.isMainFrame
            let decision = AutoDLPage.navigationDecision(
                for: navigationAction.request.url, userActivated: userActivated,
                isMainFrame: isMainFrame
            )
            // A target="_blank" link reuses this top-level page. Script-created
            // windows never receive a new WebView or trigger an external browser.
            if navigationAction.targetFrame == nil {
                decisionHandler(.cancel)
                if userActivated {
                    followNewWindowLink(navigationAction.request, decision: decision, in: webView)
                }
                return
            }
            switch decision {
            case .allow:
                decisionHandler(.allow)
            case .openExternally:
                decisionHandler(.cancel)
                if let url = navigationAction.request.url { NSWorkspace.shared.open(url) }
            case .cancel:
                decisionHandler(.cancel)
            }
        }
    }

    nonisolated func webView(
        _ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse,
        decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void
    ) {
        MainActor.assumeIsolated {
            guard navigationResponse.isForMainFrame,
                  let url = navigationResponse.response.url, AutoDLPage.isLocalURL(url),
                  navigationResponse.canShowMIMEType else {
                decisionHandler(.cancel)
                return
            }
            if let response = navigationResponse.response as? HTTPURLResponse,
               response.statusCode >= 400 {
                isLoading = false
                loadError = "AutoDL 本地服务返回 HTTP \(response.statusCode)，请检查服务后重新连接。"
                decisionHandler(.cancel)
                return
            }
            decisionHandler(.allow)
        }
    }

    nonisolated func webView(
        _ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
        for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures
    ) -> WKWebView? {
        MainActor.assumeIsolated {
            guard navigationAction.targetFrame == nil,
                  navigationAction.navigationType == .linkActivated,
                  navigationAction.sourceFrame.isMainFrame else { return nil }
            let decision = AutoDLPage.navigationDecision(
                for: navigationAction.request.url, userActivated: true
            )
            followNewWindowLink(navigationAction.request, decision: decision, in: webView)
            return nil
        }
    }

    private func followNewWindowLink(
        _ request: URLRequest, decision: AutoDLPage.NavigationDecision, in webView: WKWebView
    ) {
        switch decision {
        case .allow:
            webView.load(request)
        case .openExternally:
            if let url = request.url { NSWorkspace.shared.open(url) }
        case .cancel:
            break
        }
    }
}
