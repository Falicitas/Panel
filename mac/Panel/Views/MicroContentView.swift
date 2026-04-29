import SwiftUI

struct MicroContentView: View {
    @Environment(AppState.self) private var state
    @State private var buffer: String = ""
    @State private var loadedFor: String?  // 当前 buffer 对应的 todolist name
    @State private var isShowingRecurring = false

    var body: some View {
        Group {
            if let today = state.microToday {
                editor(for: today)
                    .navigationTitle(today.name)
                    .background(saveShortcut(for: today))
                    .toolbar {
                        ToolbarItem {
                            Button {
                                isShowingRecurring = true
                            } label: {
                                Label("管理复发", systemImage: "repeat")
                            }
                            .help("管理每日复发 todo")
                        }
                    }
                    .sheet(isPresented: $isShowingRecurring) {
                        RecurringSheet().environment(state)
                    }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
    }

    @ViewBuilder
    private func editor(for today: Todolist) -> some View {
        ScrollView {
            MilkDownEditor(
                text: $buffer,
                onBlur: { saveIfChanged(today: today) }
            )
            .frame(minHeight: 480)
            .padding(.horizontal)
            .padding(.vertical, 12)
        }
        .onAppear { syncBuffer(today: today) }
        .onChange(of: today.name) { _, _ in syncBuffer(today: today) }
        .onChange(of: today.rawMD) { _, newValue in
            // 外部修改 (panel 后台 polling 检测到 .md 文件变化) 同步到 buffer
            if loadedFor == today.name && buffer != newValue {
                buffer = newValue
            }
        }
    }

    private func syncBuffer(today: Todolist) {
        if loadedFor != today.name {
            buffer = today.rawMD
            loadedFor = today.name
        }
    }

    private func saveIfChanged(today: Todolist) {
        guard buffer != today.rawMD else { return }
        Task { await state.saveMicroToday(rawMD: buffer) }
    }

    private func saveShortcut(for today: Todolist) -> some View {
        Button("") { saveIfChanged(today: today) }
            .keyboardShortcut("s", modifiers: .command)
            .frame(width: 0, height: 0)
            .opacity(0)
            .accessibilityHidden(true)
    }
}
