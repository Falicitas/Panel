package english

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// rssFeed 描述 RSS 2.0 feed 顶层结构。仅保留我们关心的字段。
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Description string       `xml:"description"`
	Link        string       `xml:"link"`
	GUID        string       `xml:"guid"`
	PubDate     string       `xml:"pubDate"`
	Enclosure   rssEnclosure `xml:"enclosure"`
	// itunes:duration ─ XML namespace 用 url 形式匹配
	Duration string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int64  `xml:"length,attr"`
}

func parseRSS(data []byte) (*rssFeed, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("unmarshal rss: %w", err)
	}
	return &feed, nil
}

// parsePubDate 试 RFC1123Z (含时区偏移) 优先, fallback RFC1123 (含命名时区)。
// VOA 用 "Mon, 23 Mar 2026 00:30:03 +0000" 格式 = RFC1123Z。
func parsePubDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty pubDate")
	}
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("parse pubDate %q: not RFC1123(Z)", s)
}

// parseHMS 解析 itunes:duration 字段。
// 接受格式: "HH:MM:SS" / "MM:SS" / "SSSS" (纯秒)。无法解析返 0。
func parseHMS(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	var h, m, sec int
	switch len(parts) {
	case 3:
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
		sec, _ = strconv.Atoi(parts[2])
	case 2:
		m, _ = strconv.Atoi(parts[0])
		sec, _ = strconv.Atoi(parts[1])
	case 1:
		sec, _ = strconv.Atoi(parts[0])
	default:
		return 0
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second
}
