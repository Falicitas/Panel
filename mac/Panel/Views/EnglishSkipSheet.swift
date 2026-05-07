import SwiftUI

/// 跳过模态框 — 选下一个音频。两种触发场景:
///   1. 用户主动点 audio 卡右上的"跳过"按钮
///   2. 当前 audio 所有 segment 全勾完, AppState.toggleEnglishSegment 检测到自动调起
/// 两种走完全相同流程: 选 topic → 抽一个 → 不喜欢"再抽" → "确认"切过去。
/// "取消" 不归档当前 audio。
struct EnglishSkipSheet: View {
    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss

    /// Picker 候选 (tag = topic 字符串值; "" = 不过滤)。
    /// 跟 backend 的 free-text topic 字段对齐 (backend 不做 enum 校验)。
    private static let topicOptions: [(label: String, value: String)] = [
        ("全部", ""),
        ("金融", "finance"),
        ("科技", "tech"),
        ("时事", "current_affairs"),
        ("自然科学", "science"),
        ("综合", "general"),
    ]

    var body: some View {
        @Bindable var state = state
        VStack(spacing: 0) {
            header
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    topicPicker(state: state)
                    candidateCard
                }
                .padding(20)
            }
        }
        .frame(minWidth: 480, minHeight: 420)
    }

    @ViewBuilder
    private var header: some View {
        HStack {
            Text("选择下一个音频")
                .font(.headline)
            Spacer()
            Button("取消") {
                state.cancelSkipSheet()
                dismiss()
            }
            Button("确认") {
                guard let id = state.skipSheetCandidate?.audio?.id else { return }
                Task {
                    await state.confirmSkipToAudio(selectedAudioID: id)
                    dismiss()
                }
            }
            .buttonStyle(.borderedProminent)
            .keyboardShortcut(.defaultAction)
            .disabled(state.skipSheetCandidate?.audio == nil || state.skipSheetIsLoading)
        }
        .padding()
    }

    @ViewBuilder
    private func topicPicker(state: AppState) -> some View {
        HStack {
            Text("主题")
                .frame(width: 60, alignment: .leading)
                .foregroundStyle(.secondary)
            Picker("", selection: Binding(
                get: { state.skipSheetTopic },
                set: { newValue in
                    Task { await state.setSkipSheetTopic(newValue) }
                }
            )) {
                ForEach(Self.topicOptions, id: \.value) { opt in
                    Text(opt.label).tag(opt.value)
                }
            }
            .labelsHidden()
            .frame(maxWidth: 220)
            Spacer()
        }
    }

    @ViewBuilder
    private var candidateCard: some View {
        if state.skipSheetIsLoading && state.skipSheetCandidate == nil {
            HStack {
                ProgressView()
                Text("抽取中…").foregroundStyle(.secondary)
                Spacer()
            }
            .padding(.vertical, 24)
        } else if let cand = state.skipSheetCandidate, let audio = cand.audio {
            VStack(alignment: .leading, spacing: 10) {
                Text(audio.title)
                    .font(.title3.weight(.semibold))
                    .lineLimit(3)
                HStack(spacing: 12) {
                    Text("[\(audio.source.uppercased())]")
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                    Text("时长 \(formatDur(audio.durationMs))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    if !audio.topic.isEmpty {
                        Text(topicLabel(audio.topic))
                            .font(.caption.weight(.medium))
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.accentColor.opacity(0.15), in: Capsule())
                    }
                    Spacer()
                }
                if !cand.descriptionPreview.isEmpty {
                    Text(cand.descriptionPreview)
                        .font(.body)
                        .foregroundStyle(.primary.opacity(0.85))
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, 4)
                }
                HStack {
                    Spacer()
                    Button {
                        Task { await state.rerollSkipCandidate() }
                    } label: {
                        Label("再抽一个", systemImage: "shuffle")
                    }
                    .buttonStyle(.bordered)
                    .disabled(state.skipSheetIsLoading)
                }
                .padding(.top, 4)
            }
            .padding(16)
            .background(
                RoundedRectangle(cornerRadius: 10)
                    .fill(Color.gray.opacity(0.08))
            )
        } else {
            // candidate=nil 表示已抽完所有候选 / fetch 失败
            VStack(alignment: .leading, spacing: 8) {
                Label("暂无候选", systemImage: "tray")
                    .font(.headline)
                    .foregroundStyle(.secondary)
                Text("所选主题下没有更多未归档音频。换个主题或等 fetcher 抓更多 (~6 小时一轮)。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: 10)
                    .fill(Color.gray.opacity(0.05))
            )
        }
    }

    private func formatDur(_ ms: Int64) -> String {
        let sec = max(0, Int(ms / 1000))
        let m = sec / 60
        let s = sec % 60
        return String(format: "%d:%02d", m, s)
    }

    private func topicLabel(_ value: String) -> String {
        Self.topicOptions.first(where: { $0.value == value })?.label ?? value
    }
}
