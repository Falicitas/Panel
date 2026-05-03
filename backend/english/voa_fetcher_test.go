//go:build integration

// Integration test: 真实联网拉 VOA RSS + 下载单条 mp3。
// 跑法: go test -tags integration -v -run TestVOAFetcher ./english/
// 默认 go test 不跑 (没有 -tags integration)。
package english

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panel/store"
)

func TestVOAFetcher_FetchListsItems(t *testing.T) {
	f := NewVOAFetcher()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	items, err := f.Fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("got 0 items, expected ≥ 1")
	}
	t.Logf("fetched %d items", len(items))

	first := items[0]
	if first.Source != "voa" {
		t.Errorf("source = %q, want voa", first.Source)
	}
	if first.SourceRef == "" {
		t.Errorf("source_ref empty")
	}
	if first.AudioURL == "" {
		t.Errorf("audio_url empty")
	}
	if !strings.HasSuffix(first.AudioURL, ".mp3") {
		t.Errorf("audio_url not mp3: %q", first.AudioURL)
	}
	if first.PublishedAt == 0 {
		t.Errorf("published_at = 0 (pubDate parse failed?)")
	}
	if first.DurationMs == 0 {
		t.Errorf("duration_ms = 0 (itunes:duration parse failed?)")
	}
	t.Logf("first item: title=%q ref=%q dur=%dms url=%s",
		first.Title, first.SourceRef, first.DurationMs, first.AudioURL)
}

func TestVOAFetcher_DownloadOneAudio(t *testing.T) {
	tmpDir := t.TempDir()
	englishDir := filepath.Join(tmpDir, "english")
	if err := os.MkdirAll(englishDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f := NewVOAFetcher()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	items, err := f.Fetch(ctx)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(items) == 0 {
		t.Skip("no items in feed")
	}

	first := items[0]
	dest := filepath.Join(englishDir, "test.mp3")
	n, err := f.DownloadAudio(ctx, first.AudioURL, dest)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if n < 100*1024 { // < 100KB 太小, 必有问题 (VOA 节目通常 5-30MB)
		t.Errorf("downloaded %d bytes, suspiciously small", n)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Size() != n {
		t.Errorf("file size %d != return value %d", info.Size(), n)
	}
	t.Logf("downloaded %d bytes -> %s", n, dest)

	// 第二次同 destPath 应跳过 (返回 0, nil)
	n2, err := f.DownloadAudio(ctx, first.AudioURL, dest)
	if err != nil {
		t.Errorf("second download: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second download returned %d, expected 0 (skip)", n2)
	}
}

func TestRunFetchAll_LimitsDownloads(t *testing.T) {
	tmpDir := t.TempDir()
	englishDir := filepath.Join(tmpDir, "english")
	dbPath := filepath.Join(tmpDir, "panel.db")
	if err := os.MkdirAll(englishDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// 限额 1 — 只下载 1 条 mp3, 但所有 metadata 都该入库
	if err := RunFetchAll(ctx, st, []Fetcher{NewVOAFetcher()}, englishDir, 100*time.Millisecond, 1); err != nil {
		t.Fatalf("RunFetchAll: %v", err)
	}

	audios, err := st.ListAudios(false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(audios) == 0 {
		t.Fatal("got 0 audios in db, expected ≥ 1")
	}
	t.Logf("persisted %d audio metadata rows", len(audios))

	withLocal := 0
	for _, a := range audios {
		if a.LocalPath != "" {
			withLocal++
			info, err := os.Stat(a.LocalPath)
			if err != nil {
				t.Errorf("stat %s: %v", a.LocalPath, err)
			} else {
				t.Logf("downloaded id=%d size=%d %s", a.ID, info.Size(), a.LocalPath)
			}
		}
	}
	if withLocal != 1 {
		t.Errorf("with local_path = %d, want 1 (limit was 1)", withLocal)
	}
	if withLocal > len(audios) {
		t.Errorf("withLocal %d > total %d (impossible)", withLocal, len(audios))
	}
	fmt.Printf("=== summary: %d audios in db, %d with local mp3 ===\n", len(audios), withLocal)
}
