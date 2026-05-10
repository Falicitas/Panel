package integration

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"panel/store"
)

// reviewItemRespJSON 镜像 server.reviewItemResp 的 JSON 响应; integration 包不能 import server。
type reviewItemRespJSON struct {
	store.ReviewItem
	OverdueDays int                  `json:"overdue_days"`
	Segment     store.EnglishSegment `json:"segment"`
}

type reviewGroupRespJSON struct {
	Audio store.EnglishAudio   `json:"audio"`
	Items []reviewItemRespJSON `json:"items"`
}

type reviewTodayRespJSON struct {
	Date           string                `json:"date"`
	DailyLimit     int                   `json:"daily_limit"`
	DueTotal       int                   `json:"due_total"`
	ShownTotal     int                   `json:"shown_total"`
	PostponedTotal int                   `json:"postponed_total"`
	Groups         []reviewGroupRespJSON `json:"groups"`
}

// TestEnglishReview_CheckEnqueues 验证勾选段落 → review queue 入队 stage=0 next=tomorrow。
func TestEnglishReview_CheckEnqueues(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev1", LocalPath: "/x/rev1.mp3"})
	seedThreeSegments(e, id)

	checkOne := func(idx int) {
		body := map[string]any{
			"audio_id": id, "seg_idx": idx, "checked": true, "seconds": 10.0,
		}
		status, raw := e.post("/api/english/segments/check", body)
		e.mustOK("POST", "/check", status, raw)
	}
	checkOne(0)
	checkOne(2)

	it0, found, err := e.store.GetReviewItemBySeg(id, 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("seg 0 not enqueued")
	}
	if it0.Stage != 0 {
		t.Errorf("seg 0 stage = %d, want 0", it0.Stage)
	}
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if it0.NextReviewAt != tomorrow {
		t.Errorf("seg 0 next = %q, want %q", it0.NextReviewAt, tomorrow)
	}

	_, found1, _ := e.store.GetReviewItemBySeg(id, 1)
	if found1 {
		t.Error("seg 1 should NOT be enqueued (not checked)")
	}
}

// TestEnglishReview_UncheckRemoves 取消勾选 → review queue 物理删除。
func TestEnglishReview_UncheckRemoves(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev2", LocalPath: "/x/rev2.mp3"})
	seedThreeSegments(e, id)

	check := func(idx int, checked bool, seconds float64) {
		body := map[string]any{
			"audio_id": id, "seg_idx": idx, "checked": checked, "seconds": seconds,
		}
		status, raw := e.post("/api/english/segments/check", body)
		e.mustOK("POST", "/check", status, raw)
	}
	check(0, true, 10.0)
	// advance 一次让 stage 变 1
	it, _, _ := e.store.GetReviewItemBySeg(id, 0)
	if _, err := e.store.AdvanceReviewItem(it.ID, e.today, time.Now().Unix()); err != nil {
		t.Fatalf("advance: %v", err)
	}
	check(0, false, 0)

	_, found, _ := e.store.GetReviewItemBySeg(id, 0)
	if found {
		t.Error("seg 0 should be physically deleted after uncheck")
	}
	// 重新勾应该 stage=0 (重头开始)
	check(0, true, 10.0)
	it2, found2, _ := e.store.GetReviewItemBySeg(id, 0)
	if !found2 {
		t.Fatal("seg 0 not re-enqueued after re-check")
	}
	if it2.Stage != 0 {
		t.Errorf("re-enqueued stage = %d, want 0 (full restart)", it2.Stage)
	}
}

// TestEnglishReview_TodayQuotaOverflow 30 条 due + limit 5 → shown 5, postponed 25。
func TestEnglishReview_TodayQuotaOverflow(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev3", LocalPath: "/x/rev3.mp3"})
	// 注入 30 段并模拟全部 due today (直接绕 store 入队避免靠勾选制造)
	segs := make([]store.EnglishSegment, 30)
	for i := 0; i < 30; i++ {
		segs[i] = store.EnglishSegment{
			Idx: i, StartMs: int64(i * 1000), EndMs: int64((i + 1) * 1000), Text: "s",
		}
	}
	if err := e.store.ReplaceSegments(id, segs); err != nil {
		t.Fatalf("replace segs: %v", err)
	}
	for i := 0; i < 30; i++ {
		if _, err := e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix()); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}

	// 改 settings 配额到 5
	es, _ := e.store.GetEnglishSettings()
	es.ReviewDailyLimit = 5
	if err := e.store.SetEnglishSettings(es); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	status, raw := e.get("/api/english/review/today")
	e.mustOK("GET", "/review/today", status, raw)
	var resp reviewTodayRespJSON
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DailyLimit != 5 {
		t.Errorf("daily_limit = %d, want 5", resp.DailyLimit)
	}
	if resp.DueTotal != 30 {
		t.Errorf("due_total = %d, want 30", resp.DueTotal)
	}
	if resp.ShownTotal != 5 {
		t.Errorf("shown_total = %d, want 5", resp.ShownTotal)
	}
	if resp.PostponedTotal != 25 {
		t.Errorf("postponed_total = %d, want 25", resp.PostponedTotal)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(resp.Groups))
	}
	if len(resp.Groups[0].Items) != 5 {
		t.Errorf("group[0] items = %d, want 5", len(resp.Groups[0].Items))
	}

	// 验证溢出的 25 条 next_review_at 已经被 postpone 到明天
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	postponedCount := 0
	for i := 0; i < 30; i++ {
		it, _, _ := e.store.GetReviewItemBySeg(id, i)
		if it.NextReviewAt == tomorrow {
			postponedCount++
		}
	}
	if postponedCount != 25 {
		t.Errorf("postponed in db = %d, want 25", postponedCount)
	}
}

// TestEnglishReview_DonePostponeFinish 三个动作分别推进 stage / 延后 / 永久跳过。
func TestEnglishReview_DonePostponeFinish(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev4", LocalPath: "/x/rev4.mp3"})
	seedThreeSegments(e, id)
	// 直接入队 due today
	for i := 0; i < 3; i++ {
		_, _ = e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix())
	}

	itDone, _, _ := e.store.GetReviewItemBySeg(id, 0)
	itPost, _, _ := e.store.GetReviewItemBySeg(id, 1)
	itFin, _, _ := e.store.GetReviewItemBySeg(id, 2)

	// done → stage 1, next = today + 3
	status, raw := e.post("/api/english/review/"+i64s(itDone.ID)+"/done", nil)
	e.mustOK("POST", "/done", status, raw)
	got, _, _ := e.store.GetReviewItemBySeg(id, 0)
	if got.Stage != 1 {
		t.Errorf("done: stage = %d, want 1", got.Stage)
	}
	wantNext := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	if got.NextReviewAt != wantNext {
		t.Errorf("done: next = %q, want %q", got.NextReviewAt, wantNext)
	}

	// postpone → next = tomorrow, postponed_count = 1
	status, raw = e.post("/api/english/review/"+i64s(itPost.ID)+"/postpone", nil)
	e.mustOK("POST", "/postpone", status, raw)
	got, _, _ = e.store.GetReviewItemBySeg(id, 1)
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if got.NextReviewAt != tomorrow {
		t.Errorf("postpone: next = %q, want %q", got.NextReviewAt, tomorrow)
	}
	if got.PostponedCount != 1 {
		t.Errorf("postpone: count = %d, want 1", got.PostponedCount)
	}

	// finish → finished_at 非空, ListDueReviewItems 不返回它
	status, raw = e.post("/api/english/review/"+i64s(itFin.ID)+"/finish", nil)
	e.mustOK("POST", "/finish", status, raw)
	got, _, _ = e.store.GetReviewItemBySeg(id, 2)
	if got.FinishedAt == nil {
		t.Error("finish: finished_at not set")
	}
	due, _ := e.store.ListDueReviewItems(e.today)
	for _, it := range due {
		if it.SegIdx == 2 {
			t.Error("finished item should not appear in due list")
		}
	}
}

// TestEnglishReview_PostponeWithToDate body 显式带 to_date 时使用之。
func TestEnglishReview_PostponeWithToDate(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev5", LocalPath: "/x/rev5.mp3"})
	seedThreeSegments(e, id)
	_, _ = e.store.EnsureReviewItem(id, 0, e.today, time.Now().Unix())
	it, _, _ := e.store.GetReviewItemBySeg(id, 0)

	body := map[string]any{"to_date": "2099-01-15"}
	status, raw := e.post("/api/english/review/"+i64s(it.ID)+"/postpone", body)
	e.mustOK("POST", "/postpone", status, raw)
	got, _, _ := e.store.GetReviewItemBySeg(id, 0)
	if got.NextReviewAt != "2099-01-15" {
		t.Errorf("next = %q, want 2099-01-15", got.NextReviewAt)
	}
}

// TestEnglishReview_DeleteAudioCascades audio 永久删除 → review_queue 同 audio_id 全清。
func TestEnglishReview_DeleteAudioCascades(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev6", LocalPath: "/x/rev6.mp3"})
	seedThreeSegments(e, id)
	for i := 0; i < 3; i++ {
		_, _ = e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix())
	}

	if err := e.store.DeleteAudio(id); err != nil {
		t.Fatalf("DeleteAudio: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, found, _ := e.store.GetReviewItemBySeg(id, i)
		if found {
			t.Errorf("seg %d still in review queue after audio delete", i)
		}
	}
}

// TestEnglishReview_ReplaceSegmentsPrunesOrphans ASR 重跑 segments 减少时, 多余 idx 的 review 项级联清。
func TestEnglishReview_ReplaceSegmentsPrunesOrphans(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev7", LocalPath: "/x/rev7.mp3"})
	seedThreeSegments(e, id) // idx 0,1,2
	for i := 0; i < 3; i++ {
		_, _ = e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix())
	}

	// ASR 重跑只剩 idx 0,1
	newSegs := []store.EnglishSegment{
		{Idx: 0, StartMs: 0, EndMs: 5000, Text: "x"},
		{Idx: 1, StartMs: 5000, EndMs: 10000, Text: "y"},
	}
	if err := e.store.ReplaceSegments(id, newSegs); err != nil {
		t.Fatalf("replace: %v", err)
	}

	for _, idx := range []int{0, 1} {
		_, found, _ := e.store.GetReviewItemBySeg(id, idx)
		if !found {
			t.Errorf("seg %d should be retained (still in new idx set)", idx)
		}
	}
	_, found2, _ := e.store.GetReviewItemBySeg(id, 2)
	if found2 {
		t.Error("seg 2 should be pruned (no longer in segments)")
	}

	// 全清: ReplaceSegments(empty)
	if err := e.store.ReplaceSegments(id, []store.EnglishSegment{}); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	for _, idx := range []int{0, 1} {
		_, found, _ := e.store.GetReviewItemBySeg(id, idx)
		if found {
			t.Errorf("seg %d should be cleared (empty replace)", idx)
		}
	}
}

// TestEnglishReview_TodayResponseEnrichment GET response 富化了 segment 文本/timestamp + audio 元数据。
func TestEnglishReview_TodayResponseEnrichment(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "rev8", Title: "Test Episode", LocalPath: "/x/rev8.mp3",
	})
	seedThreeSegments(e, id)
	_, _ = e.store.EnsureReviewItem(id, 1, e.today, time.Now().Unix())

	status, raw := e.get("/api/english/review/today")
	e.mustOK("GET", "/review/today", status, raw)
	var resp reviewTodayRespJSON
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(resp.Groups))
	}
	g := resp.Groups[0]
	if g.Audio.Title != "Test Episode" {
		t.Errorf("audio title = %q, want 'Test Episode'", g.Audio.Title)
	}
	if len(g.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(g.Items))
	}
	it := g.Items[0]
	if it.SegIdx != 1 || it.Segment.Idx != 1 || it.Segment.Text != "World." {
		t.Errorf("seg enrichment wrong: %+v", it.Segment)
	}
}

// TestEnglishReview_TodayInfersOverdueDays 跨日积压自然冒泡, OverdueDays 反映逾期天数。
func TestEnglishReview_TodayInfersOverdueDays(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev9", LocalPath: "/x/rev9.mp3"})
	seedThreeSegments(e, id)
	threeDaysAgo := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	if _, err := e.store.EnsureReviewItem(id, 0, threeDaysAgo, time.Now().Unix()); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	status, raw := e.get("/api/english/review/today")
	e.mustOK("GET", "/review/today", status, raw)
	var resp reviewTodayRespJSON
	_ = json.Unmarshal(raw, &resp)
	if len(resp.Groups) != 1 || len(resp.Groups[0].Items) != 1 {
		t.Fatalf("groups/items shape wrong: %+v", resp.Groups)
	}
	if resp.Groups[0].Items[0].OverdueDays != 3 {
		t.Errorf("overdue_days = %d, want 3", resp.Groups[0].Items[0].OverdueDays)
	}
}

// TestEnglishReview_TodaySummaryOnEnglishToday GET /api/english/today 响应也带复习摘要。
func TestEnglishReview_TodaySummaryOnEnglishToday(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev10", LocalPath: "/x/rev10.mp3"})
	seedThreeSegments(e, id)
	for i := 0; i < 3; i++ {
		_, _ = e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix())
	}
	// advance 一条 → CountReviewDoneToday = 1
	it, _, _ := e.store.GetReviewItemBySeg(id, 0)
	_, _ = e.store.AdvanceReviewItem(it.ID, e.today, time.Now().Unix())

	status, raw := e.get("/api/english/today")
	e.mustOK("GET", "/today", status, raw)
	var resp map[string]any
	_ = json.Unmarshal(raw, &resp)
	if int(resp["review_done_today"].(float64)) != 1 {
		t.Errorf("review_done_today = %v, want 1", resp["review_done_today"])
	}
	if int(resp["review_daily_limit"].(float64)) != 15 {
		t.Errorf("review_daily_limit = %v, want 15 (default)", resp["review_daily_limit"])
	}
	// 已 advance 的 1 条已不 due, 剩 2 条
	if int(resp["review_due_total"].(float64)) != 2 {
		t.Errorf("review_due_total = %v, want 2", resp["review_due_total"])
	}
}

// i64s int64 → 字符串 (URL path 拼接)。
func i64s(n int64) string {
	return strconv.FormatInt(n, 10)
}
