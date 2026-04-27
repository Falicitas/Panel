import SwiftUI

struct FocusCard: View {
    let focus: FocusResult

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let crumbs = focus.breadcrumbs, !crumbs.isEmpty {
                HStack(spacing: 4) {
                    ForEach(Array(crumbs.enumerated()), id: \.offset) { idx, crumb in
                        if idx > 0 {
                            Image(systemName: "chevron.right")
                                .font(.caption2)
                                .foregroundStyle(.tertiary)
                        }
                        Text(crumb)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Text(focus.text ?? "")
                .font(.title3.weight(.medium))
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
            if let n = focus.lineNo {
                Text("L\(n)")
                    .font(.caption2.monospacedDigit())
                    .foregroundStyle(.tertiary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(Color.accentColor.opacity(0.08))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(Color.accentColor.opacity(0.3), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .padding(.horizontal)
    }
}
