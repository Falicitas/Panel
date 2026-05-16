package integration

import (
	"strings"
	"testing"

	"panel/store"
)

// 复现路径: 用户首次进入普通模式时, backend 把 free kind 复发 todo 注入 micro 顶部。
// morning kind 已做成晨间 CommitRitualView 蒙版, phone kind 做成睡前 ritual 蒙版,
// 两者都有蒙版承接, 不再注入 micro md (早期 morning 也注入, 改蒙版后成冗余, 去掉)。
// 本测试锁死: 注入 micro 的只有 free, morning + phone 都不进。
func TestInjectRecurringOnlyFreeKind(t *testing.T) {
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
	mustAdd(store.RecurringKindFree, "整理桌面")

	// 首次 POST /api/state/today {mode=normal} → 触发 injectRecurringIntoMicro
	status, body := e.post("/api/state/today", map[string]any{
		"mode":      "normal",
		"wake_time": "06:30",
	})
	e.mustOK("POST", "/api/state/today", status, body)

	// 验证 micro 内容: 只含 free, 不含 morning / phone
	status, body = e.get("/api/micro/today")
	e.mustOK("GET", "/api/micro/today", status, body)

	got := string(body)
	for _, want := range []string{"做拉伸", "整理桌面"} {
		if !strings.Contains(got, want) {
			t.Errorf("micro should contain free item %q, got: %s", want, quote(got))
		}
	}
	for _, notWant := range []string{"起床喝水", "吃早餐", "放下手机", "刷牙洗澡"} {
		if strings.Contains(got, notWant) {
			t.Errorf("micro should NOT contain morning/phone item %q (only free is injected — "+
				"morning/phone have ritual masks), got: %s", notWant, quote(got))
		}
	}
}

// 边界: 全是 morning/phone (无 free) 时, 首次进普通模式不该往 micro 写任何内容,
// micro 当日文件保持空 (filtered 为空 → injectRecurringIntoMicro early return)。
func TestInjectRecurringNoFreeLeavesMicroEmpty(t *testing.T) {
	e := newTestEnv(t)

	if _, err := e.store.CreateRecurring("起床喝水", store.RecurringKindMorning); err != nil {
		t.Fatalf("create morning: %v", err)
	}
	if _, err := e.store.CreateRecurring("放下手机", store.RecurringKindPhone); err != nil {
		t.Fatalf("create phone: %v", err)
	}

	status, body := e.post("/api/state/today", map[string]any{
		"mode":      "normal",
		"wake_time": "06:30",
	})
	e.mustOK("POST", "/api/state/today", status, body)

	status, body = e.get("/api/micro/today")
	e.mustOK("GET", "/api/micro/today", status, body)

	got := string(body)
	for _, notWant := range []string{"起床喝水", "放下手机"} {
		if strings.Contains(got, notWant) {
			t.Errorf("micro should stay empty (no free kind), but contains %q, got: %s",
				notWant, quote(got))
		}
	}
}
