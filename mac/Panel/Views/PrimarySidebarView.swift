import SwiftUI

struct PrimarySidebarView: View {
    @Binding var selected: AppTab

    /// 拒绝把 selection 切到 disabled tab — 用户点 disabled 行时 binding setter 拒收,
    /// selected 保持原值, 视觉上 row 灰显且不会被高亮。
    private var safeSelected: Binding<AppTab> {
        Binding(
            get: { selected },
            set: { newValue in
                if newValue.isEnabled { selected = newValue }
            }
        )
    }

    var body: some View {
        VStack(spacing: 0) {
            List(selection: safeSelected) {
                ForEach(AppTab.allCases) { tab in
                    Label(tab.label, systemImage: tab.icon)
                        .tag(tab)
                        .foregroundStyle(tab.isEnabled ? .primary : .tertiary)
                        .help(tab.isEnabled ? "" : tab.comingSoonHint)
                }
            }
            .listStyle(.sidebar)
            .navigationTitle("Panel")
            StatusFooter()
        }
        .navigationSplitViewColumnWidth(min: 160, ideal: 180, max: 240)
    }
}
