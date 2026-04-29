import SwiftUI

struct FocusCard: View {
    @Environment(AppState.self) private var state
    let focus: FocusResult

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if let crumbs = focus.breadcrumbs, !crumbs.isEmpty {
                HStack(spacing: 6) {
                    ForEach(Array(crumbs.enumerated()), id: \.offset) { idx, crumb in
                        if idx > 0 {
                            Image(systemName: "chevron.right")
                                .font(.body)
                                .foregroundStyle(.tertiary)
                        }
                        Text(crumb)
                            .font(.title3)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Text(focus.text ?? "")
                .font(.title.weight(.semibold))
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
            if let n = focus.lineNo {
                Button {
                    Task { await state.jumpToFocusLine(n) }
                } label: {
                    Text("L\(n)")
                        .font(.body.monospacedDigit())
                        .underline()
                }
                .buttonStyle(.plain)
                .foregroundStyle(Color.anchorAccent)
                .help("跳到锚定 todolist 第 \(n) 行")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(20)
        .background(Color.anchorAccent.opacity(0.08))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(Color.anchorAccent.opacity(0.3), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal)
    }
}
