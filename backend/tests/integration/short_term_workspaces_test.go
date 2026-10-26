package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"panel/dualnotes"
	"panel/server"
	"panel/store"
)

// The monthly workspace shares the independent notes process and remains
// usable when the main backend is stopped, without changing daily notes.
func TestWorkspace_IndependentServiceAndDailyNotesIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	mainStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mainStore.Close() })
	wantDay, err := mainStore.SaveDualNoteDay(context.Background(), "2026-10-26", 0, []store.DualNoteRow{{ID: "daily", Left: "每日双栏", Right: "独立保留"}})
	if err != nil {
		t.Fatal(err)
	}
	mainServer := httptest.NewServer(server.New(nil, mainStore, nil).Routes())
	t.Cleanup(mainServer.Close)
	notesStore, err := store.OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = notesStore.Close() })
	notesServer := httptest.NewServer(dualnotes.New(notesStore).Routes())
	t.Cleanup(notesServer.Close)
	workspacePath := "/api/short-term-workspaces/2026-10"
	notesRequest(t, mainServer.URL, "GET", workspacePath, "", http.StatusNotFound)
	notesRequest(t, mainServer.URL, "PUT", workspacePath, `{"revision":0,"tables":[]}`, http.StatusNotFound)
	mainServer.Close()
	if err := mainStore.Close(); err != nil {
		t.Fatal(err)
	}
	body := `{"revision":0,"tables":[{"id":"one","header":"本月任务","rows":[{"id":"row","content":"主服务关闭后保存"}]},{"id":"two","header":"待办","rows":[]}]}`
	saved := notesRequest(t, notesServer.URL, "PUT", workspacePath, body, http.StatusOK)
	var want store.MonthWorkspace
	if err := json.Unmarshal(saved, &want); err != nil {
		t.Fatal(err)
	}
	if want.Revision != 1 || len(want.Tables) != 2 {
		t.Fatalf("wrong saved workspace: %s", saved)
	}
	notesRequest(t, notesServer.URL, "PUT", workspacePath, body, http.StatusConflict)
	gotJSON := notesRequest(t, notesServer.URL, "GET", workspacePath, "", http.StatusOK)
	var got store.MonthWorkspace
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conflict changed workspace: %+v", got)
	}
	dailyJSON := notesRequest(t, notesServer.URL, "GET", "/api/dual-notes?end=2026-10-26&days=1", "", http.StatusOK)
	var page struct {
		Days []store.DualNoteDay `json:"days"`
	}
	if err := json.Unmarshal(dailyJSON, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 1 || !reflect.DeepEqual(page.Days[0], wantDay) {
		t.Fatalf("workspace save changed daily notes: %s", dailyJSON)
	}
	// Closing and reopening the independent store preserves monthly content.
	notesServer.Close()
	if err := notesStore.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.GetMonthWorkspace(context.Background(), "2026-10")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("restart lost workspace: %+v, %v", got, err)
	}
}
