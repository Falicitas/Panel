import SwiftUI

struct StatusFooter: View {
    @Environment(AppState.self) private var state

    var body: some View {
        HStack(spacing: 8) {
            indicator
            Text(state.modeLabel)
                .font(.caption.weight(.medium))
            if let wake = state.dailyState?.wakeTime, !wake.isEmpty {
                Text("· wake \(wake)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
            Spacer()
            Menu {
                Button("普通模式") { Task { await state.setMode("normal") } }
                Button("今天外出") { Task { await state.setMode("outside") } }
                if state.effectiveMode == "sleep" {
                    Divider()
                    Text("睡眠模式由 \(state.stateSettings?.sleepStart ?? "22:30") 强制触发,无法手动覆盖")
                }
                if state.effectiveMode == "focus" && state.dailyState?.mode != "outside" {
                    Divider()
                    Text("\(state.stateSettings?.focusStart ?? "17:00") 起自动专注;切外出可跳过专注")
                }
            } label: {
                Image(systemName: "ellipsis.circle")
                    .foregroundStyle(.tertiary)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity)
        .background(stripeBackground)
        .overlay(alignment: .top) { Divider() }
    }

    @ViewBuilder
    private var indicator: some View {
        if let c = state.themeColor {
            Circle().fill(c).frame(width: 10, height: 10)
        } else {
            Circle().fill(.secondary).frame(width: 8, height: 8)
        }
    }

    @ViewBuilder
    private var stripeBackground: some View {
        if let c = state.themeColor {
            c.opacity(0.18)
        } else {
            Color.clear
        }
    }
}
