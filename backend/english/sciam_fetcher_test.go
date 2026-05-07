//go:build integration

// Integration test: 真实联网拉 Sciam Science Quickly RSS + 下载单条 mp3 + 静音检测。
// 跑法 (M5 step 2/3, 用户人耳听样本前):
//   go test -tags integration -v -run TestSciamFetcher ./english/
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
)

func TestSciamFetcher_FetchListsItems(t *testing.T) {
	f := NewSciamFetcher()
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

	// 头 5 条 metadata log 出来给 user 审 (M5 step 2)
	for i, m := range items {
		if i >= 5 {
			break
		}
		fmt.Printf("[%d] title=%q dur=%dms (%.1fmin) ref=%q url=%s\n",
			i+1, m.Title, m.DurationMs, float64(m.DurationMs)/60_000, m.SourceRef, m.AudioURL)
	}

	// 抽检 (走 PRD #10 新源接入流程的字段断言)
	first := items[0]
	if first.Source != "sciam" {
		t.Errorf("source = %q, want sciam", first.Source)
	}
	if first.Topic != "science" {
		t.Errorf("topic = %q, want science", first.Topic)
	}
	if first.SourceRef == "" {
		t.Errorf("source_ref empty")
	}
	if first.AudioURL == "" {
		t.Errorf("audio_url empty")
	}
	if !strings.Contains(first.AudioURL, ".mp3") {
		t.Errorf("audio_url not mp3: %q", first.AudioURL)
	}
	if first.PublishedAt == 0 {
		t.Errorf("published_at = 0 (pubDate parse failed?)")
	}
	if first.DurationMs == 0 {
		t.Errorf("duration_ms = 0 (itunes:duration parse failed?)")
	}
	// 长度合理性 (Sciam Science Quickly 一集 ~5-25min, M3 cap [30s, 25min])
	if first.DurationMs < 30_000 || first.DurationMs > 1_800_000 {
		t.Logf("first item dur=%dms (%.1fmin) — out of expected [30s, 30min] range",
			first.DurationMs, float64(first.DurationMs)/60_000)
	}
}

func TestSciamFetcher_DownloadOne(t *testing.T) {
	tmpDir := t.TempDir()
	englishDir := filepath.Join(tmpDir, "english")
	if err := os.MkdirAll(englishDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f := NewSciamFetcher()
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
	// 给 sample 一个 stable 路径名 (含 source + 时间戳, 方便 user afplay)
	dest := filepath.Join(englishDir, fmt.Sprintf("sciam-sample-%d.mp3", time.Now().Unix()))
	n, err := f.DownloadAudio(ctx, first.AudioURL, dest)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if n < 100*1024 { // < 100KB 太小
		t.Errorf("downloaded %d bytes, suspiciously small", n)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	t.Logf("downloaded %d bytes -> %s", n, dest)

	// M5 step 3: 报 user 路径 + ValidateAudioEnergy 结果 (静音检测)
	energyErr := ValidateAudioEnergy(dest)
	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("Sciam Science Quickly — sample mp3 ready for human ear review")
	fmt.Println("================================================================")
	fmt.Printf("Title:    %s\n", first.Title)
	fmt.Printf("Duration: %dms (%.1fmin)\n", first.DurationMs, float64(first.DurationMs)/60_000)
	fmt.Printf("PubDate:  %s (unix=%d)\n", time.Unix(first.PublishedAt, 0).Format(time.RFC3339), first.PublishedAt)
	fmt.Printf("Topic:    %s\n", first.Topic)
	fmt.Printf("Path:     %s (size=%d)\n", dest, info.Size())
	if energyErr == nil {
		fmt.Println("Energy:   ✓ ValidateAudioEnergy 通过 (非静音占位)")
	} else {
		fmt.Printf("Energy:   ✗ %v (会被 fetcher 标 segments_status=failed)\n", energyErr)
	}
	fmt.Println()
	fmt.Println("Listen with: afplay '" + dest + "'")
	fmt.Println("If voice / topic / quality OK, register fetcher to main.go.")
	fmt.Println("================================================================")
	fmt.Println()
}
