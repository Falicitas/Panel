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
	"sync/atomic"
	"testing"
	"time"

	"panel/dualnotes"
	"panel/server"
	"panel/store"
)

// The response is deliberately disconnected after committing the copy. Retrying
// the same operation after restarting the independent service must append once.
func TestWorkspaceCopy_LostResponseAndServiceRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	mainStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mainStore.Close() })
	source, err := mainStore.SaveMonthWorkspace(context.Background(), "2026-09", 0, []store.WorkspaceTable{{ID: "source", Header: "要迁入的表", Rows: []store.WorkspaceRow{{ID: "row", Content: "源表正文"}}}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := mainStore.SaveMonthWorkspace(context.Background(), "2026-10", 0, []store.WorkspaceTable{{ID: "keep", Header: "当月原有", Rows: []store.WorkspaceRow{}}})
	if err != nil {
		t.Fatal(err)
	}
	mainServer := httptest.NewServer(server.New(nil, mainStore, nil).Routes())
	t.Cleanup(mainServer.Close)
	body := `{"operationId":"lost-response","targetMonth":"2026-10","sourceRevision":1,"targetRevision":1,"tableIds":["source"]}`
	copyPath := "/api/short-term-workspaces/2026-09/copy"
	notesRequest(t, mainServer.URL, "POST", copyPath, body, http.StatusNotFound)
	mainServer.Close()
	if err := mainStore.Close(); err != nil {
		t.Fatal(err)
	}

	notesStore, err := store.OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = notesStore.Close() })
	routes := dualnotes.New(notesStore).Routes()
	var lost atomic.Bool
	notesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == copyPath && lost.CompareAndSwap(false, true) {
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, r)
			if recorder.Code == http.StatusOK {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
			for key, values := range recorder.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorder.Code)
			_, _ = io.Copy(w, recorder.Body)
			return
		}
		routes.ServeHTTP(w, r)
	}))
	t.Cleanup(notesServer.Close)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(notesServer.URL+copyPath, "application/json", strings.NewReader(body))
	if response != nil {
		response.Body.Close()
	}
	if err == nil || !lost.Load() {
		t.Fatalf("did not simulate a lost response: response=%v err=%v", response, err)
	}
	committed, err := notesStore.GetMonthWorkspace(context.Background(), target.Month)
	if err != nil || committed.Revision != 2 || len(committed.Tables) != 2 {
		t.Fatalf("copy did not commit before response loss: %+v, %v", committed, err)
	}
	originalID := committed.Tables[1].ID
	notesServer.Close()
	if err := notesStore.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted := httptest.NewServer(dualnotes.New(reopened).Routes())
	defer restarted.Close()
	raw := notesRequest(t, restarted.URL, "POST", copyPath, body, http.StatusOK)
	var replay store.WorkspaceCopyResult
	if err := json.Unmarshal(raw, &replay); err != nil {
		t.Fatal(err)
	}
	if replay.OperationID != "lost-response" || !reflect.DeepEqual(replay.Target, committed) || !reflect.DeepEqual(replay.CopiedTableIDs, []string{originalID}) {
		t.Fatalf("retry copied again or changed identities: %+v", replay)
	}
	unchanged, err := reopened.GetMonthWorkspace(context.Background(), source.Month)
	if err != nil || !reflect.DeepEqual(unchanged, source) {
		t.Fatalf("source was changed: %+v, %v", unchanged, err)
	}
	if !reflect.DeepEqual(replay.Target.Tables[0], target.Tables[0]) {
		t.Fatal("existing destination table was changed")
	}
	copied := replay.Target.Tables[1]
	if copied.ID == "source" || copied.Rows[0].ID == "row" || copied.Header != source.Tables[0].Header || copied.Rows[0].Content != source.Tables[0].Rows[0].Content {
		t.Fatalf("incorrect independent copy: %+v", copied)
	}
}
