package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"panel/dateutil"
	"panel/english/review"
)

const (
	testToday    = "2026-05-10"
	testTomorrow = "2026-05-11"
)

// 时间戳按本地时区算 — "今日已完成"的判定用逻辑日区间 [当地 06:00, 次日 06:00),
// 写死一个 UTC 常量会随测试机时区落到隔壁那天去。
var (
	testNowUnix       = time.Date(2026, 5, 10, 12, 0, 0, 0, time.Local).Unix()
	testYesterdayUnix = time.Date(2026, 5, 9, 12, 0, 0, 0, time.Local).Unix()
)

func TestEnsureReviewItem_Insert(t *testing.T) {
	s := newTestStore(t)
	created, err := s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Errorf("created = false, want true")
	}
	it, found, err := s.GetReviewItemBySeg(1, 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("not found after ensure")
	}
	if it.Stage != 0 || it.NextReviewAt != testTomorrow ||
		it.FirstDoneAt != testNowUnix || it.LastAdvancedAt != testNowUnix ||
		it.PostponedCount != 0 || it.FinishedAt != nil || it.MasteredAt != nil {
		t.Errorf("unexpected new item: %+v", it)
	}
}

func TestEnsureReviewItem_Idempotent(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	created, err := s.EnsureReviewItem(1, 0, "2099-12-31", testNowUnix+999)
	if err != nil {
		t.Fatalf("ensure 2nd: %v", err)
	}
	if created {
		t.Error("created = true on second call, want false (no-op)")
	}
	it, _, _ := s.GetReviewItemBySeg(1, 0)
	if it.NextReviewAt != testTomorrow {
		t.Errorf("NextReviewAt mutated: got %q, want %q (no-op should not change)", it.NextReviewAt, testTomorrow)
	}
	if it.FirstDoneAt != testNowUnix {
		t.Errorf("FirstDoneAt mutated: got %d, want %d", it.FirstDoneAt, testNowUnix)
	}
}

func TestDeleteReviewItem(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	if err := s.DeleteReviewItem(1, 0); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, found, _ := s.GetReviewItemBySeg(1, 0)
	if found {
		t.Error("still found after delete")
	}
	// 不存在再删静默 nil
	if err := s.DeleteReviewItem(999, 999); err != nil {
		t.Errorf("delete non-existent: %v", err)
	}
}

func TestEnsureAfterDelete_RestartFromStage0(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	_, _ = s.AdvanceReviewItem(mustID(t, s, 1, 0), testToday, testNowUnix)
	_ = s.DeleteReviewItem(1, 0)
	created, err := s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix+1000)
	if err != nil {
		t.Fatalf("ensure after delete: %v", err)
	}
	if !created {
		t.Error("created = false, want true (delete + ensure should re-INSERT)")
	}
	it, _, _ := s.GetReviewItemBySeg(1, 0)
	if it.Stage != 0 {
		t.Errorf("stage = %d after re-ensure, want 0 (full restart)", it.Stage)
	}
	if it.FirstDoneAt != testNowUnix+1000 {
		t.Errorf("FirstDoneAt = %d, want %d (should reset to new now)", it.FirstDoneAt, testNowUnix+1000)
	}
}

func TestAdvanceReviewItem_StageProgression(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	id := mustID(t, s, 1, 0)
	cases := []struct {
		toStage  int
		wantNext string
	}{
		{1, "2026-05-13"}, // today + 3
		{2, "2026-05-17"}, // today + 7
		{3, "2026-05-24"}, // today + 14
		{4, "2026-06-09"}, // today + 30
	}
	for _, c := range cases {
		it, err := s.AdvanceReviewItem(id, testToday, testNowUnix+int64(c.toStage))
		if err != nil {
			t.Fatalf("advance to stage %d: %v", c.toStage, err)
		}
		if it.Stage != c.toStage {
			t.Errorf("stage = %d, want %d", it.Stage, c.toStage)
		}
		if it.NextReviewAt != c.wantNext {
			t.Errorf("stage %d next = %q, want %q", c.toStage, it.NextReviewAt, c.wantNext)
		}
		if it.MasteredAt != nil {
			t.Errorf("stage %d unexpectedly mastered", c.toStage)
		}
	}
	// 第 5 次 advance 进入 mastered
	it, err := s.AdvanceReviewItem(id, testToday, testNowUnix+5)
	if err != nil {
		t.Fatalf("advance to mastered: %v", err)
	}
	if it.Stage != review.MasteredStage {
		t.Errorf("stage = %d, want %d (mastered)", it.Stage, review.MasteredStage)
	}
	if it.MasteredAt == nil {
		t.Error("mastered_at not set")
	}
}

func TestAdvanceReviewItem_BlockedAfterTerminal(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	id := mustID(t, s, 1, 0)
	// finish 后 advance 应报错
	if err := s.FinishReviewItem(id, testNowUnix); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := s.AdvanceReviewItem(id, testToday, testNowUnix); err == nil {
		t.Error("advance after finish should error")
	}

	// 重置: delete + ensure + advance 5 次到 mastered → advance 应报错
	_ = s.DeleteReviewItem(1, 0)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	id = mustID(t, s, 1, 0)
	for i := 0; i < review.MasteredStage; i++ {
		if _, err := s.AdvanceReviewItem(id, testToday, testNowUnix); err != nil {
			t.Fatalf("advance %d: %v", i, err)
		}
	}
	if _, err := s.AdvanceReviewItem(id, testToday, testNowUnix); err == nil {
		t.Error("advance after mastered should error")
	}
}

func TestPostponeReviewItem(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	id := mustID(t, s, 1, 0)
	it, err := s.PostponeReviewItem(id, "2099-01-01", testNowUnix+100)
	if err != nil {
		t.Fatalf("postpone: %v", err)
	}
	if it.NextReviewAt != "2099-01-01" {
		t.Errorf("next = %q, want 2099-01-01", it.NextReviewAt)
	}
	if it.PostponedCount != 1 {
		t.Errorf("postponed_count = %d, want 1", it.PostponedCount)
	}
	if it.Stage != 0 {
		t.Errorf("stage = %d, want 0 (postpone shouldn't bump stage)", it.Stage)
	}
	// 再 postpone 一次
	it, _ = s.PostponeReviewItem(id, "2099-02-01", testNowUnix+200)
	if it.PostponedCount != 2 {
		t.Errorf("postponed_count = %d, want 2", it.PostponedCount)
	}
}

func TestFinishReviewItem(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	id := mustID(t, s, 1, 0)
	if err := s.FinishReviewItem(id, testNowUnix+500); err != nil {
		t.Fatalf("finish: %v", err)
	}
	it, _, _ := s.GetReviewItemBySeg(1, 0)
	if it.FinishedAt == nil || *it.FinishedAt != testNowUnix+500 {
		t.Errorf("finished_at = %v, want %d", it.FinishedAt, testNowUnix+500)
	}
	// 再 finish 报错
	if err := s.FinishReviewItem(id, testNowUnix); err == nil {
		t.Error("double finish should error")
	}
}

func TestListDueReviewItems_OrderingAndFilters(t *testing.T) {
	s := newTestStore(t)
	// audio 1 seg 0: due yesterday (overdue)
	_, _ = s.EnsureReviewItem(1, 0, "2026-05-08", testNowUnix)
	// audio 2 seg 5: due today
	_, _ = s.EnsureReviewItem(2, 5, "2026-05-10", testNowUnix)
	// audio 1 seg 3: due tomorrow (not yet due)
	_, _ = s.EnsureReviewItem(1, 3, "2026-05-11", testNowUnix)
	// audio 3 seg 0: due today but finished
	_, _ = s.EnsureReviewItem(3, 0, "2026-05-10", testNowUnix)
	id3, _, _ := s.GetReviewItemBySeg(3, 0)
	_ = s.FinishReviewItem(id3.ID, testNowUnix)
	// audio 4 seg 0: due today but mastered
	_, _ = s.EnsureReviewItem(4, 0, "2026-05-10", testNowUnix)
	id4, _, _ := s.GetReviewItemBySeg(4, 0)
	for i := 0; i < review.MasteredStage; i++ {
		_, _ = s.AdvanceReviewItem(id4.ID, testToday, testNowUnix)
	}

	due, err := s.ListDueReviewItems(testToday)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("len(due) = %d, want 2 (only overdue + today, exclude future / finished / mastered)", len(due))
	}
	// 顺序: 老的先 (next_review_at ASC, audio_id ASC, seg_idx ASC)
	if due[0].AudioID != 1 || due[0].SegIdx != 0 {
		t.Errorf("[0] = %d:%d, want 1:0 (overdue)", due[0].AudioID, due[0].SegIdx)
	}
	if due[1].AudioID != 2 || due[1].SegIdx != 5 {
		t.Errorf("[1] = %d:%d, want 2:5 (today)", due[1].AudioID, due[1].SegIdx)
	}
}

func TestCountReviewDoneToday(t *testing.T) {
	s := newTestStore(t)
	// 仅入队 (stage=0) 不算
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	// 今天 advance 算
	_, _ = s.EnsureReviewItem(2, 0, testTomorrow, testNowUnix)
	id2 := mustID(t, s, 2, 0)
	_, _ = s.AdvanceReviewItem(id2, testToday, testNowUnix)
	// 昨天 advance 不算 (last_advanced_at 在昨天)
	_, _ = s.EnsureReviewItem(3, 0, testTomorrow, testNowUnix)
	id3 := mustID(t, s, 3, 0)
	_, _ = s.AdvanceReviewItem(id3, "2026-05-09", testYesterdayUnix)
	// 今天 finish 不算
	_, _ = s.EnsureReviewItem(4, 0, testTomorrow, testNowUnix)
	id4 := mustID(t, s, 4, 0)
	_ = s.FinishReviewItem(id4, testNowUnix)

	count, err := s.CountReviewDoneToday(testToday)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (only audio 2)", count)
	}
}

// TestListReviewItemsDoneToday 今日已完成集合的判定 + 排序 (队列前缀就靠它)。
func TestListReviewItemsDoneToday(t *testing.T) {
	s := newTestStore(t)
	adv := func(audioID int64, today string, at int64) {
		t.Helper()
		_, _ = s.EnsureReviewItem(audioID, 0, testTomorrow, testNowUnix)
		if _, err := s.AdvanceReviewItem(mustID(t, s, audioID, 0), today, at); err != nil {
			t.Fatalf("advance audio %d: %v", audioID, err)
		}
	}
	// 1: 只入队 stage=0 → 不算 (EnsureReviewItem 也写 last_advanced_at)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	// 2 / 6: 今天 advance → 算, 按 last_advanced_at ASC 排
	adv(6, testToday, testNowUnix+10)
	adv(2, testToday, testNowUnix+20)
	// 3: 昨天 advance → 不算
	adv(3, "2026-05-09", testYesterdayUnix)
	// 4: 今天 advance 后被永久跳过 → 不算
	adv(4, testToday, testNowUnix+40)
	if err := s.FinishReviewItem(mustID(t, s, 4, 0), testNowUnix); err != nil {
		t.Fatalf("finish: %v", err)
	}
	// 5: 今天一路复到 mastered → 仍算 (毕业那条也该留在队列里看见)
	_, _ = s.EnsureReviewItem(5, 0, testTomorrow, testNowUnix)
	for i := 0; i < review.MasteredStage; i++ {
		if _, err := s.AdvanceReviewItem(mustID(t, s, 5, 0), testToday, testNowUnix+30+int64(i)); err != nil {
			t.Fatalf("advance to mastered: %v", err)
		}
	}

	got, err := s.ListReviewItemsDoneToday(testToday)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantAudio := []int64{6, 2, 5}
	if len(got) != len(wantAudio) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(wantAudio), got)
	}
	for i, want := range wantAudio {
		if got[i].AudioID != want {
			t.Errorf("[%d] audio = %d, want %d (last_advanced_at ASC)", i, got[i].AudioID, want)
		}
	}

	count, err := s.CountReviewDoneToday(testToday)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != len(got) {
		t.Errorf("count = %d, want %d (跟 list 同一判定)", count, len(got))
	}
}

// TestListReviewItemsDoneToday_LogicalDayBoundary 日界按逻辑日 (当地 06:00) 切, 不是日历日。
func TestListReviewItemsDoneToday_LogicalDayBoundary(t *testing.T) {
	s := newTestStore(t)
	start, end, err := dateutil.LogicalDayRangeUnix(testToday)
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	cases := []struct {
		audioID int64
		at      int64
		want    bool
	}{
		{1, start - 1, false},
		{2, start, true},
		{3, end - 1, true},
		{4, end, false},
	}
	for _, c := range cases {
		_, _ = s.EnsureReviewItem(c.audioID, 0, testTomorrow, testNowUnix)
		if _, err := s.AdvanceReviewItem(mustID(t, s, c.audioID, 0), testToday, c.at); err != nil {
			t.Fatalf("advance audio %d: %v", c.audioID, err)
		}
	}
	for _, c := range cases {
		got, err := s.IsReviewItemDoneToday(mustID(t, s, c.audioID, 0), testToday)
		if err != nil {
			t.Fatalf("is done audio %d: %v", c.audioID, err)
		}
		if got != c.want {
			t.Errorf("audio %d at %d: done = %v, want %v", c.audioID, c.at, got, c.want)
		}
	}
}

// TestIsReviewItemDoneToday_NotAdvanced 只入队没复习 / 不存在的行都是 false。
func TestIsReviewItemDoneToday_NotAdvanced(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	got, err := s.IsReviewItemDoneToday(mustID(t, s, 1, 0), testToday)
	if err != nil {
		t.Fatalf("is done: %v", err)
	}
	if got {
		t.Error("stage=0 不该算今日已完成")
	}
	if got, err = s.IsReviewItemDoneToday(999, testToday); err != nil || got {
		t.Errorf("不存在的行 = %v (err %v), want false", got, err)
	}
}

func TestGetReviewItemByID_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.getReviewItemByID(999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("want sql.ErrNoRows, got %v", err)
	}
}

func TestEnsureReviewItem_UniqueAudioSeg(t *testing.T) {
	s := newTestStore(t)
	created1, _ := s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	created2, _ := s.EnsureReviewItem(1, 0, testTomorrow, testNowUnix)
	if !created1 {
		t.Error("first insert should report created")
	}
	if created2 {
		t.Error("second insert (same audio_id, seg_idx) should report not created")
	}
}

// mustID 辅助: 取 (audio, seg) 对应行的 id; 找不到 t.Fatal。
func mustID(t *testing.T, s *Store, audioID int64, segIdx int) int64 {
	t.Helper()
	it, found, err := s.GetReviewItemBySeg(audioID, segIdx)
	if err != nil {
		t.Fatalf("getReviewItemBySeg %d:%d: %v", audioID, segIdx, err)
	}
	if !found {
		t.Fatalf("review item %d:%d not found", audioID, segIdx)
	}
	return it.ID
}
