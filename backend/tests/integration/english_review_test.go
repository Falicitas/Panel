package integration

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"panel/store"
)

// reviewItemRespJSON 镜像 server.reviewItemResp 的 JSON 响应; integration 包不能 import server。
type reviewItemRespJSON struct {
	store.ReviewItem
	OverdueDays int                  `json:"overdue_days"`
	DoneToday   bool                 `json:"done_today"`
	Segment     store.EnglishSegment `json:"segment"`
}

type reviewGroupRespJSON struct {
	Audio store.EnglishAudio   `json:"audio"`
	Items []reviewItemRespJSON `json:"items"`
}

type reviewTodayRespJSON struct {
	Date          string                `json:"date"`
	DailyLimit    int                   `json:"daily_limit"`
	DoneToday     int                   `json:"done_today"`
	DueTotal      int                   `json:"due_total"`
	PendingShown  int                   `json:"pending_shown"`
	DeferredTotal int                   `json:"deferred_total"`
	Groups        []reviewGroupRespJSON `json:"groups"`
}

// seedDueReviewItems 给 audio 灌 n 段并全部入队 due today (绕过勾选路径直接造队列)。
func seedDueReviewItems(t *testing.T, e *testEnv, audioID int64, n int) {
	t.Helper()
	segs := make([]store.EnglishSegment, n)
	for i := range segs {
		segs[i] = store.EnglishSegment{
			Idx: i, StartMs: int64(i * 1000), EndMs: int64((i + 1) * 1000), Text: "s",
		}
	}
	if err := e.store.ReplaceSegments(audioID, segs); err != nil {
		t.Fatalf("replace segs: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := e.store.EnsureReviewItem(audioID, i, e.today, time.Now().Unix()); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}
}

// setReviewLimit 改每日复习配额。
func setReviewLimit(t *testing.T, e *testEnv, limit int) {
	t.Helper()
	es, err := e.store.GetEnglishSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	es.ReviewDailyLimit = limit
	if err := e.store.SetEnglishSettings(es); err != nil {
		t.Fatalf("set settings: %v", err)
	}
}

// getReviewToday GET /review/today 并解析。
func getReviewToday(t *testing.T, e *testEnv) reviewTodayRespJSON {
	t.Helper()
	status, raw := e.get("/api/english/review/today")
	e.mustOK("GET", "/review/today", status, raw)
	var resp reviewTodayRespJSON
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

// flatItems 把分组拍平成一条队列 (断言前缀顺序用)。
func flatItems(resp reviewTodayRespJSON) []reviewItemRespJSON {
	out := []reviewItemRespJSON{}
	for _, g := range resp.Groups {
		out = append(out, g.Items...)
	}
	return out
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
// TestEnglishReview_QuotaIsDisplayCapOnly 配额只裁展示, 绝不写库 —
// 溢出的条目 next_review_at 原地不动 (老实现会把它们 postpone 到次日)。
func TestEnglishReview_QuotaIsDisplayCapOnly(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "rev3", LocalPath: "/x/rev3.mp3"})
	seedDueReviewItems(t, e, id, 30)
	setReviewLimit(t, e, 5)

	resp := getReviewToday(t, e)
	if resp.DailyLimit != 5 {
		t.Errorf("daily_limit = %d, want 5", resp.DailyLimit)
	}
	if resp.DueTotal != 30 {
		t.Errorf("due_total = %d, want 30", resp.DueTotal)
	}
	if resp.PendingShown != 5 {
		t.Errorf("pending_shown = %d, want 5", resp.PendingShown)
	}
	if resp.DeferredTotal != 25 {
		t.Errorf("deferred_total = %d, want 25", resp.DeferredTotal)
	}
	if resp.DoneToday != 0 {
		t.Errorf("done_today = %d, want 0", resp.DoneToday)
	}
	if len(resp.Groups) != 1 || len(resp.Groups[0].Items) != 5 {
		t.Fatalf("groups shape wrong: %+v", resp.Groups)
	}

	// 关键: 30 条 next_review_at 一条都没被改动
	for i := 0; i < 30; i++ {
		it, _, _ := e.store.GetReviewItemBySeg(id, i)
		if it.NextReviewAt != e.today {
			t.Fatalf("seg %d next = %q, want %q (配额不该写库)", i, it.NextReviewAt, e.today)
		}
		if it.PostponedCount != 0 {
			t.Fatalf("seg %d postponed_count = %d, want 0", i, it.PostponedCount)
		}
	}
}

// TestEnglishReview_LimitLowerThenRaiseRestores 配额调小再调大, 条目全回来。
// 复现用户实测: 配额 10 / 10 条没复习 → 改 5 → 改回 10, 老实现只剩 5 条。
func TestEnglishReview_LimitLowerThenRaiseRestores(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revLimit", LocalPath: "/x/revlimit.mp3"})
	seedDueReviewItems(t, e, id, 10)

	setReviewLimit(t, e, 10)
	if got := getReviewToday(t, e).PendingShown; got != 10 {
		t.Fatalf("limit=10: pending_shown = %d, want 10", got)
	}

	setReviewLimit(t, e, 5)
	resp := getReviewToday(t, e)
	if resp.PendingShown != 5 || resp.DeferredTotal != 5 {
		t.Fatalf("limit=5: pending=%d deferred=%d, want 5/5", resp.PendingShown, resp.DeferredTotal)
	}

	setReviewLimit(t, e, 10)
	resp = getReviewToday(t, e)
	if resp.PendingShown != 10 {
		t.Errorf("limit 回 10: pending_shown = %d, want 10 (条目该全回来)", resp.PendingShown)
	}
	if resp.DueTotal != 10 {
		t.Errorf("limit 回 10: due_total = %d, want 10", resp.DueTotal)
	}
}

// TestEnglishReview_DoneItemsStayInQueue 复习完的不收起来, 常驻队列前缀;
// 配额调到低于已完成数, 那几条照样在。
func TestEnglishReview_DoneItemsStayInQueue(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revStay", LocalPath: "/x/revstay.mp3"})
	seedDueReviewItems(t, e, id, 10)
	setReviewLimit(t, e, 10)

	// 挑中间两段复习 (乱序复习才能验出"前缀"而不是"原地")
	for _, segIdx := range []int{3, 7} {
		it, _, _ := e.store.GetReviewItemBySeg(id, segIdx)
		status, raw := e.post("/api/english/review/"+i64s(it.ID)+"/done", nil)
		e.mustOK("POST", "/done", status, raw)
	}

	resp := getReviewToday(t, e)
	if resp.DoneToday != 2 {
		t.Errorf("done_today = %d, want 2", resp.DoneToday)
	}
	if resp.DueTotal != 8 || resp.PendingShown != 8 {
		t.Errorf("due=%d pending=%d, want 8/8", resp.DueTotal, resp.PendingShown)
	}
	items := flatItems(resp)
	if len(items) != 10 {
		t.Fatalf("队列行数 = %d, want 10 (2 已完成 + 8 待复习)", len(items))
	}
	// 已完成的排前缀, 按复习先后
	if items[0].SegIdx != 3 || !items[0].DoneToday {
		t.Errorf("items[0] = seg %d done=%v, want seg 3 done=true", items[0].SegIdx, items[0].DoneToday)
	}
	if items[1].SegIdx != 7 || !items[1].DoneToday {
		t.Errorf("items[1] = seg %d done=%v, want seg 7 done=true", items[1].SegIdx, items[1].DoneToday)
	}
	for _, it := range items[2:] {
		if it.DoneToday {
			t.Errorf("seg %d 不该标 done_today", it.SegIdx)
		}
	}

	// 配额压到 1 (< 已完成 2): 待复习清零, 已完成的 2 条仍在
	setReviewLimit(t, e, 1)
	resp = getReviewToday(t, e)
	if resp.PendingShown != 0 {
		t.Errorf("limit=1: pending_shown = %d, want 0", resp.PendingShown)
	}
	items = flatItems(resp)
	if len(items) != 2 {
		t.Fatalf("limit=1: 队列行数 = %d, want 2 (已完成不受配额裁剪)", len(items))
	}
	for _, it := range items {
		if !it.DoneToday {
			t.Errorf("limit=1: seg %d 该是已完成行", it.SegIdx)
		}
	}

	// 配额调回 10: 8 条待复习回来
	if got := getReviewTodayAfterLimit(t, e, 10); got.PendingShown != 8 {
		t.Errorf("limit 回 10: pending_shown = %d, want 8", got.PendingShown)
	}
}

func getReviewTodayAfterLimit(t *testing.T, e *testEnv, limit int) reviewTodayRespJSON {
	t.Helper()
	setReviewLimit(t, e, limit)
	return getReviewToday(t, e)
}

// TestEnglishReview_RaiseLimitAfterAllDoneRevealsMore 复现用户实测:
// 配额 6 / 6 条全复习完 → 配额改 10, 该再冒出 4 条待复习 (老实现一条不出)。
func TestEnglishReview_RaiseLimitAfterAllDoneRevealsMore(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revRaise", LocalPath: "/x/revraise.mp3"})
	seedDueReviewItems(t, e, id, 10)
	setReviewLimit(t, e, 6)

	if got := getReviewToday(t, e).PendingShown; got != 6 {
		t.Fatalf("limit=6: pending_shown = %d, want 6", got)
	}
	status, raw := e.post("/api/english/review/done-all", nil)
	e.mustOK("POST", "/done-all", status, raw)

	resp := getReviewToday(t, e)
	if resp.DoneToday != 6 || resp.PendingShown != 0 || resp.DueTotal != 4 {
		t.Fatalf("全复习完: done=%d pending=%d due=%d, want 6/0/4",
			resp.DoneToday, resp.PendingShown, resp.DueTotal)
	}
	if n := len(flatItems(resp)); n != 6 {
		t.Errorf("全复习完队列行数 = %d, want 6 (都是已完成)", n)
	}

	setReviewLimit(t, e, 10)
	resp = getReviewToday(t, e)
	if resp.DoneToday != 6 || resp.PendingShown != 4 {
		t.Errorf("配额改 10: done=%d pending=%d, want 6/4", resp.DoneToday, resp.PendingShown)
	}
	if n := len(flatItems(resp)); n != 10 {
		t.Errorf("配额改 10: 队列行数 = %d, want 10", n)
	}
}

// TestEnglishReview_DoneTwiceSameDayConflicts 同一逻辑日重复 done 返回 409, stage 不多跳。
func TestEnglishReview_DoneTwiceSameDayConflicts(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revDup", LocalPath: "/x/revdup.mp3"})
	seedDueReviewItems(t, e, id, 1)

	it, _, _ := e.store.GetReviewItemBySeg(id, 0)
	status, raw := e.post("/api/english/review/"+i64s(it.ID)+"/done", nil)
	e.mustOK("POST", "/done", status, raw)

	status, _ = e.post("/api/english/review/"+i64s(it.ID)+"/done", nil)
	if status != http.StatusConflict {
		t.Errorf("第二次 done status = %d, want 409", status)
	}
	got, _, _ := e.store.GetReviewItemBySeg(id, 0)
	if got.Stage != 1 {
		t.Errorf("stage = %d, want 1 (重复 done 不该多跳)", got.Stage)
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

// TestEnglishReview_DoneAll 一键全部复习: 今日 due 全部 advance 到 stage 1, 队列清空, 再点幂等。
func TestEnglishReview_DoneAll(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revAll", LocalPath: "/x/revall.mp3"})
	seedThreeSegments(e, id)
	for i := 0; i < 3; i++ {
		if _, err := e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix()); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}

	status, raw := e.post("/api/english/review/done-all", nil)
	e.mustOK("POST", "/done-all", status, raw)

	wantNext := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	for i := 0; i < 3; i++ {
		got, _, _ := e.store.GetReviewItemBySeg(id, i)
		if got.Stage != 1 {
			t.Errorf("seg %d stage = %d, want 1", i, got.Stage)
		}
		if got.NextReviewAt != wantNext {
			t.Errorf("seg %d next = %q, want %q", i, got.NextReviewAt, wantNext)
		}
	}
	if due, _ := e.store.ListDueReviewItems(e.today); len(due) != 0 {
		t.Errorf("due after done-all = %d, want 0", len(due))
	}

	// 再点一次仍 2xx: 已完成数顶掉配额 + due 已空, 双重 no-op。
	status, raw = e.post("/api/english/review/done-all", nil)
	e.mustOK("POST", "/done-all (empty)", status, raw)
}

// TestEnglishReview_DoneAllRespectsCap due 超配额时 done-all 只 advance 封顶内的, 多余留 due。
func TestEnglishReview_DoneAllRespectsCap(t *testing.T) {
	e := newTestEnv(t)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "revCap", LocalPath: "/x/revcap.mp3"})
	segs := make([]store.EnglishSegment, 7)
	for i := range segs {
		segs[i] = store.EnglishSegment{
			Idx: i, StartMs: int64(i * 1000), EndMs: int64((i + 1) * 1000), Text: "s",
		}
	}
	if err := e.store.ReplaceSegments(id, segs); err != nil {
		t.Fatalf("replace segs: %v", err)
	}
	for i := 0; i < 7; i++ {
		if _, err := e.store.EnsureReviewItem(id, i, e.today, time.Now().Unix()); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}
	es, _ := e.store.GetEnglishSettings()
	es.ReviewDailyLimit = 5
	if err := e.store.SetEnglishSettings(es); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	status, raw := e.post("/api/english/review/done-all", nil)
	e.mustOK("POST", "/done-all", status, raw)

	// seg_idx ASC 排序下前 5 段 (0..4) advance, 后 2 段 (5,6) 仍 stage 0 due。
	for i := 0; i < 5; i++ {
		got, _, _ := e.store.GetReviewItemBySeg(id, i)
		if got.Stage != 1 {
			t.Errorf("seg %d stage = %d, want 1 (within cap)", i, got.Stage)
		}
	}
	for i := 5; i < 7; i++ {
		got, _, _ := e.store.GetReviewItemBySeg(id, i)
		if got.Stage != 0 {
			t.Errorf("seg %d stage = %d, want 0 (beyond cap)", i, got.Stage)
		}
	}
	if due, _ := e.store.ListDueReviewItems(e.today); len(due) != 2 {
		t.Errorf("due after capped done-all = %d, want 2", len(due))
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
