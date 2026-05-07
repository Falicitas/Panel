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

// NPR The Indicator from Planet Money — 主综合源 (替换 VOA 静音 mp3 源)。
// 经济 / 商业新闻, B2-C1 美式英语, 9-11 min 一集, ~8-10 MB, 完美适配 Groq Whisper
// (单文件 ≤25MB) 和精听 10min/天 阈值。
//
// RSS feed: https://feeds.npr.org/510325/podcast.xml
// Mp3 redirect chain (实测): prfx.byspotify.com → play.podtrac.com → npr.simplecastaudio.com
const nprIndicatorFeedURL = "https://feeds.npr.org/510325/podcast.xml"

// SSRF 防护 allowlist — feed 主机 + redirect chain 涉及的 CDN 全部 host。
// 后缀语义: host == suffix 或 host 以 "." + suffix 结尾 (子域)。
var nprAllowedHostSuffixes = []string{
	"npr.org",
	"prfx.byspotify.com",
	"play.podtrac.com",
	"simplecastaudio.com",
}

func isNPRAllowedHost(host string) bool {
	if host == "" {
		return false
	}
	for _, s := range nprAllowedHostSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

type NPRFetcher struct {
	client *http.Client
}

func NewNPRFetcher() *NPRFetcher {
	return &NPRFetcher{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (f *NPRFetcher) Name() string { return "npr" }

func (f *NPRFetcher) Fetch(ctx context.Context) ([]AudioMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nprIndicatorFeedURL, nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024)) // 4MB cap (NPR feed 比 VOA 长)
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
		if !isNPRAllowedHost(hostOf(item.Enclosure.URL)) {
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
			Source:      "npr",
			SourceRef:   ref,
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(item.Description),
			PublishedAt: pubUnix,
			DurationMs:  dur.Milliseconds(),
			AudioURL:    item.Enclosure.URL,
			Topic:       "finance",
		})
	}
	return out, nil
}

// DownloadAudio 流式下载 audioURL 到 destPath。
// destPath 已存在则跳过 (返回 0, nil)。
// 校验初始 URL host + redirect 后 final URL host 都必须在 nprAllowedHostSuffixes。
// 实测 final host 落在 npr.simplecastaudio.com, 由 simplecastaudio.com 后缀涵盖。
func (f *NPRFetcher) DownloadAudio(ctx context.Context, audioURL, destPath string) (int64, error) {
	if !isNPRAllowedHost(hostOf(audioURL)) {
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
		if !isNPRAllowedHost(resp.Request.URL.Host) {
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
