import SwiftUI

struct MicroContentView: View {
    var body: some View {
        ContentUnavailableView {
            Label("Micro Todolist", systemImage: "calendar")
        } description: {
            Text("当日临时事务 — Phase 3.B 接入")
        }
    }
}
