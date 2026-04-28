import SwiftUI

struct MicroDetailView: View {
    var body: some View {
        ContentUnavailableView {
            Label("当日 Micro 详情", systemImage: "calendar.badge.clock")
        } description: {
            Text("Phase 3.B 接入")
        }
    }
}
