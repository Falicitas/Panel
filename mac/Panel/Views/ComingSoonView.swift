import SwiftUI

struct ComingSoonView: View {
    let tab: AppTab

    var body: some View {
        ContentUnavailableView {
            Label(tab.label, systemImage: tab.icon)
        } description: {
            Text(tab.comingSoonHint.isEmpty ? "敬请期待" : "敬请期待 — \(tab.comingSoonHint)")
        }
    }
}
