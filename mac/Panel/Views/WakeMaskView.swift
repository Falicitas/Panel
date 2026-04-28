import SwiftUI

struct WakeMaskView: View {
    @Environment(AppState.self) private var state
    @State private var now = Date()
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
                VStack(spacing: 8) {
                    Text(timeString)
                        .font(.system(size: 72, weight: .light).monospacedDigit())
                    Text(dateString)
                        .font(.title3)
                        .foregroundStyle(.secondary)
                }

                Text("早安")
                    .font(.system(size: 48, weight: .light))

                HStack(spacing: 20) {
                    BigButton(
                        title: "进入普通模式",
                        subtitle: "录入复发 todo · 到点提醒",
                        systemImage: "sun.max"
                    ) {
                        Task { await state.setMode("normal", wakeTime: hmString) }
                    }
                    BigButton(
                        title: "今天外出",
                        subtitle: "不弹食物保健品提醒",
                        systemImage: "figure.walk"
                    ) {
                        Task { await state.setMode("outside", wakeTime: hmString) }
                    }
                }
                .padding(.top, 8)
            }
            .foregroundStyle(.white)
            .padding(40)
        }
        .onReceive(timer) { now = $0 }
    }

    private var timeString: String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: now)
    }

    private var dateString: String {
        let f = DateFormatter()
        f.dateFormat = "yyyy 年 M 月 d 日 EEEE"
        f.locale = Locale(identifier: "zh_CN")
        return f.string(from: now)
    }

    private var hmString: String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: now)
    }
}

private struct BigButton: View {
    let title: String
    let subtitle: String
    let systemImage: String
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
        }
        .buttonStyle(.plain)
    }
}
