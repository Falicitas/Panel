package english

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// VOA Learning English Podcast — 主综合源, 公共领域, 慢速清晰适合精听。
// 来源 RSS 由 iTunes Podcasts 反查得到 (zoneId=1689 = Learning English Podcast)。
const voaFeedURL = "https://learningenglish.voanews.com/podcast/?count=20&zoneId=1689"

const userAgent = "panel/1.0 (personal use)"

// SSRF 防护 allowlist — RSS 在 voanews 主域, mp3 enclosure 在 akamai CDN
// 入口然后 301 到 voa-audio.voanews.eu (EU 节点)。任何 host 不命中下列后缀一律拒绝。
// 后缀语义: host == suffix 或 host 以 "." + suffix 结尾 (子域)。
var voaAllowedHostSuffixes = []string{
	"voanews.com",
	"voanews.eu",
	"voa-audio-ns.akamaized.net",
}

func isVOAAllowedHost(host string) bool {
	if host == "" {
		return false
	}
	for _, s := range voaAllowedHostSuffixes {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

type VOAFetcher struct {
	client *http.Client
}

func NewVOAFetcher() *VOAFetcher {
	return &VOAFetcher{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (f *VOAFetcher) Name() string { return "voa" }

func (f *VOAFetcher) Fetch(ctx context.Context) ([]AudioMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, voaFeedURL, nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB cap
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
		if !isVOAAllowedHost(hostOf(item.Enclosure.URL)) {
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
			Source:      "voa",
			SourceRef:   ref,
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(item.Description),
			PublishedAt: pubUnix,
			DurationMs:  dur.Milliseconds(),
			AudioURL:    item.Enclosure.URL,
		})
	}
	return out, nil
}

// DownloadAudio 流式下载 audioURL 到 destPath。
// destPath 已存在则跳过 (返回 0, nil)。
// 校验初始 URL host + redirect 后 final URL host 都必须在 voaAllowedHosts。
func (f *VOAFetcher) DownloadAudio(ctx context.Context, audioURL, destPath string) (int64, error) {
	if !isVOAAllowedHost(hostOf(audioURL)) {
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
		if !isVOAAllowedHost(resp.Request.URL.Host) {
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

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
