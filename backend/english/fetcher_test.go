package english

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"panel/store"
)

// fakeFetcher 实现 Fetcher 接口, 测试 RunFetchAll 的长度 cap 过滤逻辑。
// DownloadAudio 写空文件占位 (ValidateAudioEnergy 对空文件 decode 失败 → fail-open 返 nil)。
type fakeFetcher struct {
	name  string
	items []AudioMeta
}

func (f *fakeFetcher) Name() string { return f.name }

func (f *fakeFetcher) Fetch(_ context.Context) ([]AudioMeta, error) {
	return f.items, nil
}

func (f *fakeFetcher) DownloadAudio(_ context.Context, _, destPath string) (int64, error) {
	out, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	return 0, out.Close()
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
