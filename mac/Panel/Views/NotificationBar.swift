import SwiftUI

struct NotificationBar: View {
    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "bell")
                .foregroundStyle(.tertiary)
            Text("通知占位 — Phase 5 接食物保健品 / 财务 / 记录提醒")
                .font(.caption)
                .foregroundStyle(.tertiary)
            Spacer()
        }
        .padding(.horizontal, 12)
        .frame(height: 28)
        .frame(maxWidth: .infinity)
        .background(.bar)
        .overlay(alignment: .bottom) { Divider() }
    }
}
