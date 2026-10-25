import SwiftUI
import WebKit

struct AutoDLView: View {
    @Environment(AppState.self) private var state
    @EnvironmentObject private var controller: AutoDLController

    var body: some View {
        AutoDLPane(controller: controller, pageZoom: state.editorPageZoom)
    }
}

private struct AutoDLPane: View {
    @ObservedObject var controller: AutoDLController
    let pageZoom: Double

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                Label("AutoDL", systemImage: "server.rack")
                    .font(.callout.weight(.medium))
                Text("127.0.0.1:8765")
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                Spacer(minLength: 12)
                if controller.isLoading {
                    ProgressView()
                        .controlSize(.small)
                        .accessibilityLabel("正在连接 AutoDL 本地服务")
                }
                Button(action: controller.reload) {
                    Label("刷新", systemImage: "arrow.clockwise")
                }
                .labelStyle(.iconOnly)
                .help("重新加载 AutoDL 页面，保留后台任务")
                Button(action: controller.openInBrowser) {
                    Label("在浏览器打开", systemImage: "arrow.up.right.square")
                }
                .labelStyle(.iconOnly)
                .help("在默认浏览器打开 AutoDL 控制台")
            }
            .controlSize(.small)
            .buttonStyle(.borderless)
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            Divider()
            ZStack {
                AutoDLWebView(controller: controller, pageZoom: pageZoom)
                if let error = controller.loadError {
                    ContentUnavailableView {
                        Label("本机服务未连接", systemImage: "network.slash")
                    } description: {
                        VStack(spacing: 8) {
                            Text("请先在终端启动 AutoDL 本地服务，再重新连接。")
                            Text(error)
                                .font(.caption)
                                .textSelection(.enabled)
                        }
                    } actions: {
                        Button("重新连接", action: controller.reload)
                            .buttonStyle(.borderedProminent)
                        Button("在浏览器打开", action: controller.openInBrowser)
                            .buttonStyle(.bordered)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(.background)
                }
            }
        }
        .background(.background)
    }
}

private struct AutoDLWebView: NSViewRepresentable {
    let controller: AutoDLController
    let pageZoom: Double

    func makeNSView(context: Context) -> WKWebView {
        controller.webView()
    }

    func updateNSView(_ webView: WKWebView, context: Context) {
        let zoom = CGFloat(pageZoom)
        if webView.pageZoom != zoom { webView.pageZoom = zoom }
    }
}
