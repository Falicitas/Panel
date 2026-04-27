package msgsource

import (
	"context"
	"log"
)

type Mock struct {
	name string
}

func NewMock(name string) *Mock { return &Mock{name: name} }

func (m *Mock) Name() string { return m.name }

func (m *Mock) Subscribe(ctx context.Context, ch chan<- Message) {
	<-ctx.Done()
}

func (m *Mock) Send(target, content string) error {
	log.Printf("[msgsource:%s] send target=%q content=%q", m.name, target, content)
	return nil
}

func (m *Mock) UnreadCount() int { return 0 }
