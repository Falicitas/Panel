import SwiftUI

struct DetailView: View {
    @Environment(AppState.self) private var state
    @State private var isShowingRecurring = false
    @State private var isShowingRating = false
    @State private var showFindBar: Bool = false
    @State private var findText: String = ""
    @State private var findCommand: EditorFindCommand? = nil
    @State private var findResult: EditorFindResult = EditorFindResult(count: 0, current: 0)
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
                        // find bar 打开时必须 readonly: ProseMirror 持续监听 DOM mutation,
                        // 见到 .panel-find-mark span 不在 schema 里就异步 reconcile 把它 unwrap,
                        // 后续 next/prev 调用时 marks 已空 → 立刻 '无匹配'。readonly 模式
                        // 下 ProseMirror 不 reconcile, mark 才能保留。
                        // user '找到位置后想编辑' 的需求由 esc 关闭后自然解锁 readOnly 满足,
                        // cursor 通过 commitFindCursor 同步到 cmd+F 跳到的位置, 关闭 find bar
                        // 后用户在该位置可继续编辑。
                        readOnly: showFindBar,
                        findCommand: $findCommand,
                        findResult: $findResult
                    )
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .animation(.easeInOut(duration: 0.18), value: focusForCard?.text)
                .padding(.vertical, 12)
                .frame(minWidth: 520)
                .navigationTitle(title)
                .transition(.opacity)
            } else if state.contentMode == .micro {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                    .transition(.opacity)
            } else {
                EmptyState(kind: detailEmptyKind, selectedName: state.selectedName)
                    .transition(.opacity)
            }
        }
        // 内容和空状态都撑满 detail 列。
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .animation(.easeInOut(duration: 0.25), value: hasContent)
        .animation(.easeInOut(duration: 0.25), value: detailEmptyKind)
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
            state.performPageZoom(.zoomOut)
        } label: {
            Image(systemName: "textformat.size.smaller")
        }
        .help("缩小页面（⌘－）")
        .disabled(!state.canZoomOut)
    }

    private var fontLargerButton: some View {
        Button {
            state.performPageZoom(.zoomIn)
        } label: {
            Image(systemName: "textformat.size.larger")
        }
        .help("放大页面（⌘＋）")
        .disabled(!state.canZoomIn)
    }

    /// 顶部 find bar — cmd+F 显示, ESC 关闭。输入即时搜索, 回车跳下一个, 显示 X / Y 匹配数。
    /// 期间 editor 自动 readonly, 防 ProseMirror 重建破坏 marker。
    @ViewBuilder
    private var findBar: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("搜索 (回车下一个 / Shift+回车上一个)", text: $findText)
                .textFieldStyle(.roundedBorder)
                .focused($findFieldFocused)
                .onSubmit { issueNext() }
                .onChange(of: findText) { _, new in
                    let trimmed = new.trimmingCharacters(in: .whitespaces)
                    if trimmed.isEmpty {
                        findCommand = EditorFindCommand(id: UUID(), kind: .clear)
                        findResult = EditorFindResult(count: 0, current: 0)
                    } else {
                        findCommand = EditorFindCommand(id: UUID(), kind: .search(trimmed))
                    }
                }
            Text(matchCountText)
                .font(.callout.monospacedDigit())
                .foregroundStyle(matchCountColor)
                .frame(minWidth: 64, alignment: .trailing)
            Button {
                issuePrev()
            } label: { Image(systemName: "chevron.up") }
            .buttonStyle(.borderless)
            .help("上一个 (⇧⌘G / ⇧↩)")
            .disabled(findResult.count == 0)
            Button {
                issueNext()
            } label: { Image(systemName: "chevron.down") }
            .buttonStyle(.borderless)
            .help("下一个 (⌘G / ↩)")
            .disabled(findResult.count == 0)
            Button {
                closeFindBar()
            } label: { Image(systemName: "xmark.circle.fill") }
            .buttonStyle(.plain)
            .foregroundStyle(.secondary)
            .help("关闭 (Esc)")
            // .cancelAction 把 button 绑到 ESC 键。在 macOS SwiftUI 下, cancelAction
            // 优先级高于 TextField 自己对 ESC 的处理 (TextField 默认 ESC 失焦), 即使
            // findFieldFocused=true 也能触发, 修原来 .onExitCommand 不触发的 bug。
            .keyboardShortcut(.cancelAction)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 6))
        .padding(.horizontal, 12)
    }

    private var matchCountText: String {
        if findText.trimmingCharacters(in: .whitespaces).isEmpty { return "" }
        if findResult.count == 0 { return "无匹配" }
        return "\(findResult.current) / \(findResult.count)"
    }

    private var matchCountColor: Color {
        if findText.trimmingCharacters(in: .whitespaces).isEmpty { return .secondary }
        return findResult.count == 0 ? .red : .secondary
    }

    private func issueNext() {
        guard findResult.count > 0 else { return }
        findCommand = EditorFindCommand(id: UUID(), kind: .next)
    }

    private func issuePrev() {
        guard findResult.count > 0 else { return }
        findCommand = EditorFindCommand(id: UUID(), kind: .prev)
    }

    private func closeFindBar() {
        showFindBar = false
        findText = ""
        findCommand = EditorFindCommand(id: UUID(), kind: .clear)
        findResult = EditorFindResult(count: 0, current: 0)
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
            issueNext()
        }
        .keyboardShortcut("g", modifiers: .command)
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
        .disabled(!showFindBar || findResult.count == 0)
    }

    /// shift+cmd+G 上一个。
    private var findPrevShortcut: some View {
        Button("") {
            issuePrev()
        }
        .keyboardShortcut("g", modifiers: [.command, .shift])
        .frame(width: 0, height: 0)
        .opacity(0)
        .accessibilityHidden(true)
        .disabled(!showFindBar || findResult.count == 0)
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
            breadcrumbs: first.parentChain,
            group: nil
        )
    }
}

/// 右侧 detail 列在没 content (state.detail == nil 且 contentMode == .todolist) 时
/// 三态语义:
///   - loading:        loadAll 还在 / selectedName 已设但 detail 还没拿回来
///   - failed:         loadAll 已结束但 detail 还是 nil (网络失败 / 名字不存在)
///   - nothingSelected: 用户没选中任何 todolist
/// 之前老逻辑只看 selectedName == nil 区分 "未选中" / "加载失败", 启动初 selectedName
/// 已 set 但 detail 仍 nil 的几百 ms 间隙误显示 "加载失败"。
enum DetailEmptyKind: Hashable {
    case loading
    case failed
    case nothingSelected
}

extension DetailView {
    var detailEmptyKind: DetailEmptyKind {
        if state.isLoading {
            return .loading
        }
        if state.selectedName != nil && state.detail == nil {
            // loadAll 已结束 (isLoading=false) 但 detail 仍 nil → 加载失败 (lastError 一般非空)
            return .failed
        }
        return .nothingSelected
    }
}

private struct EmptyState: View {
    let kind: DetailEmptyKind
    let selectedName: String?

    var body: some View {
        switch kind {
        case .loading:
            ProgressView("加载中…")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        case .failed:
            ContentUnavailableView {
                Label("todolist 加载失败", systemImage: "exclamationmark.triangle")
            } description: {
                Text(selectedName ?? "")
            }
        case .nothingSelected:
            ContentUnavailableView {
                Label("未选中 todolist", systemImage: "list.bullet.rectangle")
            } description: {
                Text("在左侧选一个开始")
            }
        }
    }
}
