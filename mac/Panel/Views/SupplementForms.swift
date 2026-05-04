import SwiftUI

extension Supplement {
    static var blank: Supplement {
        Supplement(
            id: 0,
            name: "",
            type: "supplement",
            windows: [],
            notes: "",
            dose: "",
            enabled: true,
            pausedUntil: nil
        )
    }
}

struct SupplementFormSheet: View {
    let initial: Supplement
    let isNew: Bool
    /// 仅 isNew=false 时挂: 用户点'复制'按钮 → 把当前 form 状态 (含未保存修改) 抛回外层,
    /// 外层 dismiss 当前 sheet 并弹新 sheet (isNew=true) 让用户改名保存。
    var onDuplicate: ((Supplement) -> Void)? = nil

    @Environment(AppState.self) private var state
    @Environment(\.dismiss) private var dismiss
    @State private var item: Supplement = .blank
    @State private var confirmingDelete = false

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text(isNew ? "新增食物 / 保健品" : "编辑")
                    .font(.headline)
                Spacer()
                Button("取消") { dismiss() }
                Button("保存") {
                    Task {
                        await state.saveSupplement(item, isNew: isNew)
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
                        Text("名称").frame(width: 70, alignment: .leading)
                        TextField("维生素 C / 早餐 / ...", text: $item.name)
                            .textFieldStyle(.roundedBorder)
                    }
                    HStack {
                        Text("类型").frame(width: 70, alignment: .leading)
                        Picker("", selection: $item.type) {
                            Text("食物").tag("food")
                            Text("保健品").tag("supplement")
                        }
                        .pickerStyle(.segmented)
                        .labelsHidden()
                    }

                    HStack {
                        Text("剂量").frame(width: 70, alignment: .leading)
                        TextField("1 颗 / 5 滴 / 30g 等 (可空)", text: $item.dose)
                            .textFieldStyle(.roundedBorder)
                    }

                    HStack {
                        Text("启用").frame(width: 70, alignment: .leading)
                        Toggle(isOn: $item.enabled) {
                            Text(item.enabled ? "通知正常,统计计入" : "停用 (通知不弹,但保留历史)")
                                .font(.caption)
                                .foregroundStyle(item.enabled ? Color.secondary : Color.red)
                        }
                        .toggleStyle(.switch)
                        Spacer()
                    }

                    if let until = item.pausedUntil, until > Int64(Date().timeIntervalSince1970) {
                        HStack {
                            Text("暂停").frame(width: 70, alignment: .leading)
                            Text("至 " + pausedUntilLabel(until))
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.orange)
                            Spacer()
                            Button("取消暂停") { item.pausedUntil = nil }
                                .buttonStyle(.borderless)
                        }
                    }

                    VStack(alignment: .leading, spacing: 8) {
                        HStack {
                            Text("时段").frame(width: 70, alignment: .leading)
                            Spacer()
                            Button {
                                item.windows.append(SupplementWindow(start: "08:00", end: "10:00"))
                            } label: {
                                Label("添加时段", systemImage: "plus.circle")
                            }
                            .buttonStyle(.borderless)
                        }
                        if item.windows.isEmpty {
                            Text("尚无时段(无时段则不会触发提醒)")
                                .font(.caption).foregroundStyle(.tertiary)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(.leading, 70)
                        } else {
                            ForEach(Array(item.windows.enumerated()), id: \.offset) { idx, _ in
                                WindowRow(
                                    start: $item.windows[idx].start,
                                    end: $item.windows[idx].end,
                                    onDelete: { item.windows.remove(at: idx) }
                                )
                            }
                            Text("时段表达式: HH:MM 或 wake / wake+Nh / wake-Nm")
                                .font(.caption2).foregroundStyle(.tertiary)
                                .padding(.leading, 70)
                            if !overlapPairs.isEmpty {
                                ForEach(overlapPairs) { pair in
                                    Label(
                                        "时段重叠:\(item.windows[pair.i].start)–\(item.windows[pair.i].end) 与 \(item.windows[pair.j].start)–\(item.windows[pair.j].end)",
                                        systemImage: "exclamationmark.triangle.fill"
                                    )
                                    .font(.caption)
                                    .foregroundStyle(.red)
                                    .padding(.leading, 70)
                                }
                            }
                        }
                    }

                    HStack(alignment: .top) {
                        Text("备注").frame(width: 70, alignment: .leading)
                        TextEditor(text: $item.notes)
                            .frame(minHeight: 60, maxHeight: 100)
                            .border(.tertiary.opacity(0.3))
                    }
                }
                .padding()
            }

            if !isNew {
                Divider()
                HStack {
                    Spacer()
                    Button {
                        onDuplicate?(item)
                        dismiss()
                    } label: {
                        Label("复制", systemImage: "doc.on.doc")
                    }
                    .help("用当前设置 (含未保存编辑) 创建新条目, 改名保存")
                    Button(role: .destructive) {
                        confirmingDelete = true
                    } label: {
                        Label("删除", systemImage: "trash")
                    }
                    .confirmationDialog("确认删除?", isPresented: $confirmingDelete) {
                        Button("删除", role: .destructive) {
                            Task {
                                await state.deleteSupplement(id: initial.id)
                                dismiss()
                            }
                        }
                        Button("取消", role: .cancel) {}
                    }
                }
                .padding()
            }
        }
        .frame(minWidth: 520, minHeight: 460)
        .onAppear { item = initial }
    }

    private var canSave: Bool {
        !item.name.trimmingCharacters(in: .whitespaces).isEmpty
            && (item.type == "food" || item.type == "supplement")
            && item.windows.allSatisfy {
                !$0.start.trimmingCharacters(in: .whitespaces).isEmpty
                    && !$0.end.trimmingCharacters(in: .whitespaces).isEmpty
            }
            && overlapPairs.isEmpty
    }

    struct OverlapPair: Identifiable, Hashable {
        let i: Int
        let j: Int
        var id: String { "\(i)-\(j)" }
    }

    /// 同项内 windows 两两 HH:MM 比较 (wake-相对的跳过) — 返回所有相交对。
    private var overlapPairs: [OverlapPair] {
        var out: [OverlapPair] = []
        let n = item.windows.count
        for i in 0..<n {
            for j in (i + 1)..<n {
                if Self.overlaps(item.windows[i], item.windows[j]) {
                    out.append(OverlapPair(i: i, j: j))
                }
            }
        }
        return out
    }

    /// 两窗口 HH:MM 是否相交 (左闭右开;wake 相对/解析失败 跳过 → 视作不交)。
    static func overlaps(_ a: SupplementWindow, _ b: SupplementWindow) -> Bool {
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

    private func pausedUntilLabel(_ unix: Int64) -> String {
        let f = DateFormatter()
        f.dateFormat = "M月d日 HH:mm"
        return f.string(from: Date(timeIntervalSince1970: TimeInterval(unix)))
    }
}

private struct WindowRow: View {
    @Binding var start: String
    @Binding var end: String
    let onDelete: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Spacer().frame(width: 70 - 8)
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
