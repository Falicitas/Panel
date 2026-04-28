import SwiftUI

struct NotificationBar: View {
    @Environment(AppState.self) private var state
    @State private var showPopover = false

    var body: some View {
        let count = state.notifications.count
        HStack(spacing: 8) {
            Group {
                if count > 0 {
                    Image(systemName: "bell.badge.fill").foregroundStyle(Color.accentColor)
                } else {
                    Image(systemName: "bell").foregroundStyle(.tertiary)
                }
            }
            if let latest = state.notifications.first {
                Text(latest.title + (latest.body.isEmpty ? "" : " · \(latest.body)"))
                    .font(.caption)
                    .lineLimit(1)
            } else {
                Text("无新通知 — 食物保健品到点提醒在此")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
            Spacer()
            if count > 0 {
                Button {
                    showPopover.toggle()
                } label: {
                    HStack(spacing: 4) {
                        Text("\(count)")
                            .font(.caption2.weight(.bold).monospacedDigit())
                            .padding(.horizontal, 6)
                            .padding(.vertical, 1)
                            .background(.red.opacity(0.85), in: Capsule())
                            .foregroundStyle(.white)
                        Image(systemName: "chevron.down")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                .buttonStyle(.borderless)
                .popover(isPresented: $showPopover, arrowEdge: .top) {
                    NotificationListPopover().environment(state)
                }
            }
        }
        .padding(.horizontal, 12)
        .frame(height: 28)
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

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("通知 (\(state.notifications.count))")
                    .font(.headline)
                Spacer()
                Button {
                    Task { await dismissAll() }
                } label: {
                    Text("全部清空").font(.caption)
                }
                .buttonStyle(.borderless)
                .disabled(state.notifications.isEmpty)
            }
            .padding()
            Divider()
            if state.notifications.isEmpty {
                ContentUnavailableView("无新通知", systemImage: "bell.slash")
                    .frame(width: 380, height: 200)
            } else {
                List {
                    ForEach(state.notifications) { n in
                        NotifRow(n: n) {
                            Task { await state.dismiss(n) }
                        }
                    }
                }
                .listStyle(.inset)
                .frame(
                    width: 380,
                    height: min(CGFloat(state.notifications.count) * 56 + 20, 420)
                )
            }
        }
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

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: levelIcon)
                .foregroundStyle(levelColor)
            VStack(alignment: .leading, spacing: 2) {
                Text(n.title).font(.body.weight(.medium))
                if !n.body.isEmpty {
                    Text(n.body).font(.caption).foregroundStyle(.secondary)
                }
                Text(formattedTime)
                    .font(.caption2.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
            Spacer()
            Button(action: onDismiss) {
                Image(systemName: "xmark.circle")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("dismiss")
        }
        .padding(.vertical, 2)
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
