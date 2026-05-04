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
    @State private var duplicating: Supplement?

    var body: some View {
        ScrollView {
            ScrollViewReader { proxy in
                VStack(alignment: .leading, spacing: 20) {
                    Header(filter: $filter, search: $search, today: state.supplementsToday, onAdd: { creating = true })
                    MealsSection(
                        dailyState: state.dailyState,
                        onTrigger: { meal in Task { await state.triggerMeal(meal) } },
                        onClear: { meal in Task { await state.triggerMeal(meal, clear: true) } }
                    )
                    AllListSection(
                        today: state.supplementsToday,
                        supplementsByID: supplementsByID,
                        orderedIDs: state.supplements.map(\.id),
                        filter: filter,
                        search: search,
                        onToggleWindow: { item, idx in
                            Task { await state.toggleSupplementWindowCheck(item, windowIndex: idx) }
                        },
                        onEdit: { sp in editing = sp },
                        onPause: { sp, days in Task { await state.pauseSupplement(sp, days: days) } },
                        onEnable: { sp, on in Task { await state.setSupplementEnabled(sp, enabled: on) } },
                        onMove: { sp, dir in Task { await state.moveSupplement(sp, direction: dir) } }
                    )
                }
                .padding(20)
                .frame(maxWidth: .infinity, alignment: .topLeading)
                .onChange(of: state.pendingScrollSupplement) { _, target in
                    guard let target else { return }
                    withAnimation(.easeInOut(duration: 0.4)) {
                        proxy.scrollTo(target, anchor: .top)
                    }
                    Task { @MainActor in
                        try? await Task.sleep(for: .milliseconds(800))
                        if state.pendingScrollSupplement == target {
                            state.pendingScrollSupplement = nil
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .navigationTitle("食物 & 保健品")
        .toolbar {
            ToolbarItem { JumpToFocusButton() }
        }
        .dynamicTypeSize(.accessibility5)  // 整体字号再放大一档 (用户反馈 .accessibility2 仍嫌小)
        .sheet(isPresented: $creating) {
            SupplementFormSheet(initial: .blank, isNew: true).environment(state)
        }
        .sheet(item: $editing) { sp in
            SupplementFormSheet(
                initial: sp,
                isNew: false,
                onDuplicate: { current in
                    // form 已 dismiss 自身, 这里异步弹复制 sheet 防 SwiftUI sheet 替换 race。
                    // current = form 当前 @State (用户可能改了 dose/notes 才点复制)
                    let dup = Supplement(
                        id: 0, name: current.name + " (副本)", type: current.type,
                        windows: current.windows, notes: current.notes, dose: current.dose,
                        enabled: true, pausedUntil: nil
                    )
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) {
                        duplicating = dup
                    }
                }
            ).environment(state)
        }
        .sheet(item: $duplicating) { sp in
            SupplementFormSheet(initial: sp, isNew: true).environment(state)
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
                .frame(maxWidth: 320)
                .font(.title3)

                TextField("搜索 (名称 / 备注)", text: $search)
                    .textFieldStyle(.roundedBorder)
                    .frame(maxWidth: 360)
                    .font(.title3)

                Spacer()

                Button(action: onAdd) {
                    Label("新增", systemImage: "plus")
                        .font(.title3)
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
        HStack(spacing: 8) {
            Circle().fill(tint).frame(width: 8, height: 8)
            Text(label)
                .font(.title3)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.title3.monospacedDigit())
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 5)
        .background(.regularMaterial, in: Capsule())
    }
}

// MARK: - 餐 Section (置顶 3 张餐卡)

private struct MealMeta {
    let key: String      // breakfast/lunch/dinner
    let label: String    // 早餐/午餐/晚餐
    let icon: String     // SF symbol
    let validRange: ClosedRange<Int>  // 小时 (闭区间) — 7...11 / 12...15 / 16...23
    let rangeLabel: String  // "7:00-11:59"
}

private let mealMetas: [MealMeta] = [
    .init(key: "breakfast", label: "早餐", icon: "sun.haze", validRange: 7...11, rangeLabel: "7:00–11:59"),
    .init(key: "lunch",     label: "午餐", icon: "sun.max",  validRange: 12...15, rangeLabel: "12:00–15:59"),
    .init(key: "dinner",    label: "晚餐", icon: "moon",     validRange: 16...23, rangeLabel: "16:00–23:59"),
]

private struct MealsSection: View {
    let dailyState: DailyState?
    let onTrigger: (String) -> Void
    let onClear: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 6) {
                Image(systemName: "fork.knife")
                    .foregroundStyle(.secondary)
                Text("今日三餐 (锚点)")
                    .font(.headline)
                Text("触发后,引用 breakfast/lunch/dinner 的保健品时段才会浮现")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 220), spacing: 12)], spacing: 12) {
                ForEach(mealMetas, id: \.key) { meta in
                    MealCard(
                        meta: meta,
                        recordedAt: recorded(meta.key),
                        onTrigger: { onTrigger(meta.key) },
                        onClear: { onClear(meta.key) }
                    )
                }
            }
        }
    }

    private func recorded(_ key: String) -> String? {
        guard let ds = dailyState else { return nil }
        let v: String
        switch key {
        case "breakfast": v = ds.breakfastAt
        case "lunch":     v = ds.lunchAt
        case "dinner":    v = ds.dinnerAt
        default:          v = ""
        }
        return v.isEmpty ? nil : v
    }
}

private struct MealCard: View {
    let meta: MealMeta
    let recordedAt: String?
    let onTrigger: () -> Void
    let onClear: () -> Void

    @Environment(AppState.self) private var state

    private var inRange: Bool {
        let _ = state.timeTick
        let hour = Calendar.current.component(.hour, from: Date())
        return meta.validRange.contains(hour)
    }

    var body: some View {
        let _ = state.timeTick
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: meta.icon)
                    .foregroundStyle(recordedAt != nil ? Color.green : Color.anchorAccent)
                    .imageScale(.large)
                Text(meta.label)
                    .font(.title3.weight(.semibold))
                Spacer()
                if let at = recordedAt {
                    Text(at)
                        .font(.body.monospacedDigit())
                        .foregroundStyle(.green)
                }
            }
            Text(meta.rangeLabel)
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
            HStack {
                if let at = recordedAt {
                    Label("已记 \(at)", systemImage: "checkmark.circle.fill")
                        .font(.caption)
                        .foregroundStyle(.green)
                    Spacer()
                    Button("清空") { onClear() }
                        .buttonStyle(.borderless)
                        .help("撤销今日触发")
                } else {
                    Button(action: onTrigger) {
                        Label("现在记", systemImage: "checkmark.circle")
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(!inRange)
                    .help(inRange ? "记录当前时刻为 \(meta.label)" : "不在 \(meta.label) 时段")
                    Spacer()
                    if !inRange {
                        Text("不在区间")
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                    }
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            (recordedAt != nil ? Color.green : Color.anchorAccent).opacity(0.06),
            in: RoundedRectangle(cornerRadius: 10)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(
                    (recordedAt != nil ? Color.green : Color.anchorAccent).opacity(0.35),
                    lineWidth: 1
                )
        )
    }
}

// MARK: - 清单 (per-item 聚合)

private struct AllListSection: View {
    let today: SupplementToday?
    let supplementsByID: [Int64: Supplement]
    /// 完整全列表 (含被 filter/search 过滤的) 在 backend 的真实 sort_order 顺序。
    /// ItemCard 用它判断 isFirst / isLast 决定上下移按钮的 disabled。
    let orderedIDs: [Int64]
    let filter: SupplementsFilter
    let search: String
    let onToggleWindow: (SupplementTodayItem, Int) -> Void
    let onEdit: (Supplement) -> Void
    let onPause: (Supplement, Int) -> Void
    let onEnable: (Supplement, Bool) -> Void
    let onMove: (Supplement, Int) -> Void  // direction = -1 上移, +1 下移

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
                        let realIdx = orderedIDs.firstIndex(of: it.id) ?? 0
                        ItemCard(
                            item: it, sp: sp,
                            isFirst: realIdx == 0,
                            isLast: realIdx == orderedIDs.count - 1,
                            onToggleWindow: { idx in onToggleWindow(it, idx) },
                            onEdit: { onEdit(sp) },
                            onPause: { days in onPause(sp, days) },
                            onEnable: { on in onEnable(sp, on) },
                            onMove: { dir in onMove(sp, dir) }
                        )
                        .id(it.id)
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
    let isFirst: Bool
    let isLast: Bool
    let onToggleWindow: (Int) -> Void
    let onEdit: () -> Void
    let onPause: (Int) -> Void
    let onEnable: (Bool) -> Void
    let onMove: (Int) -> Void  // -1 上移, +1 下移

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

            // per-window checkboxes (引用了未触发餐的 window 隐藏 — 等触发后浮现)
            if item.windows.isEmpty {
                Text("尚无时段")
                    .foregroundStyle(.tertiary)
            } else {
                let visibleWindows = item.windows.enumerated().filter { _, w in
                    w.requiresMeal.isEmpty
                }
                if visibleWindows.isEmpty {
                    let needs = Set(item.windows.compactMap { $0.requiresMeal.isEmpty ? nil : $0.requiresMeal })
                    Label("等待触发: \(needs.sorted().joined(separator: " / "))", systemImage: "clock")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                } else {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 170), spacing: 10)], spacing: 10) {
                        ForEach(visibleWindows.map(\.offset), id: \.self) { idx in
                            WindowChip(window: item.windows[idx], disabled: item.paused || !item.enabled) {
                                onToggleWindow(idx)
                            }
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
        } else if !pendingMeals.isEmpty {
            Chip(text: "等待 \(pendingMeals.joined(separator: " / "))", color: .orange)
        } else if item.doneToday {
            Chip(text: "今日已勾完", color: .green)
        }
    }

    /// 暂停 menu 的状态化 icon / 颜色 / hover 提示 — 把"永久暂停 (disable)" 和
    /// "暂停 N 天 (pausedUntil)" 合在同一菜单, 用 icon 反映当前状态:
    ///   - !enabled → playpause.circle 红, 提示已永久暂停 (停用)
    ///   - paused   → play.circle 橙, 提示限时暂停中
    ///   - 正常     → pause.circle 灰, 提示点击暂停 / 停用
    private var pauseIcon: String {
        if !item.enabled { return "playpause.circle" }
        return item.paused ? "play.circle" : "pause.circle"
    }
    private var pauseColor: Color {
        if !item.enabled { return .red }
        return item.paused ? .orange : .secondary
    }
    private var pauseHelp: String {
        if !item.enabled { return "已永久暂停 (停用), 点击启用 / 限时暂停" }
        if item.paused { return "限时暂停中, 点击调整 / 取消 / 永久暂停" }
        return "暂停 N 天 / 永久暂停"
    }

    /// 该 item 还在等待哪些餐被触发(如 ["早餐", "午餐"])。
    /// 优先级高于 doneToday,避免还没吃饭就误显示"已完成"。
    private var pendingMeals: [String] {
        let labels: [String: String] = ["breakfast": "早餐", "lunch": "午餐", "dinner": "晚餐"]
        let set = Set(item.windows.compactMap { $0.requiresMeal.isEmpty ? nil : $0.requiresMeal })
        return set.sorted().compactMap { labels[$0] }
    }

    private var actions: some View {
        HStack(spacing: 4) {
            Menu {
                Button("暂停 1 天") { onPause(1) }
                Button("暂停 7 天") { onPause(7) }
                Button("暂停 14 天") { onPause(14) }
                Button("暂停 30 天") { onPause(30) }
                Divider()
                if item.enabled {
                    Button("永久暂停 (停用)") { onEnable(false) }
                } else {
                    Button("启用") { onEnable(true) }
                }
                if item.paused {
                    Button("取消时限暂停") { onPause(0) }
                }
            } label: {
                Image(systemName: pauseIcon)
                    .foregroundStyle(pauseColor)
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .frame(width: 36)
            .help(pauseHelp)

            Button(action: onEdit) {
                Image(systemName: "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("编辑 (复制 / 删除在 form 内)")

            Button { onMove(-1) } label: {
                Image(systemName: "chevron.up")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .disabled(isFirst)
            .help("上移")

            Button { onMove(1) } label: {
                Image(systemName: "chevron.down")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .disabled(isLast)
            .help("下移")
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
        if window.doneToday { return "✓ 已完成" }
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
        guard w.requiresMeal.isEmpty else { return nil }   // 未触发餐 — 不画
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
