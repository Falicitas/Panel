import SwiftUI

struct BashContentView: View {
    @Environment(AppState.self) private var state
    @State private var input = ""

    var body: some View {
        VStack(spacing: 0) {
            transcriptScroll
            Divider()
            inputArea
        }
        .navigationTitle("Bash")
        .toolbar {
            ToolbarItem {
                Menu {
                    ForEach(state.bashHistory.prefix(20)) { entry in
                        Button(entry.cmd) { input = entry.cmd }
                    }
                } label: {
                    Image(systemName: "clock.arrow.circlepath")
                }
                .help("从历史选取")
                .disabled(state.bashHistory.isEmpty)
            }
        }
        .task { await state.loadBashHistory() }
    }

    private var transcriptScroll: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    if state.bashTranscript.isEmpty {
                        ContentUnavailableView {
                            Label("REPL 空", systemImage: "terminal")
                        } description: {
                            Text("zsh -i — Cmd+Return 执行,.zshrc PATH/alias 已 source")
                                .multilineTextAlignment(.center)
                        }
                        .frame(maxWidth: .infinity, minHeight: 240)
                    } else {
                        ForEach(state.bashTranscript) { entry in
                            TranscriptCell(entry: entry).id(entry.id)
                        }
                    }
                }
                .padding(12)
            }
            .onChange(of: state.bashTranscript.count) { _, _ in
                guard let last = state.bashTranscript.last else { return }
                withAnimation { proxy.scrollTo(last.id, anchor: .bottom) }
            }
        }
    }

    private var inputArea: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(state.bashIsRunning ? "执行中..." : "$ zsh -i -c …")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button {
                    submit()
                } label: {
                    HStack(spacing: 4) {
                        Image(systemName: "play.fill")
                        Text("Run")
                    }
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(state.bashIsRunning || trimmedInput.isEmpty)
            }
            TextEditor(text: $input)
                .font(.system(.body, design: .monospaced))
                .frame(minHeight: 80, maxHeight: 200)
                .scrollContentBackground(.hidden)
                .background(.background)
                .overlay(
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(Color.secondary.opacity(0.3), lineWidth: 0.5)
                )
        }
        .padding(12)
    }

    private var trimmedInput: String {
        input.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func submit() {
        let cmd = trimmedInput
        guard !cmd.isEmpty, !state.bashIsRunning else { return }
        input = ""
        Task { await state.execCommand(cmd) }
    }
}

private struct TranscriptCell: View {
    let entry: BashHistoryEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text("$").foregroundStyle(.secondary)
                Text(entry.cmd)
                    .font(.system(.body, design: .monospaced))
                    .textSelection(.enabled)
                Spacer()
                ExitBadge(code: entry.exitCode, durationMs: entry.durationMs)
            }
            if !entry.stdout.isEmpty {
                Text(entry.stdout)
                    .font(.system(.callout, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            if !entry.stderr.isEmpty {
                Text(entry.stderr)
                    .font(.system(.callout, design: .monospaced))
                    .textSelection(.enabled)
                    .foregroundStyle(.red)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .padding(8)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 6))
    }
}

private struct ExitBadge: View {
    let code: Int
    let durationMs: Int64

    var body: some View {
        HStack(spacing: 6) {
            Text("\(durationMs) ms")
                .font(.caption.monospacedDigit())
                .foregroundStyle(.tertiary)
            Text("exit \(code)")
                .font(.caption.weight(.medium))
                .foregroundStyle(code == 0 ? .green : .red)
        }
    }
}
