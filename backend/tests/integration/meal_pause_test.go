package integration

import (
	"encoding/json"
	"testing"
	"time"

	"panel/store"
)

// 三餐暂停的端到端测试: 走 POST /api/state/meal/{meal}/pause + 验证 GET /api/state/today
// 返回的 DailyState 包含 *paused_until 字段, 且 notifications 表正确 dismiss/复活。

// pauseMeal 调 POST /api/state/meal/{meal}/pause body {"until": ...}, 返回 DailyState。
func (e *testEnv) pauseMeal(meal string, until *int64) store.DailyState {
	e.t.Helper()
	body := map[string]any{"until": until}
	status, raw := e.post("/api/state/meal/"+meal+"/pause", body)
	e.mustOK("POST pause meal", "pause meal", status, raw)
	var ds store.DailyState
	if err := json.Unmarshal(raw, &ds); err != nil {
		e.t.Fatalf("unmarshal ds: %v body=%s", err, raw)
	}
	return ds
}

// nowInMealRange 查当前 wall-clock 在哪个 meal 区间内, 返回 meal 名 + 是否在某区间内。
// 用于"复用 wall-clock 选合适 meal 测试"的策略 — 避免 mock time.Now()。
func nowInMealRange() (string, bool) {
	now := time.Now()
	mins := now.Hour()*60 + now.Minute()
	switch {
	case mins >= 7*60 && mins <= 11*60+59:
		return "breakfast", true
	case mins >= 12*60 && mins <= 17*60+29:
		return "lunch", true
	case mins >= 17*60+30 && mins <= 23*60+59:
		return "dinner", true
	}
	return "", false
}

// TestMealPause_DailyStateExposesPausedUntil 暂停后 GET /api/state/today 返回 DailyState 含
// breakfast_paused_until 字段。这是 mac UI 渲染暂停状态的关键约定 — UI 单点取数据。
func TestMealPause_DailyStateExposesPausedUntil(t *testing.T) {
	e := newTestEnv(t)
	until := time.Now().Add(7 * 24 * time.Hour).Unix()
	ds := e.pauseMeal("breakfast", &until)
	if ds.BreakfastPausedUntil == nil || *ds.BreakfastPausedUntil != until {
		t.Errorf("BreakfastPausedUntil = %v, want %d", ds.BreakfastPausedUntil, until)
	}
	if ds.LunchPausedUntil != nil {
		t.Errorf("LunchPausedUntil should be nil, got %v", *ds.LunchPausedUntil)
	}

	// GET 也该返回相同状态 (pauseMeal 内部用 POST 响应; 这里二次验证持久化)
	status, raw := e.get("/api/state/today")
	e.mustOK("GET today", "today", status, raw)
	var fresh store.DailyState
	if err := json.Unmarshal(raw, &fresh); err != nil {
		t.Fatalf("unmarshal fresh ds: %v", err)
	}
	if fresh.BreakfastPausedUntil == nil || *fresh.BreakfastPausedUntil != until {
		t.Errorf("fresh BreakfastPausedUntil = %v, want %d", fresh.BreakfastPausedUntil, until)
	}
}

// TestMealPause_ClearWithNil POST 暂停传 until=null 取消暂停。
func TestMealPause_ClearWithNil(t *testing.T) {
	e := newTestEnv(t)
	until := time.Now().Add(7 * 24 * time.Hour).Unix()
	e.pauseMeal("dinner", &until)
	ds := e.pauseMeal("dinner", nil)
	if ds.DinnerPausedUntil != nil {
		t.Errorf("DinnerPausedUntil should be nil after clear, got %v", *ds.DinnerPausedUntil)
	}
}

// TestMealPause_InvalidMealRejected POST /api/state/meal/brunch/pause 400。
func TestMealPause_InvalidMealRejected(t *testing.T) {
	e := newTestEnv(t)
	until := time.Now().Add(7 * 24 * time.Hour).Unix()
	status, _ := e.post("/api/state/meal/brunch/pause", map[string]any{"until": until})
	if status != 400 {
		t.Errorf("invalid meal status = %d, want 400", status)
	}
}

// TestMealPause_NotificationDismissCascade 这是核心双向绑定测试:
// 1. 当前在某 meal 区间内 → POST /api/state/meal POST → 触发 GenerateMealNotifications
//    (实际上不需要 — POST /api/state/today set mode=normal 后 60s ticker 才跑, 但暂停 handler
//    会立即 kick GenerateMealNotifications)
// 2. 直接调用 GenerateMealNotifications path 先创建通知 (跳过等 ticker), 然后暂停 → 通知 dismiss
// 3. 取消暂停 → 通知复活
//
// 由于 GenerateMealNotifications 用 time.Now(), 测试依赖 wall-clock 是否在某 meal 区间内。
// 不在区间内时 skip — 仍能在 CI / 用户开发机大部分时间触发到。
func TestMealPause_NotificationDismissCascade(t *testing.T) {
	meal, inRange := nowInMealRange()
	if !inRange {
		t.Skip("wall clock not in any meal range, skip lifecycle test")
	}
	e := newTestEnv(t)

	// 1. 先用 POST /api/state/meal POST 不带 clear 触发一次生成
	//    注意: 这条 POST 用当前时刻记录该 meal, 紧接着 GenerateMealNotifications 看到
	//    recordedAt != "" 会 dismiss; 我们要的是"未记录 + 在区间内"场景。
	//    改走: 触发 mode=normal (默认就是 normal) → 直接 POST pause 取消暂停初始态。
	//    实际上 setMode 不会 kick meal notif; 用 explicit 调 backend 内部 helper 不易,
	//    最简单: post /api/state/meal/{meal}/pause with until=null 取消默认 nil 状态,
	//    handler 内部会 GenerateMealNotifications, 通知就 active 起来了。
	zero := int64(0)
	_ = zero
	ds := e.pauseMeal(meal, nil) // null until: 不暂停 + 触发一次 generate
	_ = ds

	ref := mealRef(meal, e.today)
	n := e.notifByRef(ref)
	if n == nil {
		t.Fatalf("ref %s: notif should exist after unpause kick", ref)
	}
	if n.DismissedAt != nil {
		t.Fatalf("ref %s: notif should be active, got dismissed_at=%d", ref, *n.DismissedAt)
	}

	// 2. 暂停 → 通知 dismiss (cascade)
	until := time.Now().Add(7 * 24 * time.Hour).Unix()
	e.pauseMeal(meal, &until)
	e.assertNotifDismissed(ref, true)

	// 3. 取消暂停 → 通知复活 (handler 内部 undismiss + GenerateMealNotifications)
	e.pauseMeal(meal, nil)
	e.assertNotifDismissed(ref, false)
}

