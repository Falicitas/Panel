import SwiftUI

struct MicroContentView: View {
    @Environment(AppState.self) private var state
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
        @Bindable var bindable = state
        MilkDownEditor(text: $bindable.microBuffer)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .onAppear { syncBuffer(today: today) }
            .onChange(of: today.name) { _, _ in syncBuffer(today: today) }
    }

    private func syncBuffer(today: Todolist) {
        if state.microLoadedFor != today.name {
            state.microBuffer = today.rawMD
            state.microLoadedFor = today.name
        }
    }

    private func saveShortcut(for today: Todolist) -> some View {
        Button("") {
            guard state.microBuffer != today.rawMD else { return }
            Task { await state.saveMicroToday(rawMD: state.microBuffer) }
        }
        .keyboardShortcut("s", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
    }
}
