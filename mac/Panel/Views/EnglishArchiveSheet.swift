import SwiftUI

/// 历史档案 sheet — 列出所有已归档音频, 点击展开看 segments, 每段 ▶ 直接重听。
/// 不动 db / records / archived_at, 纯只读浏览 + 复用 SegmentPlayer 播放。
struct EnglishArchiveSheet: View {
    @Binding var isPresented: Bool
    @Environment(SegmentPlayer.self) private var player

    @State private var audios: [EnglishAudio] = []
    @State private var loadError: String? = nil
    @State private var loading: Bool = true
    @State private var expandedID: Int64? = nil
    @State private var expandedSegments: [EnglishSegment] = []
    @State private var loadingSegments: Bool = false
    @State private var pendingDelete: EnglishAudio? = nil
    @State private var deletingID: Int64? = nil
    @State private var deleteError: String? = nil

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("历史档案")
                    .font(.title2.weight(.semibold))
                Spacer()
                Button("关闭") { isPresented = false }
                    .keyboardShortcut(.escape, modifiers: [])
            }
            .padding()

            Divider()

            if loading {
                Spacer()
                ProgressView("加载中…")
                Spacer()
            } else if let err = loadError {
                Spacer()
                Text("加载失败: \(err)").foregroundStyle(.red)
                Spacer()
            } else if audios.isEmpty {
                Spacer()
                Text("还没有归档音频")
                    .foregroundStyle(.secondary)
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 0) {
                        ForEach(audios) { audio in
                            archiveRow(audio: audio)
                            Divider()
                        }
                    }
                }
            }
        }
        .frame(width: 720, height: 600)
        .task { await loadArchive() }
        .confirmationDialog(
            pendingDelete.map { "永久删除「\($0.title)」?" } ?? "永久删除?",
            isPresented: Binding(
                get: { pendingDelete != nil },
                set: { if !$0 { pendingDelete = nil } }
            ),
            titleVisibility: .visible,
            presenting: pendingDelete
        ) { audio in
            Button("永久删除", role: .destructive) {
                Task { await deleteAudio(audio) }
            }
            Button("取消", role: .cancel) {}
        } message: { _ in
            Text("音频文件和字幕将被彻底清除, 已积累的学习时长保留。此操作不可恢复。")
        }
        .alert(
            "删除失败",
            isPresented: Binding(
                get: { deleteError != nil },
                set: { if !$0 { deleteError = nil } }
            ),
            presenting: deleteError
        ) { _ in
            Button("好") { deleteError = nil }
        } message: { msg in
            Text(msg)
        }
    }

    @ViewBuilder
    private func archiveRow(audio: EnglishAudio) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .top, spacing: 10) {
                HStack(alignment: .top, spacing: 10) {
                    Image(systemName: expandedID == audio.id ? "chevron.down" : "chevron.right")
                        .font(.system(size: 12))
                        .foregroundStyle(.secondary)
                        .frame(width: 14)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(audio.title)
                            .font(.body.weight(.medium))
                            .lineLimit(2)
                        HStack(spacing: 10) {
                            Text("[\(audio.source.uppercased())]")
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                            Text("时长 \(formatMs(audio.durationMs))")
                                .font(.caption).foregroundStyle(.secondary)
                            if let archived = audio.archivedAt {
                                Text("归档 \(formatUnix(archived))")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                        }
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
                .onTapGesture { Task { await toggleExpand(audio) } }
                deleteButton(for: audio)
            }
            .padding(.vertical, 8)
            .padding(.horizontal, 16)

            if expandedID == audio.id {
                if loadingSegments {
                    HStack(spacing: 6) {
                        ProgressView().controlSize(.small)
                        Text("加载段落…").font(.caption).foregroundStyle(.secondary)
                    }
                    .padding(.leading, 40).padding(.bottom, 8)
                } else if expandedSegments.isEmpty {
                    Text("无段落数据").font(.caption).foregroundStyle(.secondary)
                        .padding(.leading, 40).padding(.bottom, 8)
                } else {
                    VStack(alignment: .leading, spacing: 4) {
                        ForEach(expandedSegments) { seg in
                            archiveSegmentRow(audio: audio, segment: seg)
                        }
                    }
                    .padding(.leading, 32).padding(.trailing, 16).padding(.bottom, 8)
                }
            }
        }
    }

    @ViewBuilder
    private func deleteButton(for audio: EnglishAudio) -> some View {
        Button {
            pendingDelete = audio
        } label: {
            if deletingID == audio.id {
                ProgressView().controlSize(.small)
            } else {
                Image(systemName: "trash")
                    .font(.system(size: 13))
                    .foregroundStyle(.secondary)
            }
        }
        .buttonStyle(.plain)
        .disabled(deletingID != nil)
        .help("永久删除")
    }

    @ViewBuilder
    private func archiveSegmentRow(audio: EnglishAudio, segment: EnglishSegment) -> some View {
        let playing = player.isPlaying(audioID: audio.id, segIdx: segment.idx)
        HStack(alignment: .top, spacing: 10) {
            Button {
                togglePlay(audio: audio, segment: segment, playing: playing)
            } label: {
                Image(systemName: playing ? "stop.circle.fill" : "play.circle")
                    .font(.system(size: 18))
                    .foregroundStyle(playing ? Color.orange : Color.accentColor)
            }
            .buttonStyle(.plain)
            .disabled(audio.localPath.isEmpty)

            Text("\(segment.idx + 1).")
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(width: 28, alignment: .trailing)
            Text(segment.text)
                .font(.callout)
                .frame(maxWidth: .infinity, alignment: .leading)
                .textSelection(.enabled)
        }
        .padding(.vertical, 2)
    }

    private func togglePlay(audio: EnglishAudio, segment: EnglishSegment, playing: Bool) {
        if playing {
            player.stop()
            return
        }
        let url = URL(fileURLWithPath: audio.localPath)
        do {
            try player.play(
                url: url,
                audioID: audio.id,
                segIdx: segment.idx,
                startMs: Int(segment.startMs),
                endMs: Int(segment.endMs)
            )
        } catch {
            // silent — UI 按钮不变即说明未起播
        }
    }

    private func toggleExpand(_ audio: EnglishAudio) async {
        if expandedID == audio.id {
            expandedID = nil
            expandedSegments = []
            return
        }
        expandedID = audio.id
        expandedSegments = []
        loadingSegments = true
        defer { loadingSegments = false }
        do {
            let detail = try await APIClient.shared.getEnglishAudio(id: audio.id)
            if expandedID == audio.id {
                expandedSegments = detail.segments
            }
        } catch {
            // silent
        }
    }

    private func deleteAudio(_ audio: EnglishAudio) async {
        deletingID = audio.id
        defer { deletingID = nil }
        // 在播这条 audio 的任意 segment 时先 stop, 避免 mp3 文件被后端删除后 player 拿到陈数据。
        if player.playingAudioID == audio.id {
            player.stop()
        }
        do {
            try await APIClient.shared.deleteEnglishAudio(id: audio.id)
            audios.removeAll { $0.id == audio.id }
            if expandedID == audio.id {
                expandedID = nil
                expandedSegments = []
            }
        } catch {
            deleteError = error.localizedDescription
        }
    }

    private func loadArchive() async {
        loading = true
        defer { loading = false }
        do {
            audios = try await APIClient.shared.listEnglishAudios(archived: true)
            audios.sort { ($0.archivedAt ?? 0) > ($1.archivedAt ?? 0) }
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }

    private func formatMs(_ ms: Int64) -> String {
        let total = Int(ms / 1000)
        let m = total / 60
        let s = total % 60
        return String(format: "%d:%02d", m, s)
    }

    private func formatUnix(_ unix: Int64) -> String {
        let d = Date(timeIntervalSince1970: TimeInterval(unix))
        let f = DateFormatter()
        f.dateFormat = "MM-dd HH:mm"
        return f.string(from: d)
    }
}
