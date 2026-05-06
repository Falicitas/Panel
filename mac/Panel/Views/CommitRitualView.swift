import SwiftUI

/// 双阶段确认仪式蒙版 — 全 panel 覆盖, 强制用户做两次有意识的 commitment。
/// 阶段 1: 立刻可点的按钮 → 进阶段 2
/// 阶段 2: countdownSec 秒倒计时, 结束后按钮才可点 → 写 today commit key 到 UserDefaults
///        + onComplete 触发 SwiftUI 重画移除蒙版。
///
/// 触发逻辑由 ContentView 决定 (focusStart / 22:00 / today key 是否已 set);
/// 本 view 只关心 stage 状态和 commit 写入。
///
/// 视觉跟 WakeMaskView 的 BigButton 一致 (220x160 方形 + icon + 主+副标题), 渐变全屏。
struct CommitRitualConfig {
    let title: String
    let stage1: Stage
    let stage2: Stage
    let countdownSec: Int
    /// UserDefaults key 前缀, 实际 key = "{commitKey}-{yyyy-MM-dd}"。
    /// 跨日自然失效 (新日期 key 不存在 → 蒙版重弹)。
    let commitKey: String

    /// 单 stage 配置: body (中部说明), buttonTitle (按钮主标), buttonSubtitle (副标), systemImage (icon)。
    struct Stage {
        let body: String
        let buttonTitle: String
        let buttonSubtitle: String
        let systemImage: String
    }
}

extension CommitRitualConfig {
    static let focus = CommitRitualConfig(
        title: "进入专注模式",
        stage1: .init(
            body: "关闭社交、即时通讯,防止打扰、防止引来打扰",
            buttonTitle: "我准备好",
            buttonSubtitle: "现在去关闭社交",
            systemImage: "sparkles"
        ),
        stage2: .init(
            body: "确认所有社交已关闭?",
            buttonTitle: "确认关闭社交",
            buttonSubtitle: "5 秒后才可点",
            systemImage: "checkmark.shield"
        ),
        countdownSec: 5,
        commitKey: "focusRitualCommit"
    )

    static let phone = CommitRitualConfig(
        title: "收起白色手机",
        stage1: .init(
            body: "把白色 iPhone 14 拿到那个晚上不会再碰的位置",
            buttonTitle: "我去拿了",
            buttonSubtitle: "起身去放手机",
            systemImage: "iphone.slash"
        ),
        stage2: .init(
            body: "已经放到那个地方了吗?",
            buttonTitle: "确认放好",
            buttonSubtitle: "5 秒后才可点",
            systemImage: "checkmark.shield"
        ),
        countdownSec: 5,
        commitKey: "phoneRitualCommit"
    )
}

struct CommitRitualView: View {
    let config: CommitRitualConfig
    let onComplete: () -> Void

    @State private var stage: Int = 1
    @State private var countdown: Int = 0
    private let timer = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        ZStack {
            LinearGradient(
                colors: [Color(red: 0.10, green: 0.10, blue: 0.18), .black],
                startPoint: .top,
                endPoint: .bottom
            )
            .ignoresSafeArea()

            VStack(spacing: 36) {
                Text(config.title)
                    .font(.system(size: 48, weight: .light))

                Text(currentStage.body)
                    .font(.title2)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.white.opacity(0.9))
                    .frame(maxWidth: 600)
                    .fixedSize(horizontal: false, vertical: true)

                RitualButton(
                    title: currentStage.buttonTitle,
                    subtitle: subtitleLabel,
                    systemImage: currentStage.systemImage,
                    enabled: buttonReady,
                    action: handleButton
                )
                .padding(.top, 8)
            }
            .foregroundStyle(.white)
            .padding(40)
        }
        .onReceive(timer) { _ in
            if stage == 2 && countdown > 0 {
                countdown -= 1
            }
        }
    }

    private var currentStage: CommitRitualConfig.Stage {
        stage == 1 ? config.stage1 : config.stage2
    }

    private var buttonReady: Bool {
        stage == 1 || countdown == 0
    }

    /// 倒计时阶段把剩余秒数显示在 subtitle, 结束后回到原 subtitle。stage 1 用原 subtitle。
    private var subtitleLabel: String {
        if stage == 2 && countdown > 0 {
            return "\(countdown) 秒后才可点"
        }
        return currentStage.buttonSubtitle
    }

    private func handleButton() {
        if stage == 1 {
            stage = 2
            countdown = config.countdownSec
            return
        }
        // 阶段 2 完成 → 写今日 commit key, 触发 ContentView shouldShow* 重新计算 → 移除蒙版
        let key = "\(config.commitKey)-\(Self.todayKey())"
        UserDefaults.standard.set(true, forKey: key)
        onComplete()
    }

    /// 跨日 key, 用本地时区的 yyyy-MM-dd。次日日期变 → key 不存在 → 蒙版重弹。
    static func todayKey() -> String {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: Date())
    }

    /// 给 ContentView 的 shouldShow 逻辑用 — 当日 commit key 是否已 set。
    static func isCommitted(_ commitKey: String) -> Bool {
        UserDefaults.standard.bool(forKey: "\(commitKey)-\(todayKey())")
    }
}

/// 跟 WakeMaskView.BigButton 同款 — 220x160 方形, icon + 主标 + 副标 + 玻璃描边。
/// 区别: 加 enabled 状态 (false 时整体淡化 0.4, disabled hit testing)。
private struct RitualButton: View {
    let title: String
    let subtitle: String
    let systemImage: String
    let enabled: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(spacing: 8) {
                Image(systemName: systemImage)
                    .font(.system(size: 36, weight: .light))
                Text(title)
                    .font(.title3.weight(.medium))
                Text(subtitle)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .frame(width: 220, height: 160)
            .padding()
            .background(.white.opacity(0.08), in: RoundedRectangle(cornerRadius: 16))
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(.white.opacity(0.2), lineWidth: 1)
            )
            .opacity(enabled ? 1 : 0.4)
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
    }
}
