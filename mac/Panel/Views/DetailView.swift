import SwiftUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isShowingRecurring = false

    var body: some View {
        @Bindable var bindable = state

        let title: String?
        let focusForCard: FocusResult?
        let bufferBinding: Binding<String>
        let hasContent: Bool

        switch state.contentMode {
        case .micro:
            if let today = state.microToday {
                title = today.name
                focusForCard = microFocus(today: today)
                bufferBinding = $bindable.microBuffer
                hasContent = true
            } else {
                title = nil
                focusForCard = nil
                bufferBinding = .constant("")
                hasContent = false
            }
        case .todolist:
            if let detail = state.detail {
                title = detail.name
                focusForCard = state.focus?.found == true ? state.focus : nil
                bufferBinding = $bindable.editorBuffer
                hasContent = true
            } else {
                title = nil
                focusForCard = nil
                bufferBinding = .constant("")
                hasContent = false
            }
        }

        return Group {
            if hasContent, let title {
                VStack(alignment: .leading, spacing: 16) {
                    if let focus = focusForCard {
                        FocusCard(focus: focus)
                            .id(focus.text ?? "")
                            .transition(.opacity)
                    }
                    MilkDownEditor(
                        text: bufferBinding,
                        pageZoom: state.editorPageZoom,
                        scrollText: $bindable.pendingScrollText
                    )
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .animation(.easeInOut(duration: 0.18), value: focusForCard?.text)
                .padding(.vertical, 12)
                .frame(minWidth: 520)
                .navigationTitle(title)
            } else if state.contentMode == .micro {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                EmptyState(selectedName: state.selectedName)
            }
        }
        .background(saveShortcut)
        .toolbar { toolbarItems }
        .onAppear { syncMicroIfNeeded() }
        .onChange(of: state.contentMode) { _, _ in syncMicroIfNeeded() }
        .onChange(of: state.microToday?.name) { _, _ in syncMicroIfNeeded() }
        .sheet(isPresented: $isShowingRecurring) {
            RecurringSheet().environment(state)
        }
    }

    @ToolbarContentBuilder
    private var toolbarItems: some ToolbarContent {
        ToolbarItem { modeToggleButton }
        ToolbarItem { anchorButton }
        ToolbarItem { fontSmallerButton }
        ToolbarItem { fontLargerButton }
        if state.contentMode == .micro {
            ToolbarItem {
                Button { isShowingRecurring = true } label: {
                    Image(systemName: "repeat")
                }
                .help("管理每日复发 todo")
            }
        }
    }

    private var modeToggleButton: some View {
        Button {
            state.contentMode = state.contentMode == .micro ? .todolist : .micro
        } label: {
            Image(systemName: state.contentMode == .micro ? "list.bullet.rectangle" : "calendar")
        }
        .help(state.contentMode == .micro ? "切到 Todolist" : "切到当日 Micro")
    }

    @ViewBuilder
    private var anchorButton: some View {
        let selected = state.selectedName
        let isAnchored = selected != nil && selected == state.anchor
        let inMicro = state.contentMode == .micro
        Button {
            guard let name = selected else { return }
            Task { await state.setAnchor(name: isAnchored ? nil : name) }
        } label: {
            Image(systemName: isAnchored ? "pin.fill" : "pin")
        }
        .help(inMicro ? "锚定按钮 (todolist 模式下生效)" : (isAnchored ? "取消锚定" : "锚定为主 todolist"))
        .tint(isAnchored ? Color.anchorAccent : Color.secondary)
        .disabled(inMicro || selected == nil)
    }

    private var fontSmallerButton: some View {
        Button {
            state.editorPageZoom = max(0.7, state.editorPageZoom - 0.1)
        } label: {
            Image(systemName: "textformat.size.smaller")
        }
        .help("缩小字号")
        .disabled(state.editorPageZoom <= 0.7)
    }

    private var fontLargerButton: some View {
        Button {
            state.editorPageZoom = min(1.8, state.editorPageZoom + 0.1)
        } label: {
            Image(systemName: "textformat.size.larger")
        }
        .help("放大字号")
        .disabled(state.editorPageZoom >= 1.8)
    }

    private var saveShortcut: some View {
        Button("") {
            switch state.contentMode {
            case .todolist:
                Task { await state.saveDetail() }
            case .micro:
                guard let mt = state.microToday, state.microBuffer != mt.rawMD else { return }
                Task { await state.saveMicroToday(rawMD: state.microBuffer) }
            }
        }
        .keyboardShortcut("s", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
    }

    private func syncMicroIfNeeded() {
        guard state.contentMode == .micro, let today = state.microToday else { return }
        if state.microLoadedFor != today.name {
            state.microBuffer = today.rawMD
            state.microLoadedFor = today.name
        }
    }

    private func microFocus(today: Todolist) -> FocusResult? {
        guard let first = today.items.first(where: { !$0.checked && !$0.skip }) else {
            return nil
        }
        return FocusResult(
            anchored: today.name,
            found: true,
            text: first.text,
            lineNo: first.lineNo,
            breadcrumbs: first.parentChain
        )
    }
}

private struct EmptyState: View {
    let selectedName: String?
    var body: some View {
        ContentUnavailableView {
            Label(
                selectedName == nil ? "未选中 todolist" : "todolist 加载失败",
                systemImage: selectedName == nil ? "list.bullet.rectangle" : "exclamationmark.triangle"
            )
        } description: {
            Text(selectedName == nil ? "在左侧选一个开始" : "")
        }
    }
}
