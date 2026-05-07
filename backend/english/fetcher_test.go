package english

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"panel/store"
)

// fakeFetcher 实现 Fetcher 接口, 测试 RunFetchAll 的过滤逻辑。
// DownloadAudio 写 sparse 文件 (size = item.AudioURL 末尾的 ?size=N 提示, 默认 0)。
// 用 sparse Truncate 让 size 任意大不占盘。
type fakeFetcher struct {
	name      string
	items     []AudioMeta
	writeSize map[string]int64 // ref → 期望写出的文件 size (sparse)
}

func (f *fakeFetcher) Name() string { return f.name }

func (f *fakeFetcher) Fetch(_ context.Context) ([]AudioMeta, error) {
	return f.items, nil
}

func (f *fakeFetcher) DownloadAudio(_ context.Context, audioURL, destPath string) (int64, error) {
	out, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	// 找 audioURL 对应的 ref, lookup writeSize map (用 ref 找比 url 直观)
	var ref string
	for _, m := range f.items {
		if m.AudioURL == audioURL {
			ref = m.SourceRef
			break
		}
	}
	size := int64(0)
	if f.writeSize != nil {
		size = f.writeSize[ref]
	}
	if size > 0 {
		if err := out.Truncate(size); err != nil {
			return 0, err
		}
	}
	return size, nil
}

func TestRunFetchAll_DurationCap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	// 准备 5 条 audio: [0(=未知, 接受), 5s(<30s 拒), 600s(OK), 1800s(>25min 拒), 3min(OK)]
	items := []AudioMeta{
		{Source: "fake", SourceRef: "ref-0",   Title: "T0", PublishedAt: 1700000100, DurationMs: 0,         AudioURL: "http://example.test/0.mp3", Topic: "general"},
		{Source: "fake", SourceRef: "ref-5s",  Title: "T5", PublishedAt: 1700000200, DurationMs: 5_000,     AudioURL: "http://example.test/5.mp3", Topic: "general"},
		{Source: "fake", SourceRef: "ref-10m", Title: "T10", PublishedAt: 1700000300, DurationMs: 600_000,  AudioURL: "http://example.test/10.mp3", Topic: "general"},
		{Source: "fake", SourceRef: "ref-30m", Title: "T30", PublishedAt: 1700000400, DurationMs: 1_800_000, AudioURL: "http://example.test/30.mp3", Topic: "general"},
		{Source: "fake", SourceRef: "ref-3m",  Title: "T3",  PublishedAt: 1700000500, DurationMs: 180_000,  AudioURL: "http://example.test/3.mp3", Topic: "general"},
	}
	f := &fakeFetcher{name: "fake", items: items}

	if err := RunFetchAll(context.Background(), s, []Fetcher{f}, dir, 0, 10); err != nil {
		t.Fatalf("RunFetchAll: %v", err)
	}

	got, err := s.ListAudios(false, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	gotRefs := make(map[string]bool, len(got))
	for _, a := range got {
		gotRefs[a.SourceRef] = true
	}

	wantIn := []string{"ref-0", "ref-10m", "ref-3m"}
	wantOut := []string{"ref-5s", "ref-30m"}
	for _, ref := range wantIn {
		if !gotRefs[ref] {
			t.Errorf("missing %q in db (should be inserted, dur in range)", ref)
		}
	}
	for _, ref := range wantOut {
		if gotRefs[ref] {
			t.Errorf("unexpected %q in db (should be skipped, out of [30s, 25min])", ref)
		}
	}
	if len(got) != len(wantIn) {
		t.Errorf("got %d audios, want %d (refs=%v)", len(got), len(wantIn), gotRefs)
	}
}

// TestRunFetchAll_SizeCap 沉淀 Sciam 24min/192kbps = 33MB 的真实 regression:
// duration 在 [30s, 25min] cap 内, 但 size > 25MB 仍触发 segments_status=failed.
func TestRunFetchAll_SizeCap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	// 2 条 audio (duration 都在 cap 内, 但模拟下载后 size 不同):
	//   small      1MB → 过 size cap → energy 短文件分支 (decode-fail fail-open) → pending
	//   sciam-33   33MB → size cap 拒 → 不跑 energy → status=failed
	// 边界 case (25MB) 已在 ValidateAudioSize 单测覆盖, 这里不重复跑慢的 energy decode。
	items := []AudioMeta{
		{Source: "fake", SourceRef: "small",    Title: "T1", PublishedAt: 1700000100, DurationMs: 600_000, AudioURL: "http://example.test/1.mp3", Topic: "general"},
		{Source: "fake", SourceRef: "sciam-33", Title: "T3", PublishedAt: 1700000200, DurationMs: 1_400_000, AudioURL: "http://example.test/3.mp3", Topic: "general"},
	}
	f := &fakeFetcher{
		name:  "fake",
		items: items,
		writeSize: map[string]int64{
			"small":    1 * 1024 * 1024,
			"sciam-33": 33 * 1024 * 1024,
		},
	}

	if err := RunFetchAll(context.Background(), s, []Fetcher{f}, dir, 0, 10); err != nil {
		t.Fatalf("RunFetchAll: %v", err)
	}

	got, err := s.ListAudios(false, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byRef := make(map[string]store.EnglishAudio, len(got))
	for _, a := range got {
		byRef[a.SourceRef] = a
	}
	if len(byRef) != 2 {
		t.Fatalf("expected 2 rows in db (all duration-valid), got %d (%v)", len(byRef), byRef)
	}

	if a := byRef["small"]; a.SegmentsStatus != store.EnglishStatusPending {
		t.Errorf("small: status=%q, want pending", a.SegmentsStatus)
	}
	a := byRef["sciam-33"]
	if a.SegmentsStatus != store.EnglishStatusFailed {
		t.Errorf("sciam-33: status=%q, want failed (33MB > 25MB)", a.SegmentsStatus)
	}
	if !strings.Contains(a.SegmentsFailedReason, "too large") {
		t.Errorf("sciam-33 reason=%q, want contains 'too large'", a.SegmentsFailedReason)
	}
}
