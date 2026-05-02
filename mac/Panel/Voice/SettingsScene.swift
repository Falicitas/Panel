import SwiftUI

/// Cmd+, 触发的 SwiftUI Settings scene 内容。
/// OpenRouter API key 输入 + 测试连接,保存到 UserDefaults。
struct VoiceSettingsView: View {
    @State private var keyInput: String = KeyStore.get() ?? ""
    @State private var modelInput: String = KeyStore.getModel()
    @State private var testStatus: TestStatus = .idle
    @State private var saved: Bool = false

    enum TestStatus: Equatable {
        case idle
        case testing
        case success
        case failure(String)
    }

    var body: some View {
        Form {
            Section("OpenRouter") {
                SecureField("API Key", text: $keyInput, prompt: Text("sk-or-v1-..."))
                    .textFieldStyle(.roundedBorder)
                    .onChange(of: keyInput) { _, _ in
                        saved = false
                        testStatus = .idle
                    }
                TextField("模型", text: $modelInput, prompt: Text(KeyStore.defaultModel))
                    .textFieldStyle(.roundedBorder)
                    .onChange(of: modelInput) { _, _ in saved = false }
                HStack {
                    Button("保存") {
                        KeyStore.set(keyInput)
                        KeyStore.setModel(modelInput.isEmpty ? KeyStore.defaultModel : modelInput)
                        VoiceController.shared.refresh()
                        saved = true
                    }
                    .disabled(keyInput.isEmpty)
                    Button("清除 Key") {
                        KeyStore.delete()
                        keyInput = ""
                        VoiceController.shared.refresh()
                        saved = false
                    }
                    .disabled(keyInput.isEmpty && !KeyStore.hasKey)
                    Button("测试连接") {
                        Task { await testConnection() }
                    }
                    .disabled(keyInput.isEmpty || testStatus == .testing)
                    Spacer()
                    if saved {
                        Label("已保存", systemImage: "checkmark.circle.fill")
                            .foregroundStyle(.green)
                            .font(.callout)
                    }
                }
                statusLine
            }
            Section("使用") {
                Text("F12 在任意可编辑文本框开始录音,再按 F12 结束并转写,文字注入光标处。")
                    .font(.callout).foregroundStyle(.secondary)
                Text("权限要求:输入监控 (F12) + 麦克风 (录音) + 辅助功能 (注入文字)。")
                    .font(.callout).foregroundStyle(.secondary)
                Text("模型推荐:google/gemini-2.5-pro (默认,中文不出繁体) / google/gemini-2.5-flash (便宜快但中文偶有繁体)。OpenAI audio preview 当前 OpenRouter 端 model ID 待确认。")
                    .font(.callout).foregroundStyle(.tertiary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 520, height: 380)
    }

    @ViewBuilder
    private var statusLine: some View {
        switch testStatus {
        case .idle:
            EmptyView()
        case .testing:
            HStack(spacing: 6) {
                ProgressView().controlSize(.small)
                Text("测试中...").font(.callout).foregroundStyle(.secondary)
            }
        case .success:
            Label("Key 有效", systemImage: "checkmark.circle.fill")
                .foregroundStyle(.green).font(.callout)
        case .failure(let msg):
            Label(msg, systemImage: "xmark.circle.fill")
                .foregroundStyle(.red).font(.callout)
        }
    }

    private func testConnection() async {
        testStatus = .testing
        let ok = await Transcriber.validateKey(keyInput)
        testStatus = ok ? .success : .failure("Key 无效或网络错误")
    }
}
