import SwiftUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isShowingRecurring = false
    @State private var isShowingRating = false
    @State private var showFindBar: Bool = false
    @State private var findText: String = ""
    @State private var findCommand: EditorFindCommand? = nil
    @FocusState private var findFieldFocused: Bool

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
                    if showFindBar { findBar }
                    MilkDownEditor(
                        text: bufferBinding,
                        pageZoom: state.editorPageZoom,
                        scrollText: $bindable.pendingScrollText,
                        findCommand: $findCommand
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
        .overlay(alignment: .bottomTrailing) {
            if isShowingRating {
                RatingReferencePanel(onClose: { isShowingRating = false })
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut(duration: 0.18), value: isShowingRating)
        .background(saveShortcut)
        .background(findShortcut)
        .background(findNextShortcut)
        .background(findPrevShortcut)
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
        ToolbarItem { ratingButton }
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

    /// 星级评估浮窗 toggle。
    private var ratingButton: some View {
        Button {
            isShowingRating.toggle()
        } label: {
            Image(systemName: isShowingRating ? "star.fill" : "star")
        }
        .help(isShowingRating ? "关闭星级评估" : "打开星级评估浮窗")
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

    /// 顶部 find bar — cmd+F 显示, ESC 关闭。输入直接 incremental 搜索, 回车跳下一个。
    @ViewBuilder
    private var findBar: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("搜索 (回车下一个 / Shift+回车上一个)", text: $findText)
                .textFieldStyle(.roundedBorder)
                .focused($findFieldFocused)
                .onSubmit { issueFind(backwards: false) }
                .onChange(of: findText) { _, new in
                    findCommand = EditorFindCommand(id: UUID(), query: new, backwards: false)
                }
            Button {
                issueFind(backwards: true)
            } label: { Image(systemName: "chevron.up") }
            .buttonStyle(.borderless)
            .help("上一个 (⇧⌘G)")
            .disabled(findText.isEmpty)
            Button {
                issueFind(backwards: false)
            } label: { Image(systemName: "chevron.down") }
            .buttonStyle(.borderless)
            .help("下一个 (⌘G)")
            .disabled(findText.isEmpty)
            Button {
                closeFindBar()
            } label: { Image(systemName: "xmark.circle.fill") }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("关闭 (Esc)")
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 6))
        .padding(.horizontal, 12)
        .onExitCommand { closeFindBar() }
    }

    private func issueFind(backwards: Bool) {
        guard !findText.isEmpty else { return }
        findCommand = EditorFindCommand(id: UUID(), query: findText, backwards: backwards)
    }

    private func closeFindBar() {
        showFindBar = false
        findText = ""
        // 清高亮
        findCommand = EditorFindCommand(id: UUID(), query: "", backwards: false)
    }

    /// cmd+F 打开 find bar 并 focus 输入框。
    private var findShortcut: some View {
        Button("") {
            showFindBar = true
            findFieldFocused = true
        }
        .keyboardShortcut("f", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
    }

    /// cmd+G 下一个匹配 (find bar 必须先打开)。
    private var findNextShortcut: some View {
        Button("") {
            issueFind(backwards: false)
        }
        .keyboardShortcut("g", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
        .disabled(!showFindBar || findText.isEmpty)
    }

    /// shift+cmd+G 上一个。
    private var findPrevShortcut: some View {
        Button("") {
            issueFind(backwards: true)
        }
        .keyboardShortcut("g", modifiers: [.command, .shift])
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
        .disabled(!showFindBar || findText.isEmpty)
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
