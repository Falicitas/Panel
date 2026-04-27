package msgsource

import "context"

// Message 是从消息源拉取/推送的统一表示。
type Message struct {
	Source    string `json:"source"`
	From      string `json:"from"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Source 是 panel 对接外部 IM (微信/QQ) 的可插拔抽象。
// Phase 1 mock 即可,真接入留到 Phase 9。
type Source interface {
	Name() string
	Subscribe(ctx context.Context, ch chan<- Message)
	Send(target, content string) error
	UnreadCount() int
}
