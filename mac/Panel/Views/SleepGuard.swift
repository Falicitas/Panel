import SwiftUI

/// 22:00 ~ 次日 06:00 (硬编码) 期间 todolist 蒙版, 强制脱离工作进入睡眠。
/// 没有 toggle / 没有 settings 入口 — 改时间需要 rebuild Panel.app, 即用户原话
/// "除非重新打包改硬编码否则解除不了"。
enum SleepGuard {
    /// 蒙版起效小时 (本地时区, 0-23)。改这里需要 rebuild。
    static let cutoffHour: Int = 22
    /// 蒙版解除小时 (本地时区, 0-23)。22:00 ~ 次日 wakeHour:00 都显示。
    static let wakeHour: Int = 6

    static func isActive(now: Date = Date()) -> Bool {
        let hour = Calendar.current.component(.hour, from: now)
        return hour >= cutoffHour || hour < wakeHour
    }
}

/// 全屏黑底蒙版 view。挂在 todolist content / detail column 的 .overlay 里,
/// TimelineView 每 30s 自动重算 isActive (跨 22:00 边界不需要手动 trigger)。
/// 蒙版透 0.92, 下方 todolist 仍隐约可见但 hit testing 被 ZStack 截住, 无法编辑/勾选。
struct SleepGuardOverlay: View {
    var body: some View {
        TimelineView(.periodic(from: .now, by: 30)) { ctx in
            if SleepGuard.isActive(now: ctx.date) {
                ZStack {
                    Color.black.opacity(0.92).ignoresSafeArea()
                    VStack(spacing: 18) {
                        Image(systemName: "moon.stars.fill")
                            .font(.system(size: 64))
                            .foregroundStyle(.white.opacity(0.75))
                        Text("夜间锁定中, 请休息")
                            .font(.title.weight(.semibold))
                            .foregroundStyle(.white)
                        Text("明天再继续 todolist")
                            .font(.title3)
                            .foregroundStyle(.white.opacity(0.7))
                        Text("(蒙版硬编码, 解除需重新打包)")
                            .font(.caption)
                            .foregroundStyle(.white.opacity(0.4))
                    }
                    .padding(40)
                }
                .transition(.opacity)
            }
        }
    }
}
