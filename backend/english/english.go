// Package english 是英文练习模块: 多源音频抓取 + 句子级勾选 + 时段窗口通知。
//
// 该包不直接持久化,而是把 fetcher 输出的 AudioMeta 交给 store 层存。
// ASR 句子级 segments 由 mac 端 (Apple Speech Framework) 离线生成后 PUT 回。
package english

// AudioMeta 是 fetcher 输出的"音频条目"中间形式。
// 调用 store.InsertAudioIfNew 入库时映射为 store.EnglishAudio。
type AudioMeta struct {
	Source      string // "voa" | "sciam60" | ...
	SourceRef   string // 源端唯一 ID (RSS GUID / item URL)
	Title       string
	Description string
	PublishedAt int64 // unix sec
	DurationMs  int64
	AudioURL    string // 远程 mp3 URL
}
