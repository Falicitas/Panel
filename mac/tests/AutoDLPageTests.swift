import AppKit
import Foundation
import Observation
import SwiftUI
import WebKit

/// Exercises the production controller with a WebView that never loads a URL.
@main
struct AutoDLPageTests {
    @MainActor
    static func main() throws {
        NSApplication.shared.setActivationPolicy(.accessory)
        testSidebarEntry()
        testNavigationBoundary()
        testRetainedPageAndRecovery()
        testIndependentWindowPages()
        try testLoopbackTransportException()
        print("AutoDL navigation, retained page, recovery, and local HTTP checks passed")
    }

    private static func testSidebarEntry() {
        expect(AppTab.autodl.isEnabled, "The AutoDL sidebar entry is enabled")
        expect(AppTab.autodl.label == "AutoDL", "The entry identifies the independent service")
        expect(AppTab.allCases.filter { $0 == .autodl }.count == 1,
               "AutoDL appears exactly once in the sidebar enumeration")
        expect(AppTab.autodl.comingSoonHint.isEmpty, "An enabled entry has no disabled-module hint")
    }

    private static func testNavigationBoundary() {
        expect(AutoDLPage.url.absoluteString == "http://127.0.0.1:8765/",
               "Use the existing independent AutoDL service")
        for value in [
            "http://127.0.0.1:8765/",
            "http://127.0.0.1:8765/index.html",
            "http://127.0.0.1:8765/?view=instances#jobs",
        ] {
            let url = URL(string: value)!
            expect(AutoDLPage.isLocalURL(url), "The exact local origin is accepted")
            expect(AutoDLPage.navigationDecision(for: url, userActivated: false) == .allow,
                   "The initial local load is allowed without a click")
            expect(AutoDLPage.navigationDecision(for: url, userActivated: true) == .allow,
                   "Local links stay in the control panel")
            expect(AutoDLPage.navigationDecision(for: url, userActivated: false,
                                                isMainFrame: false) == .cancel,
                   "Subframes cannot navigate the embedded control panel")
        }
        for value in [
            "http://127.0.0.1:8766/", "http://127.0.0.1/",
            "http://localhost:8765/", "http://127.1:8765/",
            "http://[::1]:8765/", "http://2130706433:8765/",
            "http://127.0.0.1.example.com:8765/", "http://127.0.0.1.:8765/",
            "https://127.0.0.1:8765/", "https://example.com/",
            "file:///tmp/autodl.html", "about:blank", "javascript:alert(1)",
            "data:text/html,hello", "ssh://127.0.0.1:8765/",
        ] {
            let url = URL(string: value)!
            expect(!AutoDLPage.isLocalURL(url), "Reject a different origin: \(value)")
            expect(AutoDLPage.navigationDecision(for: url, userActivated: false) == .cancel,
                   "Redirects and scripts cannot leave the local origin: \(value)")
        }
        for value in [
            "http://user@127.0.0.1:8765/",
            "http://user:password@127.0.0.1:8765/",
            "http://@127.0.0.1:8765/",
            "http://127.0.0.1:8765@example.com/",
            "https://user:password@example.com/",
        ] {
            let url = URL(string: value)!
            expect(!AutoDLPage.isLocalURL(url), "User information never identifies the local origin")
            for clicked in [false, true] {
                expect(AutoDLPage.navigationDecision(for: url, userActivated: clicked) == .cancel,
                       "A click must not bypass the user-information restriction")
            }
        }
        for value in ["https://example.com/help", "http://example.com/help"] {
            let url = URL(string: value)!
            expect(AutoDLPage.navigationDecision(for: url, userActivated: true) == .openExternally,
                   "Only an explicit main-frame web link opens in the browser")
            expect(AutoDLPage.navigationDecision(for: url, userActivated: true,
                                                isMainFrame: false) == .cancel,
                   "Subframes do not open the system browser")
        }
        for value in ["file:///tmp/example", "javascript:alert(1)", "ssh://example.com/"] {
            expect(AutoDLPage.navigationDecision(for: URL(string: value)!,
                                                userActivated: true) == .cancel,
                   "A click cannot invoke files, scripts or arbitrary applications")
        }
        expect(AutoDLPage.navigationDecision(for: nil, userActivated: true) == .cancel,
               "Missing navigation URL fails closed")
    }

    @MainActor
    private static func testRetainedPageAndRecovery() {
        let spy = SpyWebView(frame: .zero, configuration: WKWebViewConfiguration())
        var factoryCalls = 0
        let controller = AutoDLController(webViewFactory: {
            factoryCalls += 1
            return spy
        })
        expect(factoryCalls == 0, "Do not create a WebView before AutoDL is first selected")
        let first = controller.webView()
        expect(first === spy && factoryCalls == 1, "Create the injected page once")
        expect(spy.requests.count == 1, "The first visit loads one page")
        expect(spy.requests[0].url == AutoDLPage.url, "Load the control panel as the top-level document")
        expect(spy.requests[0].httpMethod == "GET", "Selecting the tab only reads the homepage")
        expect(spy.requests[0].value(forHTTPHeaderField: "Authorization") == nil,
               "The Mac page does not copy AutoDL credentials")
        expect(controller.isLoading, "Expose the initial loading state")
        for _ in 0..<20 {
            expect(controller.webView() === first, "Reattaching after a tab change preserves the page")
        }
        expect(factoryCalls == 1 && spy.requests.count == 1,
               "SwiftUI view recreation must not duplicate loads or discard pending form state")

        controller.webView(spy, didFailProvisionalNavigation: nil,
                           withError: URLError(.cannotConnectToHost))
        expect(controller.loadError?.isEmpty == false && !controller.isLoading,
               "A stopped local service exposes a retryable load failure")
        controller.reload()
        expect(controller.loadError == nil && controller.isLoading,
               "Explicit retry clears the stale load error")
        expect(spy.requests.count == 2 && spy.requests.last?.url == AutoDLPage.url,
               "Retry loads the fixed local homepage exactly once")
        controller.webView(spy, didFail: nil, withError: URLError(.cancelled))
        expect(controller.loadError == nil && controller.isLoading,
               "Cancellation of an old navigation must not report refresh failure")
        controller.webView(spy, didFinish: nil)
        expect(controller.loadError == nil && !controller.isLoading,
               "A completed navigation clears the loading state")
        expect(controller.webView() === first && spy.requests.count == 2,
               "Returning from another tab does not reload a recovered page")

        controller.webView(spy, didFail: nil, withError: URLError(.networkConnectionLost))
        expect(controller.loadError?.isEmpty == false && !controller.isLoading,
               "Failures after a provisional load also reach the error view")
        controller.reload()
        controller.webView(spy, didFinish: nil)
        expect(controller.loadError == nil && spy.requests.count == 3,
               "A committed-navigation error can recover through the same retry action")
        controller.webViewWebContentProcessDidTerminate(spy)
        expect(controller.loadError?.isEmpty == false && !controller.isLoading,
               "A terminated WebKit process exposes recovery instead of leaving a blank page")
        controller.reload()
        expect(controller.webView() === first && spy.requests.count == 4,
               "Recovery retains the same WebView and does not submit any backend operation")
        expect(spy.requests.allSatisfy { $0.url == AutoDLPage.url && $0.httpMethod == "GET" },
               "Every controller request is a homepage read")
    }

    @MainActor
    private static func testIndependentWindowPages() {
        let firstPage = SpyWebView(frame: .zero, configuration: WKWebViewConfiguration())
        let secondPage = SpyWebView(frame: .zero, configuration: WKWebViewConfiguration())
        var firstFactoryCalls = 0
        var secondFactoryCalls = 0
        let firstWindow = AutoDLController(webViewFactory: {
            firstFactoryCalls += 1
            return firstPage
        })
        let secondWindow = AutoDLController(webViewFactory: {
            secondFactoryCalls += 1
            return secondPage
        })
        let first = firstWindow.webView()
        let second = secondWindow.webView()
        expect(first !== second, "Different windows must never share an NSView")
        for _ in 0..<10 {
            expect(firstWindow.webView() === first, "The first window retains its own page")
            expect(secondWindow.webView() === second, "The second window retains its own page")
        }
        expect(firstFactoryCalls == 1 && secondFactoryCalls == 1,
               "Each window creates exactly one page across sidebar changes")
        expect(firstPage.requests.count == 1 && secondPage.requests.count == 1,
               "Reattaching either window does not reload either page")
        firstWindow.webView(firstPage, didFailProvisionalNavigation: nil,
                            withError: URLError(.cannotConnectToHost))
        expect(firstWindow.loadError != nil && secondWindow.loadError == nil,
               "One window's navigation failure does not cover the other window")
        firstWindow.reload()
        expect(firstPage.requests.count == 2 && secondPage.requests.count == 1,
               "Refreshing one window does not reload the other window")
        expect(firstWindow.webView() === first && secondWindow.webView() === second,
               "Both windows retain their original NSViews after recovery")
    }

    private static func testLoopbackTransportException() throws {
        guard CommandLine.arguments.count == 2 else {
            preconditionFailure("Pass the generated Panel Info.plist path")
        }
        let data = try Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))
        let plist = try PropertyListSerialization.propertyList(from: data, format: nil) as! [String: Any]
        let ats = plist["NSAppTransportSecurity"] as? [String: Any]
        let domains = ats?["NSExceptionDomains"] as? [String: [String: Any]]
        expect(domains.map { Set($0.keys) } == Set(["127.0.0.1"]),
               "The generated app metadata permits only the exact loopback host")
        expect(domains?["127.0.0.1"]?["NSExceptionAllowsInsecureHTTPLoads"] as? Bool == true,
               "The fixed loopback HTTP service has its explicit ATS exception")
        for key in ["NSAllowsArbitraryLoads", "NSAllowsArbitraryLoadsInWebContent",
                    "NSAllowsArbitraryLoadsForMedia", "NSAllowsLocalNetworking"] {
            expect(ats?[key] == nil, "Do not add a broad ATS bypass: \(key)")
        }
    }

    private static func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
        precondition(condition(), message)
    }
}

/// The real ContentView and AutoDLView compile against their state dependencies.
/// No shared AppState owns an AutoDLController; ContentView owns one per window.
@MainActor
@Observable
final class AppState {
    var editorPageZoom = 1.0
    var selectedTab: AppTab = .autodl
    var lastError: String?

    func startPolling() async {}
}

/// Avoid linking the unrelated modules or starting their services in this test.
struct MainLayout: View {
    var body: some View { EmptyView() }
}

@MainActor
private final class SpyWebView: WKWebView {
    private(set) var requests: [URLRequest] = []

    override func load(_ request: URLRequest) -> WKNavigation? {
        requests.append(request)
        return nil
    }
}
