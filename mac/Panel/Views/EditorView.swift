import SwiftUI

struct EditorView: View {
    @Binding var text: String
    let onSave: () -> Void
    let onCancel: () -> Void

    var body: some View {
        VStack(spacing: 8) {
            HStack {
                Text("编辑模式 · WYSIWYG (MilkDown)")
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
            MilkDownEditor(text: $text)
                .frame(minHeight: 480)
                .padding(.horizontal)
        }
    }
}
