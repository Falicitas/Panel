import SwiftUI

/// 「问题星级评估」浮窗。固定参考文本,只读 MilkDown 渲染。
/// 调位置 / 尺寸 / 字号在下面 4 个常量。
struct RatingReferencePanel: View {
    /// 距底部 (pt)。
    static let bottomInset: CGFloat = 40
    /// 距右侧 (pt)。
    static let trailingInset: CGFloat = 40
    /// 浮窗宽度 (pt)。
    static let panelWidth: CGFloat = 850
    /// 浮窗高度 (pt)。
    static let panelHeight: CGFloat = 756
    /// MilkDown pageZoom — 1.0 是默认,每 0.1 = 1 级。1.4 = 默认 +4 级。
    static let panelZoom: Double = 1.4

    let onClose: () -> Void
    @State private var content: String = Self.markdown
    @State private var scrollText: String? = nil

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Image(systemName: "star.fill")
                    .foregroundStyle(Color.anchorAccent)
                Text("问题星级评估")
                    .font(.headline)
                Spacer()
                Button(action: onClose) {
                    Image(systemName: "xmark.circle.fill")
                        .imageScale(.large)
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .help("关闭")
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)

            Divider()

            MilkDownEditor(
                text: $content,
                pageZoom: Self.panelZoom,
                scrollText: $scrollText,
                readOnly: true
            )
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .frame(width: Self.panelWidth, height: Self.panelHeight)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(Color.secondary.opacity(0.3), lineWidth: 1)
        )
        .shadow(color: .black.opacity(0.18), radius: 12, x: 0, y: 4)
        .padding(.bottom, Self.bottomInset)
        .padding(.trailing, Self.trailingInset)
    }

    /// 评估参考文本。Markdown 由 MilkDown 渲染。
    private static let markdown: String = """
    # 🌟 问题星级评估

    - **1 星**：直接上手，5 分钟内完成。做什么、怎么做都清楚。
    - **2 星**：需要拆 2-3 步，但每步都清楚，查一两个东西就够。
    - **3 星**：知道去哪找答案（官方文档、源码），但还没读完 / 理解完。花 10-15 分钟略读完拆成若干 15 分钟的理解子任务，按 2 星节奏推。
    - **4 星**：搜到的东西对不上，缺乏可靠的信息源或问题边界没定义。开始前花 2 分钟写下：验证什么、什么算走得通、什么算走不通的信号。30 分钟闹钟做最小实验，到点停止验证，10 分钟对着条件做判断。方向确认了就看能否降成 3 星继续，没确认就换方向。
      - 需要做：探讨 how — 写下「A 为什么有效/无效、A 之后为什么能接 B」。说不出来 → 写下卡在哪。
      - 余力：边界 — 这解法什么时候会失效？至少想一个反例或边界。
      - 余力：迁移 — 这机制还能解释别的什么我见过的现象？跟旧认知连一条线。

    ## 额外维度微调

    影响线上用户、涉及数据或钱的，往上调一级。
    """
}
