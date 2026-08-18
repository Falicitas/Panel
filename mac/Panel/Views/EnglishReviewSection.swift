import SwiftUI

/// 精听复习区. 按 audio 分组 + DisclosureGroup 默认展开, 顶部显示「今日已完成 / 配额」.
///
/// 显示在 EnglishView 的 windowsSection 之后 / audioSection 之前 — 用户进英文 tab
/// 第一眼看复习, 跟 Anki 类 SRS 工具的 UX 一致 (先复习再学新).
///
/// 复习完的条目不收起来 — 常驻队列前缀 (灰掉打勾), 配额调小也不赶走它们。
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
            "标记剩余 \(review.pendingShown) 段复习完成?", isPresented: $showDoneAllConfirm
        ) {
            Button("全部完成") {
                Task { await state.reviewMarkAllDone() }
            }
            .keyboardShortcut(.defaultAction)
            Button("取消", role: .cancel) {}
        } message: {
            Text("这 \(review.pendingShown) 段都会推进到下一复习阶段.")
        }
    }

    @ViewBuilder
    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("复习队列")
                .font(.headline)
            Text("(\(review.doneToday) / \(review.dailyLimit))")
                .font(.body.monospacedDigit())
                .foregroundStyle(review.doneToday >= review.dailyLimit ? .green : .secondary)
                .help("今日已完成 \(review.doneToday) 条 · 配额 \(review.dailyLimit) 条")
            Spacer()
            if review.pendingShown == 0 && review.doneToday > 0 {
                Text("今日配额已复习完")
                    .font(.caption)
                    .foregroundStyle(.green)
            }
            if review.deferredTotal > 0 {
                Text("还有 \(review.deferredTotal) 条超出今日配额")
                    .font(.caption)
                    .foregroundStyle(.orange)
                    .help("没有被推到明天, 调大配额就能继续复习")
            }
            if review.pendingShown > 0 {
                Button {
                    showDoneAllConfirm = true
                } label: {
                    Label("全部复习", systemImage: "checkmark.circle.fill")
                        .labelStyle(.titleAndIcon)
                }
                .buttonStyle(.borderedProminent)
                .tint(.green)
                .help("一键标记剩余 \(review.pendingShown) 段复习完成")
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
                let done = group.items.filter(\.doneToday).count
                Text(done > 0 ? "\(done) / \(group.items.count) 段" : "\(group.items.count) 段")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .help(done > 0 ? "本组已复习 \(done) 段" : "待复习 \(group.items.count) 段")
            }
        }
    }
}
