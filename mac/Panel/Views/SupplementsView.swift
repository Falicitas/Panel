import SwiftUI

/// 食物 / 保健品统一大版面 (合并原 ContentView + DetailView,2 列布局)。
/// Sections:
///   - 头部:filter + 搜索 + 新增
///   - 今日时间线 (横条,过去/现在/将来三色)
///   - 现在该做 (active windows + 勾选)
///   - 全部清单 (勾选 / 暂停 / 启用 / 编辑 / 删除)
struct SupplementsView: View {
    @Environment(AppState.self) private var state
    @State private var filter: SupplementsFilter = .all
    @State private var search: String = ""
    @State private var creating = false
    @State private var editing: Supplement?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Header(filter: $filter, search: $search, onAdd: { creating = true })
                TodayTimelineCard(today: state.supplementsToday)
                ActiveNowSection(today: state.supplementsToday, filter: filter, search: search) { item in
                    Task { await state.toggleSupplementCheck(item) }
                }
                AllListSection(
                    today: state.supplementsToday,
                    supplementsByID: supplementsByID,
                    filter: filter,
                    search: search,
                    onCheck: { item in Task { await state.toggleSupplementCheck(item) } },
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
    let onAdd: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Picker("", selection: $filter) {
                ForEach(SupplementsFilter.allCases) { t in
                    Text(t.label).tag(t)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .frame(maxWidth: 240)

            TextField("搜索 (名称 / 备注)", text: $search)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 280)

            Spacer()

            Button(action: onAdd) {
                Label("新增", systemImage: "plus")
            }
            .buttonStyle(.borderedProminent)
        }
    }
}

// MARK: - 今日时间线

private struct TodayTimelineCard: View {
    let today: SupplementToday?
    @Environment(AppState.self) private var state

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 8) {
                Image(systemName: "calendar.day.timeline.left")
                    .foregroundStyle(Color.anchorAccent)
                Text("今日时间线")
                    .font(.headline)
                Spacer()
                if let t = today {
                    Text("已勾 \(t.doneCount)/\(t.totalActive) · 当前在窗 \(t.activeCount)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            TimelineStrip(today: today)
                .frame(height: 56)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
    }
}

/// 横条:6:00 → 22:00,所有 active 窗口画成色块,颜色按 status。
private struct TimelineStrip: View {
    let today: SupplementToday?
    @Environment(AppState.self) private var state

    private static let dayStartMin = 6 * 60   // 06:00
    private static let dayEndMin = 22 * 60    // 22:00 (16h 区间)

    var body: some View {
        let _ = state.timeTick   // 跟着 tick 重画 now 标尺
        GeometryReader { geo in
            let w = geo.size.width
            ZStack(alignment: .topLeading) {
                // 底条
                Capsule()
                    .fill(Color.secondary.opacity(0.12))
                    .frame(height: 18)
                    .padding(.top, 18)

                // 窗口色块
                ForEach(allWindowSegments) { seg in
                    Capsule()
                        .fill(color(for: seg.status).opacity(0.7))
                        .frame(width: max(2, w * seg.widthFrac), height: 18)
                        .offset(x: w * seg.startFrac, y: 18)
                        .help("\(seg.name) · \(seg.startHM)–\(seg.endHM)")
                }

                // 现在
                let nowF = nowFrac()
                if (0...1).contains(nowF) {
                    Rectangle()
                        .fill(Color.anchorAccent)
                        .frame(width: 2, height: 32)
                        .offset(x: w * nowF, y: 11)
                }

                // 刻度: 6 / 10 / 14 / 18 / 22
                ForEach([6, 10, 14, 18, 22], id: \.self) { hour in
                    Text(String(format: "%02d", hour))
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.tertiary)
                        .offset(x: w * hourFrac(hour) - 8, y: 0)
                }
            }
        }
    }

    private struct Segment: Identifiable {
        let id: String
        let name: String
        let startHM: String
        let endHM: String
        let startFrac: Double
        let widthFrac: Double
        let status: String
    }

    private var allWindowSegments: [Segment] {
        guard let items = today?.items else { return [] }
        var out: [Segment] = []
        for it in items where it.enabled && !it.paused {
            for (idx, w) in it.windows.enumerated() {
                guard let s = parseHM(w.startHM), let e = parseHM(w.endHM), e > s else { continue }
                let sf = fracFor(s)
                let ef = fracFor(e)
                guard ef > sf else { continue }
                out.append(.init(
                    id: "\(it.id):\(idx)",
                    name: it.name,
                    startHM: w.startHM,
                    endHM: w.endHM,
                    startFrac: max(0, sf),
                    widthFrac: min(1, ef) - max(0, sf),
                    status: w.status
                ))
            }
        }
        return out
    }

    private func parseHM(_ s: String) -> Int? {
        let parts = s.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]) else { return nil }
        return h * 60 + m
    }

    private func fracFor(_ minutes: Int) -> Double {
        let span = Double(Self.dayEndMin - Self.dayStartMin)
        return max(0, min(1, (Double(minutes) - Double(Self.dayStartMin)) / span))
    }

    private func hourFrac(_ hour: Int) -> Double { fracFor(hour * 60) }

    private func nowFrac() -> Double {
        let comps = Calendar.current.dateComponents([.hour, .minute], from: Date())
        let mins = (comps.hour ?? 0) * 60 + (comps.minute ?? 0)
        return fracFor(mins)
    }

    private func color(for status: String) -> Color {
        switch status {
        case "active":   return Color.anchorAccent
        case "passed":   return Color.green
        case "upcoming": return Color.secondary
        default:         return Color.gray
        }
    }
}

// MARK: - 现在该做

private struct ActiveNowSection: View {
    let today: SupplementToday?
    let filter: SupplementsFilter
    let search: String
    let onCheck: (SupplementTodayItem) -> Void

    var body: some View {
        let items = activeItems
        if !items.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 6) {
                    Image(systemName: "sun.haze.fill").foregroundStyle(Color.anchorAccent)
                    Text("现在该做")
                        .font(.headline)
                    Text("\(items.count) 项")
                        .font(.caption).foregroundStyle(.tertiary)
                }
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 220), spacing: 12)], spacing: 12) {
                    ForEach(items) { it in
                        ActiveCard(item: it, onCheck: { onCheck(it) })
                    }
                }
            }
        }
    }

    private var activeItems: [SupplementTodayItem] {
        guard let t = today else { return [] }
        return t.items.filter { it in
            it.enabled && !it.paused && it.hasActive
                && matchesFilter(it.type)
                && matchesSearch(it.name, it.notes)
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

private struct ActiveCard: View {
    let item: SupplementTodayItem
    let onCheck: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Button(action: onCheck) {
                Image(systemName: item.doneToday ? "checkmark.square.fill" : "square")
                    .font(.title2)
                    .foregroundStyle(item.doneToday ? Color.green : Color.anchorAccent)
            }
            .buttonStyle(.plain)
            .help(item.doneToday ? "取消今天勾选" : "标记今天已吃")

            VStack(alignment: .leading, spacing: 2) {
                Text(item.name)
                    .font(.body.weight(.medium))
                    .strikethrough(item.doneToday, color: .secondary)
                    .foregroundStyle(item.doneToday ? .secondary : .primary)
                if !item.dose.isEmpty {
                    Text(item.dose)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
                if let active = item.windows.first(where: { $0.status == "active" }) {
                    Text("\(active.startHM)–\(active.endHM)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(Color.anchorAccent)
                }
            }
            Spacer()
        }
        .padding(12)
        .background(Color.anchorAccent.opacity(item.doneToday ? 0.04 : 0.10),
                    in: RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(Color.anchorAccent.opacity(0.35), lineWidth: 1)
        )
    }
}

// MARK: - 全部清单

private struct AllListSection: View {
    let today: SupplementToday?
    let supplementsByID: [Int64: Supplement]
    let filter: SupplementsFilter
    let search: String
    let onCheck: (SupplementTodayItem) -> Void
    let onEdit: (Supplement) -> Void
    let onPause: (Supplement, Int) -> Void
    let onEnable: (Supplement, Bool) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 6) {
                Image(systemName: "list.bullet.rectangle")
                    .foregroundStyle(.secondary)
                Text("全部清单")
                    .font(.headline)
                Text("\(filteredItems.count) 项")
                    .font(.caption).foregroundStyle(.tertiary)
            }
            if filteredItems.isEmpty {
                ContentUnavailableView {
                    Label("没有匹配的项", systemImage: "magnifyingglass")
                } description: {
                    Text("调整 filter / 搜索词,或右上 + 新增")
                }
                .frame(maxWidth: .infinity, minHeight: 200)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
            } else {
                VStack(spacing: 8) {
                    ForEach(filteredItems) { it in
                        if let sp = supplementsByID[it.id] {
                            ItemRow(
                                item: it, sp: sp,
                                onCheck: { onCheck(it) },
                                onEdit: { onEdit(sp) },
                                onPause: { days in onPause(sp, days) },
                                onEnable: { on in onEnable(sp, on) }
                            )
                        }
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

private struct ItemRow: View {
    let item: SupplementTodayItem
    let sp: Supplement
    let onCheck: () -> Void
    let onEdit: () -> Void
    let onPause: (Int) -> Void
    let onEnable: (Bool) -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Button(action: onCheck) {
                Image(systemName: item.doneToday ? "checkmark.square.fill" : "square")
                    .font(.title3)
                    .foregroundStyle(item.doneToday ? Color.green : .secondary)
            }
            .buttonStyle(.plain)
            .help(item.doneToday ? "取消今天勾选" : "标记今天已吃")
            .disabled(item.paused || !item.enabled)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Image(systemName: item.type == "supplement" ? "pills" : "leaf")
                        .foregroundStyle(item.type == "supplement" ? Color.accentColor : Color.green)
                    Text(item.name)
                        .font(.body.weight(.medium))
                        .strikethrough(item.doneToday, color: .secondary)
                    if !item.dose.isEmpty {
                        Text("· \(item.dose)")
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(.secondary)
                    }
                    statusChip
                }
                if !item.windows.isEmpty {
                    Text(windowsLabel)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                if !item.notes.isEmpty {
                    Text(item.notes)
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(2)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)

            actions
        }
        .padding(12)
        .background(rowBackground, in: RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(Color.secondary.opacity(0.18), lineWidth: 1)
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
            Chip(text: "今日已勾", color: .green)
        }
    }

    private var actions: some View {
        HStack(spacing: 6) {
            // 暂停 / 取消暂停
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
            .frame(width: 32)
            .help(item.paused ? "暂停中,点击调整 / 取消" : "暂停 N 天")

            // 启用 toggle
            Button {
                onEnable(!item.enabled)
            } label: {
                Image(systemName: item.enabled ? "power" : "power.dotted")
                    .foregroundStyle(item.enabled ? Color.secondary : Color.red)
            }
            .buttonStyle(.borderless)
            .help(item.enabled ? "停用 (通知不弹,历史保留)" : "重新启用")

            // 编辑
            Button(action: onEdit) {
                Image(systemName: "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("编辑")
        }
    }

    private var windowsLabel: String {
        item.windows.map { w in
            let hm = (w.startHM.isEmpty ? w.start : w.startHM) + "–" + (w.endHM.isEmpty ? w.end : w.endHM)
            return hm + statusSuffix(w.status)
        }.joined(separator: " · ")
    }

    private func statusSuffix(_ status: String) -> String {
        switch status {
        case "passed":   return "✓"
        case "active":   return "●"
        case "upcoming": return ""
        default:         return ""
        }
    }

    private func dateLabel(_ unix: Int64) -> String {
        let d = Date(timeIntervalSince1970: TimeInterval(unix))
        let f = DateFormatter()
        f.dateFormat = "M月d日"
        return f.string(from: d)
    }

    private var rowBackground: Color {
        if item.hasActive { return Color.anchorAccent.opacity(0.06) }
        if item.doneToday { return Color.green.opacity(0.05) }
        return Color.clear
    }
}

private struct Chip: View {
    let text: String
    let color: Color
    var body: some View {
        Text(text)
            .font(.caption2)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.18), in: Capsule())
            .foregroundStyle(color)
    }
}
