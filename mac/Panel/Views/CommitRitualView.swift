import SwiftUI

/// 双阶段确认仪式蒙版 — 全 panel 覆盖, 强制用户做两次有意识的 commitment。
/// 阶段 1: 立刻可点的按钮 → 进阶段 2
/// 阶段 2: countdownSec 秒倒计时, 结束后按钮才可点 → 写 today commit key 到 UserDefaults
///        + onComplete 触发 SwiftUI 重画移除蒙版。
///
/// 触发逻辑由 ContentView 决定 (focusStart / 22:00 / today key 是否已 set);
/// 本 view 只关心 stage 状态和 commit 写入。
///
/// 视觉参考 WakeMaskView 的渐变全屏样式。
struct CommitRitualConfig {
    let title: String
    let stage1Body: String
    let stage1Button: String
    let stage2Body: String
    let stage2Button: String
    let countdownSec: Int
    /// UserDefaults key 前缀, 实际 key = "{commitKey}-{yyyy-MM-dd}"。
    /// 跨日自然失效 (新日期 key 不存在 → 蒙版重弹)。
    let commitKey: String
}

extension CommitRitualConfig {
    static let focus = CommitRitualConfig(
        title: "进入专注模式",
        stage1Body: "关闭社交、即时通讯,防止打扰、防止引来打扰",
        stage1Button: "我准备好",
        stage2Body: "确认所有社交已关闭?",
        stage2Button: "确认关闭社交",
        countdownSec: 5,
        commitKey: "focusRitualCommit"
    )

    static let phone = CommitRitualConfig(
        title: "收起白色手机",
        stage1Body: "把白色 iPhone 14 拿到那个晚上不会再碰的位置",
        stage1Button: "我去拿了",
        stage2Body: "已经放到那个地方了吗?",
        stage2Button: "确认放好",
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

                Text(stage == 1 ? config.stage1Body : config.stage2Body)
                    .font(.title2)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.white.opacity(0.9))
                    .frame(maxWidth: 600)
                    .fixedSize(horizontal: false, vertical: true)

                Button(action: handleButton) {
                    Text(buttonLabel)
                        .font(.title2.weight(.medium))
                        .foregroundStyle(buttonReady ? .white : .white.opacity(0.4))
                        .frame(width: 360, height: 72)
                        .background(
                            buttonReady ? Color.anchorAccent : .white.opacity(0.08),
                            in: RoundedRectangle(cornerRadius: 16)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 16)
                                .stroke(.white.opacity(buttonReady ? 0 : 0.2), lineWidth: 1)
                        )
                }
                .buttonStyle(.plain)
                .disabled(!buttonReady)
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

    private var buttonReady: Bool {
        stage == 1 || countdown == 0
    }

    private var buttonLabel: String {
        if stage == 1 { return config.stage1Button }
        if countdown > 0 { return "\(config.stage2Button) (\(countdown))" }
        return config.stage2Button
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
