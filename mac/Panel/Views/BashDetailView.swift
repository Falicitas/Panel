import SwiftUI

struct BashDetailView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        Group {
            if state.bashHistory.isEmpty {
                ContentUnavailableView {
                    Label("无 bash 历史", systemImage: "clock.arrow.circlepath")
                } description: {
                    Text("执行命令后这里会持续累积")
                }
            } else {
                List {
                    ForEach(state.bashHistory) { entry in
                        HistoryRow(entry: entry)
                            .contentShape(Rectangle())
                            .onTapGesture { state.loadBashEntryToTranscript(entry) }
                    }
                }
                .listStyle(.inset)
            }
        }
        .navigationTitle("Bash 历史")
        .toolbar {
            ToolbarItem {
                Button {
                    Task { await state.loadBashHistory() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("刷新历史")
            }
        }
    }
}

private struct HistoryRow: View {
    let entry: BashHistoryEntry

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text(entry.cmd)
                    .font(.system(.body, design: .monospaced))
                    .lineLimit(2)
                Spacer()
                Text("exit \(entry.exitCode)")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(entry.exitCode == 0 ? .green : .red)
            }
            HStack {
                Text(timestamp)
                Spacer()
                Text("\(entry.durationMs) ms")
            }
            .font(.caption.monospacedDigit())
            .foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
    }

    private var timestamp: String {
        let date = Date(timeIntervalSince1970: TimeInterval(entry.createdAt))
        let f = DateFormatter()
        f.dateFormat = "MM-dd HH:mm:ss"
        return f.string(from: date)
    }
}
