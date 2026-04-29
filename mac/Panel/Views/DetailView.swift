import SwiftUI

struct DetailView: View {
    @Environment(AppState.self) private var state

    var body: some View {
        Group {
            if let detail = state.detail {
                @Bindable var bindable = state
                ScrollViewReader { proxy in
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            if let focus = state.focus, focus.found {
                                FocusCard(focus: focus)
                            }
                            MilkDownEditor(
                                text: $bindable.editorBuffer,
                                onBlur: { Task { await state.saveDetail() } }
                            )
                            .frame(minHeight: 480)
                            .padding(.horizontal)
                            .id("md-content")
                        }
                        .padding(.vertical, 12)
                    }
                    .onChange(of: state.pendingScrollLine) { _, newValue in
                        guard let line = newValue else { return }
                        Task { @MainActor in
                            try? await Task.sleep(for: .milliseconds(800))
                            // TODO(user): proxy.scrollTo("line-\(line)", anchor: .top)
                            let _ = (proxy, line)
                            state.pendingScrollLine = nil
                        }
                    }
                }
                .frame(minWidth: 520)
                .navigationTitle(detail.name)
                .background(saveShortcut)
                .toolbar {
                    ToolbarItem { anchorButton }
                }
            } else {
                EmptyState(selectedName: state.selectedName)
            }
        }
    }

    private var saveShortcut: some View {
        Button("") { Task { await state.saveDetail() } }
            .keyboardShortcut("s", modifiers: .command)
            .frame(width: 0, height: 0)
            .opacity(0)
            .accessibilityHidden(true)
    }

    @ViewBuilder
    private var anchorButton: some View {
        let selected = state.selectedName
        let isAnchored = selected != nil && selected == state.anchor
        Button {
            guard let name = selected else { return }
            Task { await state.setAnchor(name: isAnchored ? nil : name) }
        } label: {
            Image(systemName: isAnchored ? "pin.fill" : "pin")
        }
        .help(isAnchored ? "取消锚定" : "锚定为主 todolist")
        .tint(isAnchored ? Color.anchorAccent : Color.secondary)
        .disabled(selected == nil)
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
