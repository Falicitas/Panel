package store

import (
	"fmt"
	"testing"
)

// 用户主动删 supplement → 同 source_ref 前缀 'supplement:{id}:' 的所有通知应硬 DELETE,
// 不是 dismiss。剩下其它 supplement 的通知不受影响。
func TestDeleteSupplement_CascadesNotifications(t *testing.T) {
	s := newTestStore(t)

	sp1, err := s.CreateSupplement(Supplement{
		Name: "vitamin C", Type: "supplement", Enabled: true,
		Windows: []Window{{Start: "08:00", End: "09:00"}},
	})
	if err != nil {
		t.Fatalf("CreateSupplement sp1: %v", err)
	}
	sp2, err := s.CreateSupplement(Supplement{
		Name: "fish oil", Type: "supplement", Enabled: true,
		Windows: []Window{{Start: "21:00", End: "22:00"}},
	})
	if err != nil {
		t.Fatalf("CreateSupplement sp2: %v", err)
	}

	// 给 sp1 写 2 条通知 (window 0 + 1, 一条 dismissed 一条未 dismissed)
	if _, err := s.CreateNotificationIdempotent(Notification{
		Source: "supplement", SourceRef: fmt.Sprintf("supplement:%d:window:0:date:2026-01-01", sp1.ID),
		Title: "vit C 时段", Body: "vitamin C 时段 08:00-09:00", Level: "info", CreatedAt: 1700000000,
	}); err != nil {
		t.Fatalf("create n1: %v", err)
	}
	if _, err := s.CreateNotificationIdempotent(Notification{
		Source: "supplement", SourceRef: fmt.Sprintf("supplement:%d:window:1:date:2026-01-02", sp1.ID),
		Title: "vit C 时段 2", Body: "vit C 时段 2", Level: "info", CreatedAt: 1700000100,
	}); err != nil {
		t.Fatalf("create n2: %v", err)
	}
	_ = s.DismissNotificationByRef(fmt.Sprintf("supplement:%d:window:1:date:2026-01-02", sp1.ID), 1700000200)

	// sp2 一条通知, 应不受影响
	if _, err := s.CreateNotificationIdempotent(Notification{
		Source: "supplement", SourceRef: fmt.Sprintf("supplement:%d:window:0:date:2026-01-01", sp2.ID),
		Title: "fish oil 时段", Body: "fish oil 时段 21:00-22:00", Level: "info", CreatedAt: 1700000300,
	}); err != nil {
		t.Fatalf("create n3: %v", err)
	}

	// 删 sp1
	if err := s.DeleteSupplement(sp1.ID); err != nil {
		t.Fatalf("DeleteSupplement: %v", err)
	}

	// supplement 1 没了
	if _, err := s.GetSupplement(sp1.ID); err == nil {
		t.Fatalf("expected sp1 deleted, still found")
	}

	// sp2 还在
	if _, err := s.GetSupplement(sp2.ID); err != nil {
		t.Fatalf("expected sp2 still exists, err=%v", err)
	}

	// notifications: 全列出 (含 dismissed) 应只剩 sp2 那条
	all, err := s.ListNotifications(true)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 remaining notif (sp2's), got %d: %+v", len(all), all)
	}
	want := fmt.Sprintf("supplement:%d:window:0:date:2026-01-01", sp2.ID)
	if all[0].SourceRef != want {
		t.Fatalf("expected remaining ref=%q got %q", want, all[0].SourceRef)
	}
}
