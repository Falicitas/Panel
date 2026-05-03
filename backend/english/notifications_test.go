package english

import (
	"testing"
	"time"

	"panel/store"
)

func newNotifTestStore(t *testing.T) (*store.Store, string, time.Time) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	const date = "2026-05-03"
	if err := s.SetEnglishSettings(store.EnglishSettings{
		Windows:      []store.EnglishWindow{{Start: "21:00", End: "22:30"}},
		ThresholdMin: 10,
		Enabled:      true,
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
	if err := s.SetDailyState(store.DailyState{Date: date, Mode: "normal"}); err != nil {
		t.Fatalf("set daily state: %v", err)
	}
	// 任意"参考时刻"; 各 case 自己派生 now
	ref, err := time.ParseInLocation("2006-01-02 15:04", date+" 22:20", time.Local)
	if err != nil {
		t.Fatalf("parse ref: %v", err)
	}
	return s, date, ref
}

func mkLocalUnix(t *testing.T, date, hhmm string) int64 {
	t.Helper()
	tt, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, time.Local)
	if err != nil {
		t.Fatalf("parse %s %s: %v", date, hhmm, err)
	}
	return tt.Unix()
}

func TestGenerateNotifications_InLeadRangeUnderThresholdCreates(t *testing.T) {
	s, _, ref := newNotifTestStore(t)
	now := ref // 22:20, lead range = [22:15, 22:30)

	n, err := GenerateNotifications(s, now)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if n != 1 {
		t.Errorf("created %d, want 1", n)
	}
	// 第二次调用 = 幂等, 0 new
	n, err = GenerateNotifications(s, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if n != 0 {
		t.Errorf("second call created %d, want 0 (idempotent)", n)
	}

	notifs, _ := s.ListNotifications(false)
	if len(notifs) != 1 || notifs[0].Source != "english" {
		t.Errorf("notifications: %+v", notifs)
	}
	if notifs[0].SourceRef == "" {
		t.Errorf("source_ref empty")
	}
}

func TestGenerateNotifications_BeforeLeadRangeNoCreate(t *testing.T) {
	s, date, _ := newNotifTestStore(t)
	now, _ := time.ParseInLocation("2006-01-02 15:04", date+" 21:30", time.Local) // window 内但远早于 lead

	n, _ := GenerateNotifications(s, now)
	if n != 0 {
		t.Errorf("created %d, want 0", n)
	}
}

func TestGenerateNotifications_AfterEndNoCreate(t *testing.T) {
	s, date, _ := newNotifTestStore(t)
	now, _ := time.ParseInLocation("2006-01-02 15:04", date+" 23:00", time.Local) // 已过 end

	n, _ := GenerateNotifications(s, now)
	if n != 0 {
		t.Errorf("created %d, want 0", n)
	}
}

func TestGenerateNotifications_ThresholdReachedNoCreate(t *testing.T) {
	s, date, ref := newNotifTestStore(t)
	v := 660.0 // 11 min
	if _, err := s.UpsertRecord(store.Record{
		Module: SegModule, Date: date, Key: "1:0", ValueNum: &v,
		CreatedAt: mkLocalUnix(t, date, "21:30"),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	n, _ := GenerateNotifications(s, ref) // 22:20, lead 内但已达阈值
	if n != 0 {
		t.Errorf("created %d, want 0 (threshold reached)", n)
	}
}

func TestGenerateNotifications_OutsideMode(t *testing.T) {
	s, date, ref := newNotifTestStore(t)
	if err := s.SetDailyState(store.DailyState{Date: date, Mode: "outside"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	n, _ := GenerateNotifications(s, ref)
	if n != 0 {
		t.Errorf("outside mode: created %d, want 0", n)
	}
}

func TestGenerateNotifications_DisabledSettings(t *testing.T) {
	s, date, ref := newNotifTestStore(t)
	if err := s.SetEnglishSettings(store.EnglishSettings{
		Windows:      []store.EnglishWindow{{Start: "21:00", End: "22:30"}},
		ThresholdMin: 10,
		Enabled:      false,
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	_ = date
	n, _ := GenerateNotifications(s, ref)
	if n != 0 {
		t.Errorf("disabled: created %d, want 0", n)
	}
}

func TestGenerateNotifications_SnoozeShiftsLeadRange(t *testing.T) {
	s, date, _ := newNotifTestStore(t)
	// snooze 1 次 → effective end = 22:30 + 30min = 23:00, lead = [22:45, 23:00)
	one := 1.0
	if _, err := s.UpsertRecord(store.Record{
		Module: SnoozeModule, Date: date, Key: SnoozeKey(0), ValueNum: &one,
	}); err != nil {
		t.Fatalf("upsert snooze: %v", err)
	}

	// 22:20 (原 lead 内, 但 snooze 后 lead 还没到) → 0
	now1, _ := time.ParseInLocation("2006-01-02 15:04", date+" 22:20", time.Local)
	n, _ := GenerateNotifications(s, now1)
	if n != 0 {
		t.Errorf("22:20 with snooze: created %d, want 0", n)
	}

	// 22:50 (snooze 后 lead 内) → 1
	now2, _ := time.ParseInLocation("2006-01-02 15:04", date+" 22:50", time.Local)
	n, _ = GenerateNotifications(s, now2)
	if n != 1 {
		t.Errorf("22:50 with snooze: created %d, want 1", n)
	}

	// 23:10 (snooze 后也已过 end) → 0
	now3, _ := time.ParseInLocation("2006-01-02 15:04", date+" 23:10", time.Local)
	n, _ = GenerateNotifications(s, now3)
	if n != 0 {
		t.Errorf("23:10 (after effective end): created %d, want 0", n)
	}
}

func TestGenerateNotifications_ExpiresOldNotification(t *testing.T) {
	s, date, ref := newNotifTestStore(t)
	// 22:20 创建
	n, _ := GenerateNotifications(s, ref)
	if n != 1 {
		t.Fatalf("setup created %d, want 1", n)
	}
	// 23:00 跑 — 应该 dismiss 那条 (effective end 已过, 仍未达阈值)
	later, _ := time.ParseInLocation("2006-01-02 15:04", date+" 23:00", time.Local)
	_, err := GenerateNotifications(s, later)
	if err != nil {
		t.Fatalf("expire run: %v", err)
	}
	all, _ := s.ListNotifications(true)
	dismissed := 0
	for _, x := range all {
		if x.DismissedAt != nil {
			dismissed++
		}
	}
	if dismissed != 1 {
		t.Errorf("dismissed = %d, want 1", dismissed)
	}
	// undismissed list 应空
	live, _ := s.ListNotifications(false)
	if len(live) != 0 {
		t.Errorf("live notifications after expire: %d, want 0", len(live))
	}
}

func TestGenerateNotifications_ThresholdLaterDismisses(t *testing.T) {
	s, date, ref := newNotifTestStore(t)
	// 22:20 在 lead 内创建通知
	if _, err := GenerateNotifications(s, ref); err != nil {
		t.Fatalf("create: %v", err)
	}
	live, _ := s.ListNotifications(false)
	if len(live) != 1 {
		t.Fatalf("setup: %d live notifs", len(live))
	}
	// 用户狂勾, minutes_done 达 11
	v := 660.0
	if _, err := s.UpsertRecord(store.Record{
		Module: SegModule, Date: date, Key: "1:0", ValueNum: &v,
		CreatedAt: mkLocalUnix(t, date, "22:25"),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 再调一次 generator (handler 已 dismiss 应是常规, 但 generator 兜底)
	if _, err := GenerateNotifications(s, ref.Add(2*time.Minute)); err != nil {
		t.Fatalf("after threshold: %v", err)
	}
	live, _ = s.ListNotifications(false)
	if len(live) != 0 {
		t.Errorf("after threshold: live=%d, want 0", len(live))
	}
}

func TestSnoozeKeyAndParseRef(t *testing.T) {
	if got := SnoozeKey(3); got != "window:3" {
		t.Errorf("SnoozeKey(3) = %q", got)
	}
	const today = "2026-05-03"
	got := notifRef(2, today)
	want := "english:window:2:date:2026-05-03"
	if got != want {
		t.Errorf("notifRef = %q, want %q", got, want)
	}
	idx, date, ok := parseNotifRef(got)
	if !ok || idx != 2 || date != today {
		t.Errorf("parseNotifRef = (%d, %q, %v), want (2, %q, true)", idx, date, ok, today)
	}
	// 错的格式 → ok=false
	_, _, ok = parseNotifRef("supplement:1:window:2:date:foo")
	if ok {
		t.Errorf("parseNotifRef should reject non-english source")
	}
}
