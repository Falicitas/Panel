import SwiftUI

struct EditorView: View {
    @Binding var text: String
    let onSave: () -> Void
    let onCancel: () -> Void

    var body: some View {
        VStack(spacing: 8) {
            HStack {
                Text("编辑模式 · text only")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("取消", action: onCancel)
                    .keyboardShortcut(.cancelAction)
                Button("保存", action: onSave)
                    .keyboardShortcut("s", modifiers: .command)
                    .buttonStyle(.borderedProminent)
            }
            .padding(.horizontal)
            TextEditor(text: $text)
                .font(.system(.body, design: .monospaced))
                .frame(minHeight: 420)
                .padding(.horizontal)
        }
    }
}
