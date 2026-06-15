import SwiftUI

/// 精听复习区. 按 audio 分组 + DisclosureGroup 默认展开, 顶部显示「shown / limit」.
///
/// 显示在 EnglishView 的 windowsSection 之后 / audioSection 之前 — 用户进英文 tab
/// 第一眼看复习, 跟 Anki 类 SRS 工具的 UX 一致 (先复习再学新).
///
/// 空态: 「今日没有到期复习, 继续学新内容 ↓」, 不挡 audioSection。
struct EnglishReviewSection: View {
    let review: EnglishReviewToday

    @Environment(AppState.self) private var state
    @State private var showDoneAllConfirm = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            if review.groups.isEmpty {
                emptyState
            } else {
                ForEach(review.groups, id: \.audio.id) { group in
                    audioGroup(group)
                }
            }
        }
        .confirmationDialog(
            "标记今日全部 \(review.shownTotal) 段复习完成?", isPresented: $showDoneAllConfirm
        ) {
            Button("全部完成") {
                Task { await state.reviewMarkAllDone() }
            }
            .keyboardShortcut(.defaultAction)
            Button("取消", role: .cancel) {}
        } message: {
            Text("这 \(review.shownTotal) 段都会推进到下一复习阶段.")
        }
    }

    @ViewBuilder
    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("复习队列")
                .font(.headline)
            Text("(\(review.shownTotal) / \(review.dailyLimit))")
                .font(.body.monospacedDigit())
                .foregroundStyle(.secondary)
            Spacer()
            if review.postponedTotal > 0 {
                Text("\(review.postponedTotal) 条因配额已延到明天")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
            if !review.groups.isEmpty {
                Button {
                    showDoneAllConfirm = true
                } label: {
                    Label("全部复习", systemImage: "checkmark.circle.fill")
                        .labelStyle(.titleAndIcon)
                }
                .buttonStyle(.borderedProminent)
                .tint(.green)
                .help("一键标记今日全部 \(review.shownTotal) 段复习完成")
            }
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        HStack {
            Image(systemName: "checkmark.seal")
                .foregroundStyle(.green)
            Text("今日没有到期复习, 继续学新内容 ↓")
                .foregroundStyle(.secondary)
        }
        .padding(.vertical, 6)
    }

    @ViewBuilder
    private func audioGroup(_ group: EnglishReviewGroup) -> some View {
        DisclosureGroup {
            VStack(spacing: 0) {
                ForEach(group.items) { item in
                    EnglishReviewRow(audio: group.audio, item: item)
                    if item.id != group.items.last?.id {
                        Divider().padding(.leading, 36 + 12 + 24)
                    }
                }
            }
        } label: {
            HStack(alignment: .firstTextBaseline) {
                Text("[\(group.audio.source.uppercased())]")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                Text(group.audio.title)
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                if group.audio.archivedAt != nil {
                    Text("(已归档)").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Text("\(group.items.count) 段")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
        }
    }
}
