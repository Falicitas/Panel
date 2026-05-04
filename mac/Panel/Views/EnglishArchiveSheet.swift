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
    }

    @ViewBuilder
    private func archiveRow(audio: EnglishAudio) -> some View {
        VStack(alignment: .leading, spacing: 6) {
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
                Spacer()
            }
            .contentShape(Rectangle())
            .padding(.vertical, 8)
            .padding(.horizontal, 16)
            .onTapGesture { Task { await toggleExpand(audio) } }

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
