import SwiftUI

struct NotificationBar: View {
    @Environment(AppState.self) private var state
    @State private var showPopover = false

    var body: some View {
        let count = state.notifications.count
        HStack(spacing: 10) {
            Group {
                if count > 0 {
                    Image(systemName: "bell.badge.fill").foregroundStyle(Color.accentColor)
                } else {
                    Image(systemName: "bell").foregroundStyle(.tertiary)
                }
            }
            .imageScale(.large)
            if let latest = state.notifications.first {
                Text(latest.title + (latest.body.isEmpty ? "" : " · \(latest.body)"))
                    .font(.body)
                    .lineLimit(1)
            } else {
                Text("无新通知 — 食物保健品到点提醒在此")
                    .font(.body)
                    .foregroundStyle(.tertiary)
            }
            Spacer()
            if count > 0 {
                Button {
                    showPopover.toggle()
                } label: {
                    HStack(spacing: 6) {
                        Text("\(count)")
                            .font(.body.weight(.bold).monospacedDigit())
                            .padding(.horizontal, 8)
                            .padding(.vertical, 2)
                            .background(.red.opacity(0.85), in: Capsule())
                            .foregroundStyle(.white)
                        Image(systemName: "chevron.down")
                            .font(.body)
                            .foregroundStyle(.secondary)
                    }
                }
                .buttonStyle(.borderless)
                .popover(isPresented: $showPopover, arrowEdge: .top) {
                    NotificationListPopover(dismissPopover: { showPopover = false })
                        .environment(state)
                }
            }
        }
        .padding(.horizontal, 14)
        .frame(height: 40)
        .frame(maxWidth: .infinity)
        .background(barBackground)
        .overlay(alignment: .bottom) { Divider() }
    }

    @ViewBuilder
    private var barBackground: some View {
        if let c = state.themeColor {
            c.opacity(0.20)
        } else {
            Rectangle().fill(.bar)
        }
    }
}

private struct NotificationListPopover: View {
    @Environment(AppState.self) private var state
    let dismissPopover: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("通知 (\(state.notifications.count))")
                    .font(.title2)
                Spacer()
                Button {
                    Task { await dismissAll() }
                } label: {
                    Text("全部清空").font(.body)
                }
                .buttonStyle(.borderless)
                .disabled(state.notifications.isEmpty)
            }
            .padding()
            Divider()
            if state.notifications.isEmpty {
                ContentUnavailableView("无新通知", systemImage: "bell.slash")
                    .frame(width: 480, height: 240)
            } else {
                List {
                    ForEach(state.notifications) { n in
                        NotifRow(
                            n: n,
                            onDismiss: { Task { await state.dismiss(n) } },
                            onJump: { handleJump(n) }
                        )
                    }
                }
                .listStyle(.inset)
                .frame(
                    width: 480,
                    height: min(CGFloat(state.notifications.count) * 80 + 32, 540)
                )
            }
        }
    }

    private func handleJump(_ n: PanelNotification) {
        switch n.source {
        case "supplement":
            guard let id = parseSupplementID(n.sourceRef) else { return }
            state.jumpToSupplement(id: id)
            dismissPopover()
        case "english":
            state.jumpToEnglish()
            dismissPopover()
        default:
            return
        }
    }

    /// source_ref 形如 "supplement:{id}:window:{idx}:date:{date}"
    private func parseSupplementID(_ ref: String) -> Int64? {
        let parts = ref.split(separator: ":")
        guard parts.count >= 2, parts[0] == "supplement", let id = Int64(parts[1]) else { return nil }
        return id
    }

    private func dismissAll() async {
        for n in state.notifications {
            await state.dismiss(n)
        }
    }
}

private struct NotifRow: View {
    let n: PanelNotification
    let onDismiss: () -> Void
    let onJump: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: levelIcon)
                .foregroundStyle(levelColor)
                .imageScale(.large)
            VStack(alignment: .leading, spacing: 4) {
                Text(n.title).font(.title3.weight(.medium))
                if !n.body.isEmpty {
                    Text(n.body).font(.body).foregroundStyle(.secondary)
                }
                Text(formattedTime)
                    .font(.callout.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
            Spacer(minLength: 4)
            Button(action: onDismiss) {
                Image(systemName: "xmark.circle")
                    .foregroundStyle(.secondary)
                    .imageScale(.large)
            }
            .buttonStyle(.borderless)
            .help("dismiss")
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
        .onTapGesture { onJump() }
    }

    private var levelIcon: String {
        switch n.level {
        case "alert": "exclamationmark.triangle.fill"
        case "warn":  "exclamationmark.circle"
        default:      "info.circle"
        }
    }
    private var levelColor: Color {
        switch n.level {
        case "alert": .red
        case "warn":  .orange
        default:      .accentColor
        }
    }
    private var formattedTime: String {
        let d = Date(timeIntervalSince1970: TimeInterval(n.createdAt))
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: d)
    }
}
