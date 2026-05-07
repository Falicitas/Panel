// Package integration 提供集成测试基础设施: 起一个真 sqlite + httptest server,
// 用 http.Client 模拟 mac client / curl 调 API, 断言响应 + db 状态。
//
// 跟单元测试 (各 package 内 _test.go) 互补:
//   - 单测: 单个函数 / store helper, 用 db mock 或 minimal db
//   - 集成测试: 跨 store/server/supplements 包, 全链路 API → db 状态
//
// 凡是涉及通知 lifecycle (dismiss/undismiss/cascade)、跨包状态机、issue 修复时
// 用 bash+curl 验证过的复现路径, 都该沉淀成这里的 test func — 防 LLM 在 context
// 压缩后丢失测试方法。
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panel/dateutil"
	"panel/micro"
	"panel/server"
	"panel/store"
	"panel/todolist"
)

// testEnv 一个隔离的测试环境: temp dir 起 sqlite + 起 httptest server + http.Client。
// t.Cleanup 自动关 server / 关 db。每个 test func 调一次 newTestEnv 拿独立环境。
type testEnv struct {
	t      *testing.T
	srv    *httptest.Server
	store  *store.Store
	client *http.Client
	today  string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tlDir := filepath.Join(dir, "todolists")
	microDir := filepath.Join(dir, "micro")
	repo := todolist.NewRepo(tlDir)
	microRepo := micro.NewRepo(microDir)
	// repo.Watch 用 fsnotify 起 goroutine 监听 — 集成测试不需要 watcher,
	// 直接给一个 cancelled ctx 让它 noop (Watch 内部会因 ctx 取消快速返回)。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = repo.Watch(ctx)

	srv := server.New(repo, st, microRepo)
	httpServer := httptest.NewServer(srv.Routes())
	t.Cleanup(httpServer.Close)

	return &testEnv{
		t:      t,
		srv:    httpServer,
		store:  st,
		client: &http.Client{Timeout: 5 * time.Second},
		today:  dateutil.LogicalToday(time.Now()),
	}
}

// do 发起任意 method + body 的 HTTP 请求, 返回 status + body bytes。失败直接 t.Fatal。
func (e *testEnv) do(method, path string, body any) (int, []byte) {
	e.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal %s body: %v", path, err)
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, bodyReader)
	if err != nil {
		e.t.Fatalf("new req %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (e *testEnv) post(path string, body any) (int, []byte) { return e.do("POST", path, body) }
func (e *testEnv) put(path string, body any) (int, []byte)  { return e.do("PUT", path, body) }
func (e *testEnv) get(path string) (int, []byte)            { return e.do("GET", path, nil) }

// mustOK 断言 status 在 [200, 300), 否则 t.Fatal 带 body。
func (e *testEnv) mustOK(method, path string, status int, body []byte) {
	e.t.Helper()
	if status < 200 || status >= 300 {
		e.t.Fatalf("%s %s: status %d body=%s", method, path, status, string(body))
	}
}

// notifByRef 直接读 db 拿 source_ref 对应的通知 (含已 dismissed)。返回 nil 表示不存在。
func (e *testEnv) notifByRef(ref string) *store.Notification {
	e.t.Helper()
	notifs, err := e.store.ListNotifications(true) // include dismissed
	if err != nil {
		e.t.Fatalf("ListNotifications: %v", err)
	}
	for i := range notifs {
		if notifs[i].SourceRef == ref {
			return &notifs[i]
		}
	}
	return nil
}

// assertNotifBody 断言 ref 通知存在且 body == want; nil 通知 / body 不符 直接 t.Fatal。
func (e *testEnv) assertNotifBody(ref, want string) {
	e.t.Helper()
	n := e.notifByRef(ref)
	if n == nil {
		e.t.Fatalf("ref %s: notif missing", ref)
	}
	if n.Body != want {
		e.t.Fatalf("ref %s: body=%q want=%q", ref, n.Body, want)
	}
}

// assertNotifDismissed 断言 ref 通知存在且 dismissed_at 是 / 不是 NULL。
func (e *testEnv) assertNotifDismissed(ref string, dismissed bool) {
	e.t.Helper()
	n := e.notifByRef(ref)
	if n == nil {
		e.t.Fatalf("ref %s: notif missing", ref)
	}
	got := n.DismissedAt != nil
	if got != dismissed {
		e.t.Fatalf("ref %s: dismissed=%v want=%v", ref, got, dismissed)
	}
}

// hmFromNow 当前时刻 + offset 分钟, 返回 "HH:MM"。
// 用于构造 "覆盖当前" / "已过期" / "未来" 这类相对时段的测试。
func hmFromNow(offsetMin int) string {
	t := time.Now().Add(time.Duration(offsetMin) * time.Minute)
	return t.Format("15:04")
}

// supplementRef supplement 通知 source_ref 拼接, 跟 supplements/supplements.go 内部一致。
func supplementRef(spID int64, winIdx int, date string) string {
	return fmt.Sprintf("supplement:%d:window:%d:date:%s", spID, winIdx, date)
}

// englishRef english 通知 source_ref 拼接, 跟 english/notifications.go 内部一致。
func englishRef(winIdx int, date string) string {
	return fmt.Sprintf("english:window:%d:date:%s", winIdx, date)
}

// mealRef meal 通知 source_ref 拼接, 跟 supplements/meal.go 内部一致。
func mealRef(key, date string) string {
	return fmt.Sprintf("meal:%s:date:%s", key, date)
}

// quote 把字符串转成可打印的形式 (调试 helper)。
func quote(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// seedEnglishAudioOpts 控制 seedEnglishAudio 的可选字段, 默认行为接近 "已下完未归档"。
type seedEnglishAudioOpts struct {
	Source      string
	Ref         string
	Title       string
	Description string
	Topic       string
	DurationMs  int64
	PublishedAt int64
	LocalPath   string // 空字符串 → 显式置 ""(模拟"没下完", 候选会跳过)
	Archived    int64  // > 0 → 写 archived_at, 0 → 未归档
}

// seedEnglishAudio 把一个 audio 塞进 db; 返回插入 id。失败 t.Fatal。
// 用于测试构造场景: candidate / skip / lifecycle 等。
func (e *testEnv) seedEnglishAudio(o seedEnglishAudioOpts) int64 {
	e.t.Helper()
	if o.Source == "" {
		o.Source = "npr"
	}
	if o.Ref == "" {
		e.t.Fatalf("seedEnglishAudio: Ref required")
	}
	if o.Title == "" {
		o.Title = "Title " + o.Ref
	}
	if o.PublishedAt == 0 {
		o.PublishedAt = 1700000000
	}
	if o.DurationMs == 0 {
		o.DurationMs = 600_000 // 10min default
	}
	a := store.EnglishAudio{
		Source:      o.Source,
		SourceRef:   o.Ref,
		Title:       o.Title,
		Description: o.Description,
		PublishedAt: o.PublishedAt,
		DurationMs:  o.DurationMs,
		AudioURL:    "http://example.test/" + o.Ref + ".mp3",
		LocalPath:   o.LocalPath,
		FetchedAt:   o.PublishedAt + 10,
		Topic:       o.Topic,
	}
	id, _, err := e.store.InsertAudioIfNew(a)
	if err != nil {
		e.t.Fatalf("seedEnglishAudio %s: %v", o.Ref, err)
	}
	// InsertAudioIfNew 已处理 LocalPath ("" → NULL, 非空 → 字符串), 不需二次 set。
	if o.Archived > 0 {
		if err := e.store.SetAudioArchived(id, o.Archived); err != nil {
			e.t.Fatalf("seedEnglishAudio %s archive: %v", o.Ref, err)
		}
	}
	return id
}
