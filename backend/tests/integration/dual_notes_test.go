package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"panel/dualnotes"
	"panel/server"
	"panel/store"
)

func notesRequest(t *testing.T, baseURL, method, path, body string, wantStatus int) []byte {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s: status=%d, want=%d, body=%s", method, path, response.StatusCode, wantStatus, data)
	}
	return data
}

// TestDualNotes_ContinuesAfterPanelBackendShutdown reproduces issue-66:
// stop the main backend and close its database connection, then keep reading
// and saving existing notes through the independent dual-notes service.
func TestDualNotes_ContinuesAfterPanelBackendShutdown(t *testing.T) {
	dataDir := t.TempDir()
	legacy, err := store.Open(filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = legacy.Close() })
	rows := []store.DualNoteRow{{ID: "existing", Left: "历史笔记\n\n```go\nold()\n```", Right: "原有内容"}}
	for revision := int64(0); revision < 2; revision++ {
		if _, err := legacy.SaveDualNoteDay(context.Background(), "2026-10-21", revision, rows); err != nil {
			t.Fatal(err)
		}
	}
	mainServer := httptest.NewServer(server.New(nil, legacy, nil).Routes())
	t.Cleanup(mainServer.Close)
	notesStore, err := store.OpenDualNotes(filepath.Join(dataDir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = notesStore.Close() })
	notesServer := httptest.NewServer(dualnotes.New(notesStore).Routes())
	t.Cleanup(notesServer.Close)
	notesURL := notesServer.URL
	health := notesRequest(t, notesURL, "GET", "/api/health", "", http.StatusOK)
	if !strings.Contains(string(health), `"service":"panel-dual-notes"`) {
		t.Fatalf("wrong service: %s", health)
	}
	notesRequest(t, mainServer.URL, "GET", "/api/dual-notes?end=2026-10-21&days=1", "", http.StatusNotFound)
	notesRequest(t, mainServer.URL, "PUT", "/api/dual-notes/2026-10-21", `{"revision":2,"rows":[]}`, http.StatusNotFound)
	mainServer.Close()
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	// With the main backend stopped, the old content and revision remain intact.
	raw := notesRequest(t, notesURL, "GET", "/api/dual-notes?end=2026-10-21&days=1", "", http.StatusOK)
	var page struct {
		Days []store.DualNoteDay `json:"days"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	want := store.DualNoteDay{Date: "2026-10-21", Revision: 2, Rows: rows}
	if len(page.Days) != 1 || !reflect.DeepEqual(page.Days[0], want) {
		t.Fatalf("legacy notes changed: %s", raw)
	}
	for _, path := range []string{"/api/notifications", "/api/english/today", "/api/state/today", "/api/todolists"} {
		notesRequest(t, notesURL, "GET", path, "", http.StatusNotFound)
	}
	notesRequest(t, notesURL, "POST", "/api/exec", `{"cmd":"echo unwanted"}`, http.StatusNotFound)
	saved := notesRequest(t, notesURL, "PUT", "/api/dual-notes/2026-10-21", `{"revision":2,"rows":[{"id":"new","left":"主服务关闭后保存","right":"独立服务"}]}`, http.StatusOK)
	notesRequest(t, notesURL, "PUT", "/api/dual-notes/2026-10-21", `{"revision":2,"rows":[]}`, http.StatusConflict)
	if !strings.Contains(string(saved), `"revision":3`) {
		t.Fatalf("revision was reset: %s", saved)
	}
	raw = notesRequest(t, notesURL, "GET", "/api/dual-notes?end=2026-10-21&days=1", "", http.StatusOK)
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 1 || page.Days[0].Revision != 3 || len(page.Days[0].Rows) != 1 || page.Days[0].Rows[0].Left != "主服务关闭后保存" {
		t.Fatalf("failed save or conflict overwrote content: %s", raw)
	}
}
