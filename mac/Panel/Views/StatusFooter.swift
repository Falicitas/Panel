import SwiftUI

struct StatusFooter: View {
    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(.secondary)
                .frame(width: 8, height: 8)
            Text("普通")
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity)
        .overlay(alignment: .top) { Divider() }
    }
}
