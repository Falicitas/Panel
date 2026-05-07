package integration

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"panel/store"
)

// 这个文件覆盖 supplement panel 通知的 lifecycle, 每个 sub-test 对应历史上 user 在
// 某个 issue 报过的 bug 复现路径。issue ref 写在测试名 + 注释里 — context 压缩后
// LLM 也能直接 grep 到对应历史 commit。
//
// 共同 fixture:
//   - newTestEnv 起独立 sqlite + httptest server
//   - 创建一个 supplement (helper createTestSupplement) 拿 id
//   - 操作 PUT/POST 各 endpoint 模拟 mac client
//   - 断言 db 里 notifications 表 source_ref 对应行的 body / dismissed_at

// createTestSupplement 创建一个测试 sp (默认 windows = 现在覆盖的活跃区间)
// 返回 id。caller 后续 PUT/snooze/check 用此 id。
func createTestSupplement(t *testing.T, e *testEnv, name string, windows []store.Window, mode string) int64 {
	t.Helper()
	body := map[string]any{
		"name":            name,
		"type":            "supplement",
		"windows":         windows,
		"notes":           "",
		"dose":            "",
		"enabled":         true,
		"completion_mode": mode,
	}
	status, raw := e.post("/api/supplements", body)
	e.mustOK("POST", "/api/supplements", status, raw)
	var sp store.Supplement
	if err := json.Unmarshal(raw, &sp); err != nil {
		t.Fatalf("unmarshal sp: %v body=%s", err, raw)
	}
	return sp.ID
}

// TestSupplement_BasicActiveTriggerNotif: 最基础 case —— 创建 sp 时 window 已 active
// (start = now-5min, end = now+5min) 应自动生成 panel 通知, body = "保健品 时段 X-Y"
// 且 dismissed_at = NULL。
func TestSupplement_BasicActiveTriggerNotif(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	n := e.notifByRef(ref)
	if n == nil {
		t.Fatal("notif should be created when window active")
	}
	if !strings.Contains(n.Body, "保健品 时段") {
		t.Errorf("body = %q, want contains '保健品 时段'", n.Body)
	}
	if n.DismissedAt != nil {
		t.Errorf("notif should be active (dismissed_at = NULL), got %v", n.DismissedAt)
	}
}

// TestSupplement_DeleteWindowDismissesNotif: issue-25 06db069 修复
// 复现路径 (user 报):
//   1. PUT sp 加一个覆盖当前的 window → 通知出现
//   2. 立刻 PUT 删该 window (windows = []) → 通知该 dismiss
// 老 bug: expireUnfinishedSupplementNotifications 看 idx 越界仅 continue 不 dismiss,
// 通知 stale 留在通知栏。
func TestSupplement_DeleteWindowDismissesNotif(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	e.assertNotifDismissed(ref, false)

	// PUT 删该 window
	body := map[string]any{
		"name": "维C", "type": "supplement", "windows": []store.Window{},
		"notes": "", "dose": "", "enabled": true, "completion_mode": "all",
	}
	status, raw := e.put("/api/supplements/"+itoa(id), body)
	e.mustOK("PUT", "/api/supplements/"+itoa(id), status, raw)

	e.assertNotifDismissed(ref, true)
}

// TestSupplement_SnoozeBodySync: issue-25 4878dae + 4c271a4 修复
// 复现路径 (user 报):
//   1. window active → 通知 body "保健品 时段 17:50-18:20"
//   2. snooze count=1 → effective end +30min → body 应同步到 "17:50-18:50"
//   3. snooze count=4 → 再 +2h → body 应同步到 "17:50-20:50"
// 老 bug 1: GenerateNotifications inner loop body 公式没读 snoozeMap, 永远 anchor-only 时段。
// 老 bug 2: handleSnoozeSupplement 写完 records 不 kick GenerateNotifications, 等 60s ticker。
func TestSupplement_SnoozeBodySync(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	initN := e.notifByRef(ref)
	if initN == nil {
		t.Fatal("init notif missing")
	}
	t.Logf("init body: %s", initN.Body)

	// snooze count=1 → end +30min
	status, raw := e.post("/api/supplements/"+itoa(id)+"/snooze",
		map[string]any{"date": e.today, "window_index": 0, "count": 1})
	e.mustOK("POST snooze", "snooze", status, raw)

	expEnd1 := addMinutesHM(hmFromNow(5), 30)
	expBody1 := "保健品 时段 " + hmFromNow(-5) + "–" + expEnd1
	e.assertNotifBody(ref, expBody1)
	e.assertNotifDismissed(ref, false)

	// snooze count=4 → 再 +2h
	status, raw = e.post("/api/supplements/"+itoa(id)+"/snooze",
		map[string]any{"date": e.today, "window_index": 0, "count": 4})
	e.mustOK("POST snooze 2h", "snooze", status, raw)

	expEnd2 := addMinutesHM(hmFromNow(5), 30+120)
	expBody2 := "保健品 时段 " + hmFromNow(-5) + "–" + expEnd2
	e.assertNotifBody(ref, expBody2)
}

// TestSupplement_ReactivateUndismiss: issue-25 4c271a4 修复
// 复现路径 (user 报):
//   1. PUT sp 加一个 window 16:00-17:00 (假设 17 点已过, expire 函数会 dismiss 通知)
//   2. PUT 改 window 到当前活跃区间 → 通知应 undismiss + body sync 到新时段
// 老 bug: CreateNotificationIdempotent INSERT IGNORE, 同 ref dismissed 跳过, 仅 UPDATE body
// 不动 dismissed_at → panel 通知列表不显示 (ListNotifications(false) 滤 IS NULL)。
func TestSupplement_ReactivateUndismiss(t *testing.T) {
	e := newTestEnv(t)
	// 先创建一个老 window (-60min ~ -30min, 已过期)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-60), End: hmFromNow(-30)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	// 模拟 "通知存在 + dismissed" 的状态: 直接 SQL 写一条 dismissed notif
	now := time.Now().Unix()
	if _, err := e.store.UpsertRecord(store.Record{}); err == nil {
		// 没用, 占位防 unused import; 直接用 CreateNotificationIdempotent 插
	}
	dismissedAt := now - 100
	_, _ = e.store.CreateNotificationIdempotent(store.Notification{
		Source: "supplement", SourceRef: ref,
		Title: "维C", Body: "保健品 时段 " + hmFromNow(-60) + "–" + hmFromNow(-30),
		Level: "info", CreatedAt: now - 200,
	})
	_ = e.store.DismissNotificationByRef(ref, dismissedAt)
	e.assertNotifDismissed(ref, true)

	// PUT 改 window 到当前活跃 → 应触发 inner loop undismiss + UPDATE body
	body := map[string]any{
		"name": "维C", "type": "supplement",
		"windows":         []store.Window{{Start: hmFromNow(-5), End: hmFromNow(5)}},
		"notes":           "", "dose": "", "enabled": true,
		"completion_mode": "all",
	}
	status, raw := e.put("/api/supplements/"+itoa(id), body)
	e.mustOK("PUT", "PUT sp", status, raw)

	e.assertNotifDismissed(ref, false) // 复活
	expBody := "保健品 时段 " + hmFromNow(-5) + "–" + hmFromNow(5)
	e.assertNotifBody(ref, expBody)
}

// TestSupplement_ReactivateSkipDone: issue-25 4c271a4 修复 (后续防御)
// 已勾完成的 window 不该被 inner loop 自动 undismiss 复活 — 用户主动完成 dismiss
// 的语义要尊重。
func TestSupplement_ReactivateSkipDone(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	// 勾该 window 完成 → DismissNotificationByRef
	status, raw := e.post("/api/supplements/"+itoa(id)+"/check",
		map[string]any{"date": e.today, "window_index": 0, "checked": true})
	e.mustOK("POST check", "check", status, raw)
	e.assertNotifDismissed(ref, true)

	// 触发一次 GenerateNotifications (PUT 改 notes 走 update path)
	body := map[string]any{
		"name": "维C", "type": "supplement",
		"windows":         []store.Window{{Start: hmFromNow(-5), End: hmFromNow(5)}},
		"notes":           "edited", "dose": "", "enabled": true,
		"completion_mode": "all",
	}
	status, raw = e.put("/api/supplements/"+itoa(id), body)
	e.mustOK("PUT", "PUT sp", status, raw)

	// 通知仍 dismissed (done win 不复活)
	e.assertNotifDismissed(ref, true)
}

// TestSupplement_AnyModeAllDismiss: issue-21 dec40cb (any 模式) 行为
// completion_mode="any" 下勾任一 window → 整 item 当日所有 window 通知 dismiss。
func TestSupplement_AnyModeAllDismiss(t *testing.T) {
	e := newTestEnv(t)
	// 三个 window 都 active (跨 now 前后 ±10min)
	id := createTestSupplement(t, e, "复合维生素", []store.Window{
		{Start: hmFromNow(-15), End: hmFromNow(-10)}, // passed
		{Start: hmFromNow(-5), End: hmFromNow(5)},    // active
		{Start: hmFromNow(10), End: hmFromNow(20)},   // upcoming
	}, "any")
	ref0 := supplementRef(id, 0, e.today)
	ref1 := supplementRef(id, 1, e.today)
	ref2 := supplementRef(id, 2, e.today)

	// 仅 idx=1 active 时被 INSERT 通知; idx=0 已过, idx=2 还没到, 都没 INSERT
	if e.notifByRef(ref1) == nil {
		t.Fatal("ref1 (active) should have notif")
	}

	// 勾 idx=1 → any 模式 dismiss item 当日所有 window 通知
	status, raw := e.post("/api/supplements/"+itoa(id)+"/check",
		map[string]any{"date": e.today, "window_index": 1, "checked": true})
	e.mustOK("POST check", "check", status, raw)

	if n := e.notifByRef(ref1); n == nil || n.DismissedAt == nil {
		t.Errorf("ref1 should be dismissed after any-mode check; got %+v", n)
	}
	// ref0 / ref2 本来就没生成, 无 row 算正常 (any 模式不会回头补发)
	if n := e.notifByRef(ref0); n != nil && n.DismissedAt == nil {
		t.Errorf("ref0 should not be active; got %+v", n)
	}
	_ = ref2
}

// TestSupplement_PauseSkipsActiveNotif: 暂停期内不生成新通知。
func TestSupplement_PauseSkipsActiveNotif(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	// active 时通知已存在, dismiss 它
	_ = e.store.DismissNotificationByRef(ref, time.Now().Unix())

	// pause 1 天
	until := time.Now().AddDate(0, 0, 1).Unix()
	status, raw := e.post("/api/supplements/"+itoa(id)+"/pause",
		map[string]any{"until": until})
	e.mustOK("POST pause", "pause", status, raw)

	// kick 后 GenerateNotifications 看 IsActive=false → 跳过该 sp inner loop
	// → ref 仍 dismissed (没被 undismiss 复活)
	e.assertNotifDismissed(ref, true)
}

// TestSupplement_PauseDismissesActiveNotif: issue-34
// 复现路径 (user 报):
//   1. 食物 item (杏仁) window active → 通知 INSERT (dismissed_at = NULL)
//   2. user 在 mac UI 选 "暂停一天" → POST /pause until=明天
//   3. 通知应被自动 dismiss, 通知栏不该再挂这个 active 时段
// 老 bug: expireUnfinishedSupplementNotifications 仅处理 (a) 已过期 (b) 锚点退到 window 之前
//
//	(c) winIdx 越界 (d) sp 不存在 (e) 引用未触发餐. 不处理 paused 状态.
//	GenerateNotifications inner loop 看 IsActive=false 直接 continue, 已存在的
//	active 通知没人 dismiss, 一直挂着.
//
// 修法: expire 函数加 sp.IsActive(now)=false 分支 → DismissNotificationByRef.
// 同时也兜底了 PUT update enabled=false 路径 (handleEnableSupplement disable
// 是硬 DELETE 通知, PUT 走 UpdateSupplement 不会触发 cascade, 老逻辑有同样的
// 通知 stale 问题).
func TestSupplement_PauseDismissesActiveNotif(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "杏仁", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	// 创建时 active → 通知应 INSERT 且未 dismissed
	e.assertNotifDismissed(ref, false)

	// pause 1 天
	until := time.Now().AddDate(0, 0, 1).Unix()
	status, raw := e.post("/api/supplements/"+itoa(id)+"/pause",
		map[string]any{"until": until})
	e.mustOK("POST pause", "pause", status, raw)

	// kick 后 expire 函数应看到 sp.IsActive=false → dismiss 通知
	e.assertNotifDismissed(ref, true)
}

// TestSupplement_PutDisableDismissesActiveNotif: issue-34 同源
// PUT update sp 把 enabled=false (跟 POST /enable disable 不是同一路径) 时,
// 老逻辑只让 inner loop 看 IsActive=false 跳过, 不 dismiss 已存在的 active 通知.
// 修法 (expire 函数 IsActive 分支) 同时覆盖此 case.
func TestSupplement_PutDisableDismissesActiveNotif(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "维C", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	e.assertNotifDismissed(ref, false)

	// PUT 把 enabled 改成 false (走 handleUpdateSupplement, 不走 disable 硬删路径)
	body := map[string]any{
		"name": "维C", "type": "supplement",
		"windows":         []store.Window{{Start: hmFromNow(-5), End: hmFromNow(5)}},
		"notes":           "", "dose": "", "enabled": false,
		"completion_mode": "all",
	}
	status, raw := e.put("/api/supplements/"+itoa(id), body)
	e.mustOK("PUT", "PUT sp", status, raw)

	e.assertNotifDismissed(ref, true)
}

// TestSupplement_UnpauseUndismissesIfStillActive: issue-34
// 暂停 → 取消暂停后, 若 window 仍在 active 区间, 通知应 undismiss 复活
// (跟 inner loop UndismissNotificationByRef 路径联动).
// 防御性测试: 确保 issue-34 fix (expire 里 IsActive=false dismiss) 没把
// "短暂暂停后再恢复" 路径搞坏 — 通知不该永久 dismissed.
func TestSupplement_UnpauseUndismissesIfStillActive(t *testing.T) {
	e := newTestEnv(t)
	id := createTestSupplement(t, e, "杏仁", []store.Window{
		{Start: hmFromNow(-5), End: hmFromNow(5)},
	}, "all")
	ref := supplementRef(id, 0, e.today)
	e.assertNotifDismissed(ref, false)

	// pause → dismiss
	until := time.Now().AddDate(0, 0, 1).Unix()
	status, raw := e.post("/api/supplements/"+itoa(id)+"/pause",
		map[string]any{"until": until})
	e.mustOK("POST pause", "pause", status, raw)
	e.assertNotifDismissed(ref, true)

	// unpause (until=null) → kick regen → inner loop 看 IsActive=true
	// → CreateNotificationIdempotent 跳过 (ref 存在) → UpdateBody +
	//   UndismissNotificationByRef → 通知应复活
	status, raw = e.post("/api/supplements/"+itoa(id)+"/pause",
		map[string]any{"until": nil})
	e.mustOK("POST unpause", "unpause", status, raw)
	e.assertNotifDismissed(ref, false)
}

// --- helpers ---

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// addMinutesHM "HH:MM" + 分钟 → "HH:MM" (跨午夜 mod 24h)。
func addMinutesHM(hm string, minutes int) string {
	parts := strings.SplitN(hm, ":", 2)
	if len(parts) != 2 {
		return hm
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	total := (h*60 + m + minutes) % (24 * 60)
	if total < 0 {
		total += 24 * 60
	}
	return formatHM(total)
}

func formatHM(totalMin int) string {
	return strings.Join([]string{
		zeroPad2(totalMin / 60),
		zeroPad2(totalMin % 60),
	}, ":")
}

func zeroPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
