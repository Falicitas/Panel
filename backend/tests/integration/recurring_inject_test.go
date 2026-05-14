package integration

import (
	"strings"
	"testing"

	"panel/store"
)

// 复现路径: 用户首次进入普通模式时, backend 把 recurring 注入 micro 顶部;
// kind=phone 的项专用于睡前蒙版, 不该出现在 micro 里。
// (panel-pool-1 issue-49 引入 phone kind 时的回归测试 — 防 inject 又把 phone 误注入。)
func TestInjectRecurringSkipsPhoneKind(t *testing.T) {
	e := newTestEnv(t)

	mustAdd := func(kind, content string) {
		t.Helper()
		if _, err := e.store.CreateRecurring(content, kind); err != nil {
			t.Fatalf("create %s/%s: %v", kind, content, err)
		}
	}
	mustAdd(store.RecurringKindMorning, "起床喝水")
	mustAdd(store.RecurringKindMorning, "吃早餐")
	mustAdd(store.RecurringKindPhone, "放下手机")
	mustAdd(store.RecurringKindPhone, "刷牙洗澡")
	mustAdd(store.RecurringKindFree, "做拉伸")

	// 首次 POST /api/state/today {mode=normal} → 触发 injectRecurringIntoMicro
	status, body := e.post("/api/state/today", map[string]any{
		"mode":      "normal",
		"wake_time": "06:30",
	})
	e.mustOK("POST", "/api/state/today", status, body)

	// 验证 micro 内容: 含 morning + free, 不含 phone
	status, body = e.get("/api/micro/today")
	e.mustOK("GET", "/api/micro/today", status, body)

	got := string(body)
	for _, want := range []string{"起床喝水", "吃早餐", "做拉伸"} {
		if !strings.Contains(got, want) {
			t.Errorf("micro should contain %q, got: %s", want, quote(got))
		}
	}
	for _, notWant := range []string{"放下手机", "刷牙洗澡"} {
		if strings.Contains(got, notWant) {
			t.Errorf("micro should NOT contain phone item %q (only morning + free should be injected), got: %s",
				notWant, quote(got))
		}
	}
}
