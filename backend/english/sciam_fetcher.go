package english

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scientific American — Science Quickly podcast (原"60-Second Science"升级版)。
// 内容: 科学新闻 / 访谈 / 周报。一集 ~5-25min, 适配 Groq Whisper ≤25MB / ≤30min。
// host Rachel Feltman 标准美音, B2-C1 适合精听。
//
// RSS feed: 官方 redirect, 落到 megaphone hosting。
// Mp3 enclosure 实测落在 traffic.megaphone.fm。
const sciamScienceQuicklyFeedURL = "https://www.scientificamerican.com/sciam/xml/iTunes.cfm?id=science-quickly"

// SSRF 防护 allowlist — feed 主机 (sciam) + mp3 host (megaphone)。
// 后缀语义: host == suffix 或 host 以 "." + suffix 结尾 (子域)。
var sciamAllowedHostSuffixes = []string{
	"scientificamerican.com",
	"megaphone.fm",
}

func isSciamAllowedHost(host string) bool {
	if host == "" {
		return false
	}
	for _, s := range sciamAllowedHostSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

type SciamFetcher struct {
	client *http.Client
}

func NewSciamFetcher() *SciamFetcher {
	return &SciamFetcher{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (f *SciamFetcher) Name() string { return "sciam" }

func (f *SciamFetcher) Fetch(ctx context.Context) ([]AudioMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sciamScienceQuicklyFeedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build feed req: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed status %d", resp.StatusCode)
	}
	// Sciam feed ~4.5MB (整个 podcast 历史在一个 RSS), 给 8MB cap 留 buffer。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read feed body: %w", err)
	}

	feed, err := parseRSS(body)
	if err != nil {
		return nil, err
	}

	out := make([]AudioMeta, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		if item.Enclosure.URL == "" {
			continue
		}
		if !strings.HasPrefix(item.Enclosure.Type, "audio/") {
			continue
		}
		if !isSciamAllowedHost(hostOf(item.Enclosure.URL)) {
			continue // 静默丢弃, 防 SSRF
		}
		ref := strings.TrimSpace(item.GUID)
		if ref == "" {
			ref = strings.TrimSpace(item.Link)
		}
		if ref == "" {
			continue
		}
		var pubUnix int64
		if t, err := parsePubDate(item.PubDate); err == nil {
			pubUnix = t.Unix()
		}
		dur := parseHMS(item.Duration)
		out = append(out, AudioMeta{
			Source:      "sciam",
			SourceRef:   ref,
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(item.Description),
			PublishedAt: pubUnix,
			DurationMs:  dur.Milliseconds(),
			AudioURL:    item.Enclosure.URL,
			Topic:       "science",
		})
	}
	return out, nil
}

// DownloadAudio 流式下载 audioURL 到 destPath。逻辑跟 NPR/VOA 同 (allowlist + 流式 + tmp rename).
func (f *SciamFetcher) DownloadAudio(ctx context.Context, audioURL, destPath string) (int64, error) {
	if !isSciamAllowedHost(hostOf(audioURL)) {
		return 0, fmt.Errorf("ssrf: disallowed initial host %q", hostOf(audioURL))
	}
	if _, err := os.Stat(destPath); err == nil {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir dest dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build audio req: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("get audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil {
		if !isSciamAllowedHost(resp.Request.URL.Host) {
			return 0, fmt.Errorf("ssrf: redirect to disallowed host %q", resp.Request.URL.Host)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("audio status %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("create tmp: %w", err)
	}
	n, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("copy body: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("close tmp: %w", closeErr)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("rename tmp: %w", err)
	}
	return n, nil
}
