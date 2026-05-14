package supplements

import (
	"path/filepath"
	"testing"
	"time"

	"panel/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// breakfastTime 返回今天日期 + 09:00 的 time.Time, 落在 breakfast 区间 (07:00-11:59)。
func breakfastTime(t *testing.T) time.Time {
	t.Helper()
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 9, 0, 0, 0, n.Location())
}

// TestGenerateMealNotifications_PausedMealNotCreated 暂停期内的 meal 不发新通知。
// 双向绑定: paused_until > now → GenerateMealNotifications 跳过 CreateNotificationIdempotent。
func TestGenerateMealNotifications_PausedMealNotCreated(t *testing.T) {
	s := openTestStore(t)
	now := breakfastTime(t)
	until := now.Add(7 * 24 * time.Hour).Unix()
	if err := s.SetMealPausedUntil("breakfast", &until); err != nil {
		t.Fatalf("Set paused: %v", err)
	}

	created, err := GenerateMealNotifications(s, now)
	if err != nil {
		t.Fatalf("GenerateMealNotifications: %v", err)
	}
	// breakfast 暂停 — lunch / dinner 此刻不在合法时段, 也不生成。
	if created != 0 {
		t.Errorf("created = %d, want 0 (breakfast paused, others out of range)", created)
	}
	notifs, _ := s.ListNotifications(true)
	for _, n := range notifs {
		if n.SourceRef == "meal:breakfast:date:"+todayKey(now) && n.DismissedAt == nil {
			t.Errorf("breakfast notif should not be active when paused")
		}
	}
}

// TestGenerateMealNotifications_PauseDismissesExisting 已 active 的通知在暂停时立即 dismiss。
// 这是用户场景"已经被催了再去暂停"的关键路径 — 暂停语义=未来 + 现在双向生效。
func TestGenerateMealNotifications_PauseDismissesExisting(t *testing.T) {
	s := openTestStore(t)
	now := breakfastTime(t)

	// Step 1: 未暂停时跑一次 → 应创建 breakfast 通知
	created, err := GenerateMealNotifications(s, now)
	if err != nil {
		t.Fatalf("first gen: %v", err)
	}
	if created != 1 {
		t.Fatalf("first gen created = %d, want 1 (breakfast active)", created)
	}
	ref := "meal:breakfast:date:" + todayKey(now)
	n := notifByRef(t, s, ref)
	if n == nil || n.DismissedAt != nil {
		t.Fatalf("breakfast notif should be active after first gen")
	}

	// Step 2: 暂停 breakfast → 跑 GenerateMealNotifications → 已 active 通知应 dismiss
	until := now.Add(7 * 24 * time.Hour).Unix()
	if err := s.SetMealPausedUntil("breakfast", &until); err != nil {
		t.Fatalf("Set paused: %v", err)
	}
	if _, err := GenerateMealNotifications(s, now); err != nil {
		t.Fatalf("second gen: %v", err)
	}
	n = notifByRef(t, s, ref)
	if n == nil {
		t.Fatalf("breakfast notif missing after pause")
	}
	if n.DismissedAt == nil {
		t.Errorf("breakfast notif should be dismissed after pause, got active")
	}
}

// TestGenerateMealNotifications_PausedExpiredCreatesNew 暂停过期后 (paused_until <= now)
// 通知应正常生成 — 跟 supplement.IsActive 同语义。
func TestGenerateMealNotifications_PausedExpiredCreatesNew(t *testing.T) {
	s := openTestStore(t)
	now := breakfastTime(t)
	expired := now.Add(-1 * time.Hour).Unix()
	if err := s.SetMealPausedUntil("breakfast", &expired); err != nil {
		t.Fatalf("Set paused: %v", err)
	}
	created, err := GenerateMealNotifications(s, now)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 (expired pause = active)", created)
	}
}

// TestGenerateMealNotifications_DisabledNotCreated 永久禁止的 meal 不发新通知 + dismiss 已 active。
// 跟 paused 路径同语义但不带过期, 用户主动启用后才恢复。
func TestGenerateMealNotifications_DisabledNotCreated(t *testing.T) {
	s := openTestStore(t)
	now := breakfastTime(t)

	// 先正常生成一次通知
	if _, err := GenerateMealNotifications(s, now); err != nil {
		t.Fatalf("first gen: %v", err)
	}
	ref := "meal:breakfast:date:" + todayKey(now)
	if n := notifByRef(t, s, ref); n == nil || n.DismissedAt != nil {
		t.Fatalf("breakfast notif should be active before disable")
	}

	// 禁用 breakfast → 通知应 dismiss + 不再生成新的
	if err := s.SetMealEnabled("breakfast", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	created, err := GenerateMealNotifications(s, now)
	if err != nil {
		t.Fatalf("post-disable gen: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 (breakfast disabled)", created)
	}
	if n := notifByRef(t, s, ref); n == nil || n.DismissedAt == nil {
		t.Errorf("breakfast notif should be dismissed after disable")
	}
}

// TestGenerateMealNotifications_OtherMealsUnaffected 暂停 breakfast 不影响 lunch / dinner。
func TestGenerateMealNotifications_OtherMealsUnaffected(t *testing.T) {
	s := openTestStore(t)
	// 用 lunch 时段 (12:30) 同时暂停 breakfast
	n := time.Now()
	lunchNow := time.Date(n.Year(), n.Month(), n.Day(), 12, 30, 0, 0, n.Location())
	until := lunchNow.Add(7 * 24 * time.Hour).Unix()
	if err := s.SetMealPausedUntil("breakfast", &until); err != nil {
		t.Fatalf("Set paused: %v", err)
	}
	created, err := GenerateMealNotifications(s, lunchNow)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 (lunch should fire)", created)
	}
	if notifByRef(t, s, "meal:lunch:date:"+todayKey(lunchNow)) == nil {
		t.Errorf("lunch notif should exist")
	}
}

func notifByRef(t *testing.T, s *store.Store, ref string) *store.Notification {
	t.Helper()
	notifs, err := s.ListNotifications(true)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	for i := range notifs {
		if notifs[i].SourceRef == ref {
			return &notifs[i]
		}
	}
	return nil
}

func todayKey(t time.Time) string {
	return t.Format("2006-01-02")
}
