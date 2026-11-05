package dualnotes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"panel/store"
)

func dualNotesHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.OpenDualNotes(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st).Routes()
}

func dualNotesRequest(t *testing.T, h http.Handler, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, status, w.Body)
	}
	return w
}

func TestDualNotesHTTPRoundTripAndConflict(t *testing.T) {
	h := dualNotesHandler(t)
	w := dualNotesRequest(t, h, "GET", "/api/dual-notes?end=2026-10-05&days=10", "", http.StatusOK)
	var page struct {
		Days []store.DualNoteDay `json:"days"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 10 || page.Days[0].Date != "2026-09-26" || page.Days[9].Date != "2026-10-05" {
		t.Fatalf("wrong calendar range: %+v", page)
	}
	for _, day := range page.Days {
		if day.Revision != 0 || day.Rows == nil || len(day.Rows) != 0 {
			t.Fatalf("empty day JSON contract: %+v", day)
		}
	}
	rows := []store.DualNoteRow{
		{ID: "row-b", Left: "## 推理\n\n\n\n- 一\n- 二\n\n```go\nfmt.Println(\"a | b\")\n```\n", Right: "> AI 回答\n\n**加粗**\n"},
		{ID: "row-a", Left: "", Right: "最后一行。"},
	}
	body, err := json.Marshal(map[string]any{"revision": 0, "rows": rows})
	if err != nil {
		t.Fatal(err)
	}
	w = dualNotesRequest(t, h, "PUT", "/api/dual-notes/2026-10-05", string(body), http.StatusOK)
	var saved store.DualNoteDay
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Date != "2026-10-05" || saved.Revision != 1 || !reflect.DeepEqual(saved.Rows, rows) {
		t.Fatalf("Markdown round trip failed: %+v", saved)
	}
	dualNotesRequest(t, h, "PUT", "/api/dual-notes/2026-10-05", `{"revision":0,"rows":[]}`, http.StatusConflict)
	w = dualNotesRequest(t, h, "GET", "/api/dual-notes?end=2026-10-05&days=1", "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Days) != 1 || !reflect.DeepEqual(page.Days[0], saved) {
		t.Fatalf("conflict lost data: %+v, err=%v", page, err)
	}
	w = dualNotesRequest(t, h, "PUT", "/api/dual-notes/2026-10-05", `{"revision":1,"rows":[]}`, http.StatusOK)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"revision":2`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"rows":[]`)) {
		t.Fatalf("empty save contract: %s", w.Body)
	}
}

func TestDualNotesHTTPFutureDayMatchesOverviewAndFocus(t *testing.T) {
	h := dualNotesHandler(t)
	const date = "2027-01-01"
	rows := []store.DualNoteRow{{ID: "tomorrow", Left: "## 明天\n\n跨年安排", Right: "保留提前记录的内容。"}}
	body, err := json.Marshal(map[string]any{"revision": 0, "rows": rows})
	if err != nil {
		t.Fatal(err)
	}
	w := dualNotesRequest(t, h, "PUT", "/api/dual-notes/"+date, string(body), http.StatusOK)
	var saved store.DualNoteDay
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Date != date || saved.Revision != 1 || !reflect.DeepEqual(saved.Rows, rows) {
		t.Fatalf("future day save failed: %+v", saved)
	}
	var page struct {
		Days []store.DualNoteDay `json:"days"`
	}
	w = dualNotesRequest(t, h, "GET", "/api/dual-notes?end="+date+"&days=11", "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 11 || page.Days[0].Date != "2026-12-22" || page.Days[9].Date != "2026-12-31" || !reflect.DeepEqual(page.Days[10], saved) {
		t.Fatalf("overview did not preserve its ten days and the future day: %+v", page)
	}
	w = dualNotesRequest(t, h, "GET", "/api/dual-notes?end="+date+"&days=1", "", http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Days) != 1 || !reflect.DeepEqual(page.Days[0], saved) {
		t.Fatalf("focus did not return the saved future day: %+v, err=%v", page, err)
	}
}

func TestDualNotesHTTPBadInputDoesNotOverwrite(t *testing.T) {
	h := dualNotesHandler(t)
	path := "/api/dual-notes/2026-10-05"
	dualNotesRequest(t, h, "PUT", path, `{"revision":0,"rows":[{"id":"keep","left":"草稿","right":""}]}`, http.StatusOK)
	before := dualNotesRequest(t, h, "GET", "/api/dual-notes?end=2026-10-05&days=1", "", http.StatusOK).Body.String()
	for _, body := range []string{
		``, `null`, `[]`, `{}`, `{"rows":[]}`, `{"revision":null,"rows":[]}`,
		`{"revision":1}`, `{"revision":1,"rows":null}`, `{"revision":-1,"rows":[]}`,
		`{"revision":1.5,"rows":[]}`, `{"revision":"1","rows":[]}`,
		`{"revision":1,"rows":[]} {}`, `{"revision":1,"rows":[],"unexpected":true}`,
		`{"revision":1,"rows":[null]}`, `{"revision":1,"rows":[{}]}`,
		`{"revision":1,"rows":[{"id":"","left":"","right":""}]}`,
		`{"revision":1,"rows":[{"id":"a","left":null,"right":""}]}`,
		`{"revision":1,"rows":[{"id":"a","left":"","right":false}]}`,
		`{"revision":1,"rows":[{"id":"a","left":"","right":""},{"id":"a","left":"","right":""}]}`,
	} {
		t.Run(fmt.Sprintf("body_%q", body), func(t *testing.T) {
			dualNotesRequest(t, h, "PUT", path, body, http.StatusBadRequest)
		})
	}
	for _, query := range []string{
		"", "?end=2026-10-05", "?days=1", "?end=2026-02-30&days=1",
		"?end=2026-2-03&days=1", "?end=2026-10-05&days=2", "?end=2026-10-05&days=01",
		"?end=2026-10-05&days=12",
		"?end=2026-10-05&days=1&days=10", "?end=2026-10-05&end=2026-10-04&days=1",
	} {
		dualNotesRequest(t, h, "GET", "/api/dual-notes"+query, "", http.StatusBadRequest)
	}
	dualNotesRequest(t, h, "PUT", "/api/dual-notes/2026-02-30", `{"revision":0,"rows":[]}`, http.StatusBadRequest)
	large := `{"revision":1,"rows":[{"id":"big","left":"` + strings.Repeat("x", store.MaxDualNotesBytes) + `","right":""}]}`
	dualNotesRequest(t, h, "PUT", path, large, http.StatusRequestEntityTooLarge)
	after := dualNotesRequest(t, h, "GET", "/api/dual-notes?end=2026-10-05&days=1", "", http.StatusOK).Body.String()
	if after != before {
		t.Fatalf("bad request changed data: before=%s after=%s", before, after)
	}
}
