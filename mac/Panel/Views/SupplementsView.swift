import SwiftUI

/// 食物 / 保健品统一大版面 (sidebar + 单一详情列)。
/// 设计:每项一个聚合 row,内含 per-window checkbox + 自己的小时间线。
/// 整体字号通过 dynamicTypeSize 放大一档。
struct SupplementsView: View {
    @Environment(AppState.self) private var state
    @State private var filter: SupplementsFilter = .all
    @State private var search: String = ""
    @State private var creating = false
    @State private var editing: Supplement?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Header(filter: $filter, search: $search, today: state.supplementsToday, onAdd: { creating = true })
                AllListSection(
                    today: state.supplementsToday,
                    supplementsByID: supplementsByID,
                    filter: filter,
                    search: search,
                    onToggleWindow: { item, idx in
                        Task { await state.toggleSupplementWindowCheck(item, windowIndex: idx) }
                    },
                    onEdit: { sp in editing = sp },
                    onPause: { sp, days in Task { await state.pauseSupplement(sp, days: days) } },
                    onEnable: { sp, on in Task { await state.setSupplementEnabled(sp, enabled: on) } }
                )
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .navigationTitle("食物 & 保健品")
        .dynamicTypeSize(.accessibility2)  // 整体字号放大一档
        .sheet(isPresented: $creating) {
            SupplementFormSheet(initial: .blank, isNew: true).environment(state)
        }
        .sheet(item: $editing) { sp in
            SupplementFormSheet(initial: sp, isNew: false).environment(state)
        }
    }

    private var supplementsByID: [Int64: Supplement] {
        Dictionary(uniqueKeysWithValues: state.supplements.map { ($0.id, $0) })
    }
}

enum SupplementsFilter: String, CaseIterable, Identifiable {
    case all, food, supplement
    var id: String { rawValue }
    var label: String {
        switch self {
        case .all:        "全部"
        case .food:       "食物"
        case .supplement: "保健品"
        }
    }
}

// MARK: - Header

private struct Header: View {
    @Binding var filter: SupplementsFilter
    @Binding var search: String
    let today: SupplementToday?
    let onAdd: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Picker("", selection: $filter) {
                    ForEach(SupplementsFilter.allCases) { t in
                        Text(t.label).tag(t)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .frame(maxWidth: 280)

                TextField("搜索 (名称 / 备注)", text: $search)
                    .textFieldStyle(.roundedBorder)
                    .frame(maxWidth: 320)

                Spacer()

                Button(action: onAdd) {
                    Label("新增", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
            }

            if let t = today {
                HStack(spacing: 12) {
                    StatChip(label: "今日完成", value: "\(t.doneDoses) / \(t.totalDoses) 次", tint: .green)
                    StatChip(label: "现在该做", value: "\(t.activeCount) 项", tint: .anchorAccent)
                    StatChip(label: "项目数", value: "\(t.totalActive) 项", tint: .secondary)
                    Spacer()
                }
            }
        }
    }
}

private struct StatChip: View {
    let label: String
    let value: String
    let tint: Color
    var body: some View {
        HStack(spacing: 6) {
            Circle().fill(tint).frame(width: 6, height: 6)
            Text(label).foregroundStyle(.secondary)
            Text(value).monospacedDigit()
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 4)
        .background(.regularMaterial, in: Capsule())
    }
}

// MARK: - 清单 (per-item 聚合)

private struct AllListSection: View {
    let today: SupplementToday?
    let supplementsByID: [Int64: Supplement]
    let filter: SupplementsFilter
    let search: String
    let onToggleWindow: (SupplementTodayItem, Int) -> Void
    let onEdit: (Supplement) -> Void
    let onPause: (Supplement, Int) -> Void
    let onEnable: (Supplement, Bool) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if filteredItems.isEmpty {
                ContentUnavailableView {
                    Label("没有匹配的项", systemImage: "magnifyingglass")
                } description: {
                    Text("调整 filter / 搜索词,或右上 + 新增")
                }
                .frame(maxWidth: .infinity, minHeight: 240)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
            } else {
                ForEach(filteredItems) { it in
                    if let sp = supplementsByID[it.id] {
                        ItemCard(
                            item: it, sp: sp,
                            onToggleWindow: { idx in onToggleWindow(it, idx) },
                            onEdit: { onEdit(sp) },
                            onPause: { days in onPause(sp, days) },
                            onEnable: { on in onEnable(sp, on) }
                        )
                    }
                }
            }
        }
    }

    private var filteredItems: [SupplementTodayItem] {
        guard let t = today else { return [] }
        return t.items.filter { it in
            matchesFilter(it.type) && matchesSearch(it.name, it.notes)
        }
    }

    private func matchesFilter(_ type: String) -> Bool {
        switch filter {
        case .all:        true
        case .food:       type == "food"
        case .supplement: type == "supplement"
        }
    }

    private func matchesSearch(_ name: String, _ notes: String) -> Bool {
        let q = search.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty else { return true }
        let lq = q.lowercased()
        return name.lowercased().contains(lq) || notes.lowercased().contains(lq)
    }
}

// MARK: - 单项 Card

private struct ItemCard: View {
    let item: SupplementTodayItem
    let sp: Supplement
    let onToggleWindow: (Int) -> Void
    let onEdit: () -> Void
    let onPause: (Int) -> Void
    let onEnable: (Bool) -> Void

    @Environment(AppState.self) private var state

    var body: some View {
        let _ = state.timeTick   // 跟着 tick 重画 active 状态
        VStack(alignment: .leading, spacing: 12) {
            // 顶部:名称 / 类型 / 状态 / actions
            HStack(spacing: 10) {
                Image(systemName: item.type == "supplement" ? "pills" : "leaf")
                    .foregroundStyle(item.type == "supplement" ? Color.accentColor : Color.green)
                    .imageScale(.large)
                Text(item.name)
                    .font(.title3.weight(.semibold))
                if !item.dose.isEmpty {
                    Text("· \(item.dose)")
                        .foregroundStyle(.secondary)
                }
                statusChip
                Spacer()
                actions
            }

            // per-window checkboxes
            if item.windows.isEmpty {
                Text("尚无时段")
                    .foregroundStyle(.tertiary)
            } else {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 170), spacing: 10)], spacing: 10) {
                    ForEach(Array(item.windows.enumerated()), id: \.offset) { idx, win in
                        WindowChip(window: win, disabled: item.paused || !item.enabled) {
                            onToggleWindow(idx)
                        }
                    }
                }
            }

            // 自己的小时间线
            if !item.windows.isEmpty {
                ItemTimeline(windows: item.windows)
                    .frame(height: 26)
            }

            if !item.notes.isEmpty {
                Text(item.notes)
                    .font(.callout)
                    .foregroundStyle(.tertiary)
                    .lineLimit(3)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(rowBackground, in: RoundedRectangle(cornerRadius: 12))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(rowStroke, lineWidth: item.hasActive ? 1.5 : 1)
        )
        .opacity(item.enabled ? 1 : 0.55)
    }

    @ViewBuilder
    private var statusChip: some View {
        if !item.enabled {
            Chip(text: "已禁用", color: .secondary)
        } else if item.paused, let until = item.pausedUntil {
            Chip(text: "暂停至 " + dateLabel(until), color: .orange)
        } else if item.hasActive {
            Chip(text: "现在该做", color: .anchorAccent)
        } else if item.doneToday {
            Chip(text: "今日已勾完", color: .green)
        }
    }

    private var actions: some View {
        HStack(spacing: 4) {
            Menu {
                Button("暂停 1 天") { onPause(1) }
                Button("暂停 7 天") { onPause(7) }
                Button("暂停 14 天") { onPause(14) }
                Button("暂停 30 天") { onPause(30) }
                if item.paused {
                    Divider()
                    Button("取消暂停") { onPause(0) }
                }
            } label: {
                Image(systemName: item.paused ? "play.circle" : "pause.circle")
                    .foregroundStyle(.secondary)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .frame(width: 36)
            .help(item.paused ? "暂停中,点击调整 / 取消" : "暂停 N 天")

            Button { onEnable(!item.enabled) } label: {
                Image(systemName: item.enabled ? "power" : "power.dotted")
                    .foregroundStyle(item.enabled ? Color.secondary : Color.red)
            }
            .buttonStyle(.borderless)
            .help(item.enabled ? "停用 (通知不弹,历史保留)" : "重新启用")

            Button(action: onEdit) {
                Image(systemName: "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("编辑")
        }
    }

    private var rowBackground: Color {
        if item.hasActive { return Color.anchorAccent.opacity(0.08) }
        if item.doneToday { return Color.green.opacity(0.06) }
        return Color.clear
    }

    private var rowStroke: Color {
        if item.hasActive { return Color.anchorAccent.opacity(0.5) }
        return Color.secondary.opacity(0.2)
    }

    private func dateLabel(_ unix: Int64) -> String {
        let d = Date(timeIntervalSince1970: TimeInterval(unix))
        let f = DateFormatter()
        f.dateFormat = "M月d日"
        return f.string(from: d)
    }
}

// MARK: - Window Chip (per-window checkbox)

private struct WindowChip: View {
    let window: SupplementWindowStatus
    let disabled: Bool
    let onToggle: () -> Void

    /// 是否可点击:仅 active 窗口可点 (符合用户要求"在对应时间可以吃,不在则灰色不可点")。
    /// disabled (项被暂停/禁用) 时整体不可点。
    private var isClickable: Bool {
        !disabled && window.status == "active"
    }

    var body: some View {
        Button(action: onToggle) {
            HStack(spacing: 8) {
                Image(systemName: iconName)
                    .font(.title3)
                    .foregroundStyle(iconColor)
                VStack(alignment: .leading, spacing: 2) {
                    Text(timeLabel)
                        .font(.body.monospacedDigit())
                        .foregroundStyle(textColor)
                    Text(statusText)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(bgColor.opacity(0.18), in: RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(bgColor.opacity(0.4), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
        .disabled(!isClickable)
    }

    private var timeLabel: String {
        let s = window.startHM.isEmpty ? window.start : window.startHM
        let e = window.endHM.isEmpty ? window.end : window.endHM
        return "\(s)–\(e)"
    }

    private var iconName: String {
        if window.doneToday { return "checkmark.circle.fill" }
        switch window.status {
        case "active":   return "circle"
        case "passed":   return "xmark.circle"
        case "upcoming": return "circle.dashed"
        default:         return "questionmark.circle"
        }
    }

    private var iconColor: Color {
        if window.doneToday { return .green }
        switch window.status {
        case "active":   return Color.anchorAccent
        case "passed":   return .red.opacity(0.6)
        case "upcoming": return .secondary
        default:         return .secondary
        }
    }

    private var bgColor: Color {
        if window.doneToday { return .green }
        switch window.status {
        case "active":   return Color.anchorAccent
        case "passed":   return Color.red.opacity(0.5)
        case "upcoming": return .secondary
        default:         return .secondary
        }
    }

    private var textColor: Color {
        if disabled || window.status == "upcoming" { return .secondary }
        return .primary
    }

    private var statusText: String {
        if window.doneToday { return "✓ 已" }
        switch window.status {
        case "active":   return "可勾"
        case "passed":   return "已过"
        case "upcoming": return "待开"
        default:         return ""
        }
    }
}

// MARK: - 单项时间线 (mini)

private struct ItemTimeline: View {
    let windows: [SupplementWindowStatus]

    private static let dayStartMin = 6 * 60   // 06:00
    private static let dayEndMin = 22 * 60    // 22:00

    var body: some View {
        GeometryReader { geo in
            let w = geo.size.width
            ZStack(alignment: .topLeading) {
                Capsule()
                    .fill(Color.secondary.opacity(0.12))
                    .frame(height: 8)
                    .padding(.top, 6)

                ForEach(Array(windows.enumerated()), id: \.offset) { _, win in
                    if let seg = segment(for: win) {
                        Capsule()
                            .fill(color(for: win).opacity(0.7))
                            .frame(width: max(2, w * seg.width), height: 8)
                            .offset(x: w * seg.start, y: 6)
                    }
                }

                // now ruler
                let nf = nowFrac()
                if (0...1).contains(nf) {
                    Rectangle()
                        .fill(Color.anchorAccent)
                        .frame(width: 1.5, height: 16)
                        .offset(x: w * nf, y: 2)
                }

                // tick labels 6/12/18/22
                ForEach([6, 12, 18, 22], id: \.self) { hour in
                    Text(String(format: "%02d", hour))
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.tertiary)
                        .offset(x: w * frac(hour * 60) - 8, y: 16)
                }
            }
        }
    }

    private struct Seg { let start: Double; let width: Double }

    private func segment(for w: SupplementWindowStatus) -> Seg? {
        guard let s = parseHM(w.startHM), let e = parseHM(w.endHM), e > s else { return nil }
        let sf = frac(s)
        let ef = frac(e)
        guard ef > sf else { return nil }
        return Seg(start: max(0, sf), width: min(1, ef) - max(0, sf))
    }

    private func parseHM(_ s: String) -> Int? {
        let parts = s.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return nil }
        return h * 60 + m
    }

    private func frac(_ minutes: Int) -> Double {
        let span = Double(Self.dayEndMin - Self.dayStartMin)
        return max(0, min(1, (Double(minutes) - Double(Self.dayStartMin)) / span))
    }

    private func nowFrac() -> Double {
        let comps = Calendar.current.dateComponents([.hour, .minute], from: Date())
        let mins = (comps.hour ?? 0) * 60 + (comps.minute ?? 0)
        return frac(mins)
    }

    private func color(for w: SupplementWindowStatus) -> Color {
        if w.doneToday { return .green }
        switch w.status {
        case "active":   return Color.anchorAccent
        case "passed":   return .red.opacity(0.5)
        case "upcoming": return .secondary
        default:         return .secondary
        }
    }
}

// MARK: - Chip

private struct Chip: View {
    let text: String
    let color: Color
    var body: some View {
        Text(text)
            .font(.caption)
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }
}
