import SwiftUI

struct PrimarySidebarView: View {
    @Binding var selected: AppTab

    var body: some View {
        VStack(spacing: 0) {
            List(selection: $selected) {
                ForEach(AppTab.allCases) { tab in
                    Label(tab.label, systemImage: tab.icon)
                        .tag(tab)
                        .foregroundStyle(tab.isEnabled ? .primary : .tertiary)
                }
            }
            .listStyle(.sidebar)
            .navigationTitle("Panel")
            StatusFooter()
        }
        .navigationSplitViewColumnWidth(min: 160, ideal: 180, max: 240)
    }
}
