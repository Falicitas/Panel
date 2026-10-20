package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panel/store"
)

func startNotesService(t *testing.T, dataDir string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, dataDir, listener) }()
	stopped := false
	stop := func() {
		t.Helper()
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("dual notes shutdown: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("dual notes did not stop")
		}
	}
	t.Cleanup(stop)
	return "http://" + listener.Addr().String(), stop
}

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

func TestDualNotesServiceLifecycleAndBackup(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "new-data")
	notesURL, stopNotes := startNotesService(t, dataDir)
	notesRequest(t, notesURL, "GET", "/api/dual-notes?end=2026-10-21&days=1", "", http.StatusOK)
	notesRequest(t, notesURL, "PUT", "/api/dual-notes/2026-10-21", `{"revision":0,"rows":[{"id":"new","left":"独立服务保存","right":"重启后保留"}]}`, http.StatusOK)
	backupPath := filepath.Join(dataDir, "dual-notes-backups", store.DailyBackupName(time.Now()))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(backupPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("standalone service did not create its daily backup")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopNotes()
	notesURL, _ = startNotesService(t, dataDir)
	raw := notesRequest(t, notesURL, "GET", "/api/dual-notes?end=2026-10-21&days=1", "", http.StatusOK)
	var page struct {
		Days []store.DualNoteDay `json:"days"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 1 || page.Days[0].Revision != 1 || len(page.Days[0].Rows) != 1 || page.Days[0].Rows[0].Left != "独立服务保存" {
		t.Fatalf("standalone restart lost saved notes: %s", raw)
	}
}
