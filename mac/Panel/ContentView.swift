import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state
    /// 切日时强制 view 重算 — UserDefaults 在 SwiftUI 不可观察, 用 ritualTick 触发 timeTick 之外的重画。
    @State private var ritualTick: Int = 0

    var body: some View {
        @Bindable var state = state
        ZStack {
            MainLayout()
            // 优先级: 早晨 wake > 晨间 morning > 晚 phone > 17 点 focus
            // 任一时刻最多 1 个全屏蒙版 — 22:00 后 phone ritual 优先于 focus
            // (即使 17-22 没确认 focus, 22 后 sleep 时段也不再有意义弹 focus 蒙版)。
            if shouldShowWakeMask {
                WakeMaskView()
                    .transition(.opacity)
            } else if shouldShowMorningRitual {
                CommitRitualView(config: .morning, onComplete: { ritualTick += 1 })
                    .transition(.opacity)
            } else if shouldShowPhoneRitual {
                CommitRitualView(config: .phone, onComplete: { ritualTick += 1 })
                    .transition(.opacity)
            } else if shouldShowFocusRitual {
                CommitRitualView(config: .focus, onComplete: { ritualTick += 1 })
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: shouldShowWakeMask)
        .animation(.easeInOut(duration: 0.3), value: shouldShowMorningRitual)
        .animation(.easeInOut(duration: 0.3), value: shouldShowPhoneRitual)
        .animation(.easeInOut(duration: 0.3), value: shouldShowFocusRitual)
        .task { await state.startPolling() }
        .alert(
            "加载错误",
            isPresented: Binding(
                get: { state.lastError != nil },
                set: { if !$0 { state.lastError = nil } }
            ),
            actions: { Button("OK", role: .cancel) {} },
            message: { Text(state.lastError ?? "") }
        )
    }

    /// 蒙版条件: 06:00+ 当日 mode 还没人为设置过 (mode_set_at == 0)。
    /// dailyState 还没加载完时 (启动初瞬, 异步请求未返) 不弹蒙版 — 老逻辑用
    /// `(state.dailyState?.modeSetAt ?? 0) == 0` 把 nil 当成 0 → 启动初一闪而过,
    /// 加载完发现 modeSetAt != 0 又消失。改成 guard nil 视作 "loading", 不显示。
    private var shouldShowWakeMask: Bool {
        let _ = state.timeTick
        guard let ds = state.dailyState else { return false }
        let h = Calendar.current.component(.hour, from: Date())
        guard h >= 6 else { return false }
        return ds.modeSetAt == 0
    }

    /// 用户选了普通模式 (wake mask 已确认过) 后, 早晨~午前弹晨间 4 项 ritual。
    /// 外出模式 / 已 commit / 时间过了 focusStart 都不弹 (让位给 focus / phone)。
    private var shouldShowMorningRitual: Bool {
        let _ = state.timeTick
        let _ = ritualTick
        guard let ds = state.dailyState, let ss = state.stateSettings else { return false }
        guard ds.modeSetAt != 0, ds.mode == "normal" else { return false }
        let comps = Calendar.current.dateComponents([.hour, .minute], from: Date())
        let nowMin = (comps.hour ?? 0) * 60 + (comps.minute ?? 0)
        let focusMin = Self.parseHHMM(ss.focusStart)
        guard nowMin >= 6 * 60 && nowMin < focusMin else { return false }
        return !CommitRitualView.isCommitted(CommitRitualConfig.morning)
    }

    /// 22:00 ~ 次日 06:00 弹白色手机 ritual, 当日已 commit (UserDefaults today key set) 则不弹。
    /// 时间窗复用 SleepGuard.isActive() — 跟黑色蒙版同生同灭, 跨 0 点不消失, 直到 commit。
    private var shouldShowPhoneRitual: Bool {
        let _ = state.timeTick
        let _ = ritualTick
        guard SleepGuard.isActive() else { return false }
        return !CommitRitualView.isCommitted(CommitRitualConfig.phone)
    }

    /// 17:00 (focusStart, 用户可调) 后到 sleepStart 之间弹专注 ritual,
    /// 当日已 commit 则不弹。22:00 后让位给 phone ritual (else if 优先级)。
    private var shouldShowFocusRitual: Bool {
        let _ = state.timeTick
        let _ = ritualTick
        guard let ss = state.stateSettings else { return false }
        let comps = Calendar.current.dateComponents([.hour, .minute], from: Date())
        let nowMin = (comps.hour ?? 0) * 60 + (comps.minute ?? 0)
        let focusMin = Self.parseHHMM(ss.focusStart)
        let sleepMin = Self.parseHHMM(ss.sleepStart)
        guard nowMin >= focusMin && nowMin < sleepMin else { return false }
        return !CommitRitualView.isCommitted(CommitRitualConfig.focus)
    }

    /// "HH:MM" → minutes 0..1439, 解析失败返 0 (让条件保守不触发)。
    private static func parseHHMM(_ s: String) -> Int {
        let parts = s.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return 0 }
        return h * 60 + m
    }
}
