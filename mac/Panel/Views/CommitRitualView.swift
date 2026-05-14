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

/// commit key 的「日」如何算:
/// - `.calendar`: 自然日 yyyy-MM-dd, 0 点跨日视为新一日 (key 失效, 蒙版重弹)
/// - `.sleepNight`: 22:00~次日06:00 算同一夜, 用 pre-midnight 日期, 跨 0 点不失效
///   (h < SleepGuard.wakeHour 视为前一日的 sleep night)
enum CommitRitualDayKind: Sendable {
    case calendar
    case sleepNight
}

struct CommitRitualConfig {
    let title: String
    let stage1: Stage
    let stage2: Stage
    let countdownSec: Int
    /// UserDefaults key 前缀, 实际 key = "{commitKey}-{dayKey}"。
    let commitKey: String
    /// 决定 dayKey 怎么算 — 自然日 vs 睡眠夜。default .calendar。
    let dayKind: CommitRitualDayKind

    init(
        title: String,
        stage1: Stage,
        stage2: Stage,
        countdownSec: Int,
        commitKey: String,
        dayKind: CommitRitualDayKind = .calendar
    ) {
        self.title = title
        self.stage1 = stage1
        self.stage2 = stage2
        self.countdownSec = countdownSec
        self.commitKey = commitKey
        self.dayKind = dayKind
    }

    /// 单 stage 配置: body (中部说明), buttonTitle (按钮主标), buttonSubtitle (副标), systemImage (icon)。
    /// bodyItems 非 nil 时, body 充当列表上方的引导句, items 渲染成编号 checklist。
    struct Stage {
        let body: String
        let bodyItems: [String]?
        let buttonTitle: String
        let buttonSubtitle: String
        let systemImage: String

        init(
            body: String,
            bodyItems: [String]? = nil,
            buttonTitle: String,
            buttonSubtitle: String,
            systemImage: String
        ) {
            self.body = body
            self.bodyItems = bodyItems
            self.buttonTitle = buttonTitle
            self.buttonSubtitle = buttonSubtitle
            self.systemImage = systemImage
        }
    }
}

extension CommitRitualConfig {
    static let focus = CommitRitualConfig(
        title: "进入专注模式",
        stage1: .init(
            body: "关闭社交、即时通讯；防止打扰、防止引来打扰",
            buttonTitle: "我准备好",
            buttonSubtitle: "现在去关闭社交",
            systemImage: "sparkles"
        ),
        stage2: .init(
            body: "确认所有社交已关闭？",
            buttonTitle: "确认关闭社交",
            buttonSubtitle: "5 秒后才可点",
            systemImage: "checkmark.shield"
        ),
        countdownSec: 5,
        commitKey: "focusRitualCommit"
    )

    /// items 由 backend `recurring_todos` 表 (kind=phone) 提供, 调用方 (ContentView)
    /// 已过滤好。空数组时调用方不该弹蒙版 — 这里不再做兜底。
    static func phone(items: [String]) -> CommitRitualConfig {
        let n = items.count
        return CommitRitualConfig(
            title: "睡前 \(n) 项",
            stage1: .init(
                body: "全部做完再按",
                bodyItems: items,
                buttonTitle: "都做完了",
                buttonSubtitle: "起身去把这 \(n) 项做完",
                systemImage: "moon.stars"
            ),
            stage2: .init(
                body: "再核对一次",
                bodyItems: items,
                buttonTitle: "确认入睡",
                buttonSubtitle: "5 秒后才可点",
                systemImage: "checkmark.shield"
            ),
            countdownSec: 5,
            commitKey: "phoneRitualCommit",
            dayKind: .sleepNight
        )
    }

    /// items 由 backend `recurring_todos` 表 (kind=morning) 提供, 调用方 (ContentView)
    /// 已过滤好。空数组时调用方不该弹蒙版 — 这里不再做兜底。
    static func morning(items: [String]) -> CommitRitualConfig {
        let n = items.count
        return CommitRitualConfig(
            title: "晨间 \(n) 项",
            stage1: .init(
                body: "起床后这 \(n) 项依次做完",
                bodyItems: items,
                buttonTitle: "都做完了",
                buttonSubtitle: "起身去把这 \(n) 项做完",
                systemImage: "sun.max"
            ),
            stage2: .init(
                body: "再核对一次",
                bodyItems: items,
                buttonTitle: "确认开始一天",
                buttonSubtitle: "5 秒后才可点",
                systemImage: "checkmark.shield"
            ),
            countdownSec: 5,
            commitKey: "morningRitualCommit"
        )
    }
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

                bodySection

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

    @ViewBuilder
    private var bodySection: some View {
        if let items = currentStage.bodyItems {
            VStack(spacing: 18) {
                Text(currentStage.body)
                    .font(.system(size: 15, weight: .regular))
                    .tracking(1)
                    .foregroundStyle(.white.opacity(0.55))
                checklistCard(items: items)
            }
        } else {
            Text(currentStage.body)
                .font(.title2)
                .multilineTextAlignment(.center)
                .foregroundStyle(.white.opacity(0.9))
                .frame(maxWidth: 600)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    /// 玻璃卡片包 5 项 checklist — 与 RitualButton 视觉语言一致 (white opacity bg + 描边)。
    private func checklistCard(items: [String]) -> some View {
        VStack(alignment: .leading, spacing: 18) {
            ForEach(Array(items.enumerated()), id: \.offset) { idx, item in
                HStack(alignment: .firstTextBaseline, spacing: 18) {
                    Text("\(idx + 1)")
                        .font(.system(size: 15, weight: .semibold, design: .rounded))
                        .foregroundStyle(.white.opacity(0.95))
                        .frame(width: 30, height: 30)
                        .background(Circle().fill(.white.opacity(0.14)))
                        .overlay(Circle().stroke(.white.opacity(0.4), lineWidth: 1))
                    Text(item)
                        .font(.system(size: 19, weight: .regular))
                        .foregroundStyle(.white.opacity(0.95))
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .padding(.vertical, 28)
        .padding(.horizontal, 36)
        // 卡片宽度贴最长项 (短词 morning 不会被撑到 580 显空旷, 长句 phone 也照样 fit)。
        // 纵向继续按 SwiftUI 自动算高。
        .fixedSize(horizontal: true, vertical: false)
        .background(.white.opacity(0.06), in: RoundedRectangle(cornerRadius: 20))
        .overlay(
            RoundedRectangle(cornerRadius: 20)
                .stroke(.white.opacity(0.18), lineWidth: 1)
        )
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
        // 阶段 2 完成 → 写当日 commit key, 触发 ContentView shouldShow* 重新计算 → 移除蒙版
        let key = "\(config.commitKey)-\(Self.dayKey(for: config.dayKind))"
        UserDefaults.standard.set(true, forKey: key)
        onComplete()
    }

    /// 按 dayKind 算「日」字符串, 决定 commit key 何时失效。
    /// - calendar: yyyy-MM-dd 自然日, 0 点跨日 key 失效 → 蒙版重弹
    /// - sleepNight: h < SleepGuard.wakeHour 算前一日, 0~6 点跨日 key 不失效
    static func dayKey(for kind: CommitRitualDayKind, now: Date = Date()) -> String {
        let cal = Calendar.current
        let base: Date
        switch kind {
        case .calendar:
            base = now
        case .sleepNight:
            let hour = cal.component(.hour, from: now)
            base = hour < SleepGuard.wakeHour
                ? cal.date(byAdding: .day, value: -1, to: now) ?? now
                : now
        }
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: base)
    }

    /// 给 ContentView 的 shouldShow 逻辑用 — 当日/当夜 commit key 是否已 set。
    static func isCommitted(_ config: CommitRitualConfig) -> Bool {
        UserDefaults.standard.bool(forKey: "\(config.commitKey)-\(dayKey(for: config.dayKind))")
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
