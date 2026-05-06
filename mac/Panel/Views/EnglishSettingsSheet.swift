import SwiftUI

/// 英文精听设置 sheet — 编辑 windows / threshold / enabled。
/// 触发: EnglishView 顶部齿轮按钮。
/// 保存: PUT /api/english/settings, backend 立即 kick GenerateNotifications,
///       通知 body 会同步到新 windows 边界 (issue-22 引入的 UpdateNotificationBodyByRef 机制)。
struct EnglishSettingsSheet: View {
    let initial: EnglishSettings

    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var settings: EnglishSettings

    init(initial: EnglishSettings) {
        self.initial = initial
        _settings = State(initialValue: initial)
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("英文精听设置")
                    .font(.headline)
                Spacer()
                Button("取消") { dismiss() }
                Button("保存") {
                    Task {
                        await state.saveEnglishSettings(settings)
                        dismiss()
                    }
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .disabled(!canSave)
            }
            .padding()
            Divider()

            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    HStack {
                        Text("启用").frame(width: 80, alignment: .leading)
                        Toggle(isOn: $settings.enabled) {
                            Text(settings.enabled
                                 ? "通知正常,达阈值前 15 分钟提醒"
                                 : "停用 (通知不弹,统计仍记录)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .toggleStyle(.switch)
                        Spacer()
                    }

                    HStack {
                        Text("阈值").frame(width: 80, alignment: .leading)
                        Stepper(value: $settings.thresholdMin, in: 1...240, step: 1) {
                            Text("\(settings.thresholdMin) 分钟 / 天")
                                .font(.body.monospacedDigit())
                        }
                        .frame(maxWidth: 280)
                    }

                    VStack(alignment: .leading, spacing: 8) {
                        HStack {
                            Text("时段").frame(width: 80, alignment: .leading)
                            Spacer()
                            Button {
                                settings.windows.append(EnglishWindow(start: "12:00", end: "14:00"))
                            } label: {
                                Label("添加时段", systemImage: "plus.circle")
                            }
                            .buttonStyle(.borderless)
                        }
                        if settings.windows.isEmpty {
                            Text("无时段, 不会弹精听提醒")
                                .font(.caption).foregroundStyle(.tertiary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(.leading, 80)
                        } else {
                            ForEach(Array(settings.windows.enumerated()), id: \.offset) { idx, _ in
                                WindowRow(
                                    start: $settings.windows[idx].start,
                                    end: $settings.windows[idx].end,
                                    onDelete: { settings.windows.remove(at: idx) }
                                )
                            }
                            Text("时段为 HH:MM, start 必须 < end (跨天暂不支持)")
                                .font(.caption2).foregroundStyle(.tertiary)
                                .padding(.leading, 80)
                            if !overlapPairs.isEmpty {
                                ForEach(overlapPairs) { pair in
                                    Label(
                                        "时段重叠: \(settings.windows[pair.i].start)–\(settings.windows[pair.i].end) 与 \(settings.windows[pair.j].start)–\(settings.windows[pair.j].end)",
                                        systemImage: "exclamationmark.triangle.fill"
                                    )
                                    .font(.caption)
                                    .foregroundStyle(.red)
                                    .padding(.leading, 80)
                                }
                            }
                        }
                    }
                }
                .padding()
            }
        }
        .frame(minWidth: 520, minHeight: 380)
    }

    private var canSave: Bool {
        guard settings.thresholdMin > 0, settings.thresholdMin <= 240 else { return false }
        for w in settings.windows {
            guard let s = Self.parseHHMM(w.start), let e = Self.parseHHMM(w.end), s < e else {
                return false
            }
        }
        return overlapPairs.isEmpty
    }

    struct OverlapPair: Identifiable, Hashable {
        let i: Int
        let j: Int
        var id: String { "\(i)-\(j)" }
    }

    /// windows 两两 HH:MM 比较, 返回相交对 (左闭右开, 跟保健品 SupplementForms.overlaps 同语义)。
    private var overlapPairs: [OverlapPair] {
        var out: [OverlapPair] = []
        let n = settings.windows.count
        for i in 0..<n {
            for j in (i + 1)..<n {
                if Self.overlaps(settings.windows[i], settings.windows[j]) {
                    out.append(OverlapPair(i: i, j: j))
                }
            }
        }
        return out
    }

    static func overlaps(_ a: EnglishWindow, _ b: EnglishWindow) -> Bool {
        guard let s1 = parseHHMM(a.start), let e1 = parseHHMM(a.end),
              let s2 = parseHHMM(b.start), let e2 = parseHHMM(b.end),
              e1 > s1, e2 > s2 else { return false }
        return s1 < e2 && s2 < e1
    }

    static func parseHHMM(_ s: String) -> Int? {
        let parts = s.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]),
              (0...23).contains(h), (0...59).contains(m) else { return nil }
        return h * 60 + m
    }
}

private struct WindowRow: View {
    @Binding var start: String
    @Binding var end: String
    let onDelete: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Spacer().frame(width: 80 - 8)
            TextField("起", text: $start)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 120)
            Text("→").foregroundStyle(.tertiary)
            TextField("止", text: $end)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 120)
            Spacer()
            Button(action: onDelete) {
                Image(systemName: "minus.circle")
                    .foregroundStyle(.red)
            }
            .buttonStyle(.borderless)
        }
    }
}
