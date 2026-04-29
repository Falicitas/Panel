import SwiftUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isShowingRecurring = false

    var body: some View {
        Group {
            switch state.contentMode {
            case .micro:    microView
            case .todolist: todolistView
            }
        }
        .toolbar { toolbarItems }
        .sheet(isPresented: $isShowingRecurring) {
            RecurringSheet().environment(state)
        }
    }

    @ViewBuilder
    private var todolistView: some View {
        if let detail = state.detail {
            @Bindable var bindable = state
            VStack(alignment: .leading, spacing: 16) {
                if let focus = state.focus, focus.found {
                    FocusCard(focus: focus)
                }
                MilkDownEditor(text: $bindable.editorBuffer, pageZoom: state.editorPageZoom)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .padding(.vertical, 12)
            .frame(minWidth: 520)
            .navigationTitle(detail.name)
            .background(saveTodoShortcut)
        } else {
            EmptyState(selectedName: state.selectedName)
        }
    }

    @ViewBuilder
    private var microView: some View {
        if let today = state.microToday {
            @Bindable var bindable = state
            VStack(alignment: .leading, spacing: 16) {
                if let mfocus = microFocus(today: today) {
                    FocusCard(focus: mfocus)
                }
                MilkDownEditor(text: $bindable.microBuffer, pageZoom: state.editorPageZoom)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .onAppear { syncMicroBuffer(today: today) }
                    .onChange(of: today.name) { _, _ in syncMicroBuffer(today: today) }
            }
            .padding(.vertical, 12)
            .frame(minWidth: 520)
            .navigationTitle(today.name)
            .background(saveMicroShortcut(today: today))
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
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

    private var saveTodoShortcut: some View {
        Button("") { Task { await state.saveDetail() } }
            .keyboardShortcut("s", modifiers: .command)
            .frame(width: 0, height: 0)
            .opacity(0)
            .accessibilityHidden(true)
    }

    private func saveMicroShortcut(today: Todolist) -> some View {
        Button("") {
            guard state.microBuffer != today.rawMD else { return }
            Task { await state.saveMicroToday(rawMD: state.microBuffer) }
        }
        .keyboardShortcut("s", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
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

    private func syncMicroBuffer(today: Todolist) {
        if state.microLoadedFor != today.name {
            state.microBuffer = today.rawMD
            state.microLoadedFor = today.name
        }
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
