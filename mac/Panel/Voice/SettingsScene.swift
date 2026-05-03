import SwiftUI

/// Cmd+, 触发的 SwiftUI Settings scene 内容。
/// OpenRouter API key + 模型选择,保存到 UserDefaults。
struct VoiceSettingsView: View {
    @State private var keyInput: String = KeyStore.get() ?? ""
    @State private var modelChoice: String = Self.normalizedModelChoice()
    @State private var testStatus: TestStatus = .idle
    @State private var saved: Bool = false
    @State private var groqInput: String = KeyStore.getGroq() ?? ""
    @State private var groqSaved: Bool = false

    /// 内置模型选项。后续如要更多,扩这里 (Transcriber 自动按 model 名选 endpoint)。
    private static let modelOptions: [(label: String, id: String)] = [
        ("Gemini 2.5 Pro (中文稳简体)",         "google/gemini-2.5-pro"),
        ("OpenAI GPT-4o Transcribe (英文 4.1% WER)", "openai/gpt-4o-transcribe"),
    ]

    /// KeyStore 里的 model 如果不在 options 里 (比如老版本设过别的),兜底回 default。
    private static func normalizedModelChoice() -> String {
        let cur = KeyStore.getModel()
        return modelOptions.contains(where: { $0.id == cur }) ? cur : KeyStore.defaultModel
    }

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
                Picker("模型", selection: $modelChoice) {
                    ForEach(Self.modelOptions, id: \.id) { opt in
                        Text(opt.label).tag(opt.id)
                    }
                }
                .pickerStyle(.menu)
                .onChange(of: modelChoice) { _, _ in saved = false }
                HStack {
                    Button("保存") {
                        KeyStore.set(keyInput)
                        KeyStore.setModel(modelChoice)
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
            Section("Groq (英文精听 ASR)") {
                SecureField("API Key", text: $groqInput, prompt: Text("gsk_..."))
                    .textFieldStyle(.roundedBorder)
                    .onChange(of: groqInput) { _, _ in groqSaved = false }
                HStack {
                    Button("保存") {
                        KeyStore.setGroq(groqInput)
                        groqSaved = true
                    }
                    .disabled(groqInput.isEmpty)
                    Spacer()
                    if groqSaved {
                        Label("已保存", systemImage: "checkmark.circle.fill")
                            .foregroundStyle(.green)
                            .font(.callout)
                    }
                }
                Text("英文 audio 用 Groq Whisper Large V3 直连拿词级 timestamp (OpenRouter STT 不返时间戳)。 https://console.groq.com 注册免费, free tier rate limit 7200 audio-sec/hour 足够日用。")
                    .font(.callout).foregroundStyle(.secondary)
            }
            Section("使用") {
                Text("F12 在任意可编辑文本框开始录音,再按 F12 结束并转写,文字注入光标处。最长 5 分钟自动结束。")
                    .font(.callout).foregroundStyle(.secondary)
                Text("权限要求:输入监控 (F12) + 麦克风 (录音) + 辅助功能 (注入文字)。")
                    .font(.callout).foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 520, height: 520)
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
