package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func newWorkspaceStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "panel.db")
	st, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, path
}

func TestWorkspaceUntouchedMonthsAndIsolation(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	ctx := context.Background()
	for _, month := range []string{"0001-01", "2026-01", "2026-10", "9999-12"} {
		got, err := st.GetMonthWorkspace(ctx, month)
		if err != nil || got.Month != month || got.Revision != 0 || got.Tables == nil || len(got.Tables) != 0 {
			t.Fatalf("untouched month %s: %+v, %v", month, got, err)
		}
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM short_term_workspace_months`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("read created rows: %d, %v", count, err)
	}
	first, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, []WorkspaceTable{{ID: "same", Header: "十月", Rows: []WorkspaceRow{}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.SaveMonthWorkspace(ctx, "2026-11", 0, []WorkspaceTable{{ID: "same", Header: "十一月", Rows: []WorkspaceRow{}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []MonthWorkspace{first, second} {
		got, err := st.GetMonthWorkspace(ctx, want.Month)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("month leaked: %+v, want %+v, err=%v", got, want, err)
		}
	}
}

func TestWorkspaceRoundTripOrderAndVersionedEmpty(t *testing.T) {
	st, path := newWorkspaceStore(t)
	ctx := context.Background()
	tables := []WorkspaceTable{
		{ID: "z-first", Header: "可编辑标题 <>&", Rows: []WorkspaceRow{{ID: "same", Content: "## 笔记\n\n```go\nfmt.Println(\"a | b\")\n```\n"}, {ID: "a-second", Content: "\t \r\n"}}},
		{ID: "a-second", Header: "", Rows: []WorkspaceRow{{ID: "same", Content: ""}}},
		{ID: "empty", Header: "空表", Rows: []WorkspaceRow{}},
	}
	want, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, tables)
	if err != nil || want.Revision != 1 {
		t.Fatalf("save: %+v, %v", want, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetMonthWorkspace(ctx, want.Month)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reopen changed content/order: %+v, %v", got, err)
	}
	backup := filepath.Join(t.TempDir(), "workspace.db")
	if err := st.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	copy, err := OpenDualNotes(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	copied, err := copy.GetMonthWorkspace(ctx, want.Month)
	if err != nil || !reflect.DeepEqual(copied, want) {
		t.Fatalf("backup lost workspace: %+v, %v", copied, err)
	}
	tables[0].Header = "修改后的标题"
	tables[0].Rows = append(tables[0].Rows, WorkspaceRow{ID: "added", Content: "新行"})
	updated, err := st.SaveMonthWorkspace(ctx, want.Month, 1, tables)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	empty, err := st.SaveMonthWorkspace(ctx, want.Month, 2, []WorkspaceTable{})
	if err != nil || empty.Revision != 3 || empty.Tables == nil || len(empty.Tables) != 0 {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
	for _, stale := range []int64{0, 1, 2} {
		if _, err := st.SaveMonthWorkspace(ctx, want.Month, stale, tables); !errors.Is(err, ErrWorkspaceConflict) {
			t.Fatalf("stale save %d accepted: %v", stale, err)
		}
	}
	got, err = st.GetMonthWorkspace(ctx, want.Month)
	if err != nil || !reflect.DeepEqual(got, empty) {
		t.Fatalf("stale content resurrected: %+v, %v", got, err)
	}
}

func TestWorkspaceConcurrentSaveHasOneWinner(t *testing.T) {
	st, path := newWorkspaceStore(t)
	other, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	for revision := int64(0); revision < 2; revision++ {
		const writers = 8
		start := make(chan struct{})
		type result struct {
			workspace MonthWorkspace
			err       error
		}
		results := make(chan result, writers)
		for i := 0; i < writers; i++ {
			go func(i int) {
				<-start
				target := st
				if i%2 == 1 {
					target = other
				}
				workspace, err := target.SaveMonthWorkspace(ctx, "2026-10", revision, []WorkspaceTable{{ID: fmt.Sprintf("table-%d", i), Header: fmt.Sprintf("header-%d", i), Rows: []WorkspaceRow{{ID: "row", Content: fmt.Sprintf("draft-%d", i)}}}})
				results <- result{workspace, err}
			}(i)
		}
		close(start)
		successes, conflicts := 0, 0
		var winner MonthWorkspace
		for i := 0; i < writers; i++ {
			r := <-results
			switch {
			case r.err == nil:
				successes++
				winner = r.workspace
			case errors.Is(r.err, ErrWorkspaceConflict):
				conflicts++
			default:
				t.Fatalf("concurrent write: %v", r.err)
			}
		}
		if successes != 1 || conflicts != writers-1 {
			t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
		}
		got, err := st.GetMonthWorkspace(ctx, "2026-10")
		if err != nil || winner.Revision != revision+1 || !reflect.DeepEqual(got, winner) {
			t.Fatalf("winner overwritten: %+v, want %+v, err=%v", got, winner, err)
		}
	}
}

func TestWorkspaceRejectsInvalidInputWithoutChangingSavedMonth(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	ctx := context.Background()
	valid := []WorkspaceTable{{ID: "keep", Header: "保存的标题", Rows: []WorkspaceRow{{ID: "row", Content: "保留内容"}}}}
	want, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, month := range []string{"", "2026-1", "2026-00", "2026-13", "0000-01", "10000-01", "2026-10-01", " 2026-10", "2026-10 "} {
		if _, err := st.GetMonthWorkspace(ctx, month); !errors.Is(err, ErrWorkspaceInvalid) {
			t.Errorf("invalid month read %q: %v", month, err)
		}
		if _, err := st.SaveMonthWorkspace(ctx, month, 0, valid); !errors.Is(err, ErrWorkspaceInvalid) {
			t.Errorf("invalid month save %q: %v", month, err)
		}
	}
	for _, revision := range []int64{-1, math.MaxInt64} {
		if _, err := st.SaveMonthWorkspace(ctx, "2026-10", revision, valid); !errors.Is(err, ErrWorkspaceInvalid) {
			t.Errorf("invalid revision %d: %v", revision, err)
		}
	}
	if _, err := st.SaveMonthWorkspace(ctx, "2026-12", 12, valid); !errors.Is(err, ErrWorkspaceConflict) {
		t.Errorf("nonexistent revision: %v", err)
	}
	tooManyRows := make([]WorkspaceRow, MaxWorkspaceRows/2+1)
	for i := range tooManyRows {
		tooManyRows[i] = WorkspaceRow{ID: fmt.Sprintf("row-%d", i)}
	}
	cases := [][]WorkspaceTable{
		nil, make([]WorkspaceTable, MaxWorkspaceTables+1),
		{{ID: "", Rows: []WorkspaceRow{}}}, {{ID: "  ", Rows: []WorkspaceRow{}}},
		{{ID: strings.Repeat("a", 129), Rows: []WorkspaceRow{}}},
		{{ID: "same", Rows: []WorkspaceRow{}}, {ID: "same", Rows: []WorkspaceRow{}}},
		{{ID: "no-rows"}},
		{{ID: "table", Rows: []WorkspaceRow{{ID: ""}}}},
		{{ID: "table", Rows: []WorkspaceRow{{ID: "\t"}}}},
		{{ID: "table", Rows: []WorkspaceRow{{ID: strings.Repeat("a", 129)}}}},
		{{ID: "table", Rows: []WorkspaceRow{{ID: "same"}, {ID: "same"}}}},
		{{ID: "one", Rows: tooManyRows}, {ID: "two", Rows: tooManyRows}},
		{{ID: "large", Header: strings.Repeat("x", MaxWorkspaceBytes), Rows: []WorkspaceRow{}}},
		{{ID: "large", Rows: []WorkspaceRow{{ID: "row", Content: strings.Repeat("x", MaxWorkspaceBytes)}}}},
	}
	for i, tables := range cases {
		if _, err := st.SaveMonthWorkspace(ctx, "2026-10", 1, tables); !errors.Is(err, ErrWorkspaceInvalid) {
			t.Errorf("invalid tables %d: %v", i, err)
		}
	}
	got, err := st.GetMonthWorkspace(ctx, "2026-10")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("bad input changed month: %+v, %v", got, err)
	}
}

func TestWorkspaceAllowsTableAndRowLimits(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	tables := make([]WorkspaceTable, MaxWorkspaceTables)
	for i := range tables {
		tables[i] = WorkspaceTable{ID: fmt.Sprintf("table-%d", i), Rows: []WorkspaceRow{}}
		for j := 0; j < MaxWorkspaceRows/MaxWorkspaceTables; j++ {
			tables[i].Rows = append(tables[i].Rows, WorkspaceRow{ID: fmt.Sprintf("row-%d", j)})
		}
	}
	if _, err := st.SaveMonthWorkspace(context.Background(), "2026-10", 0, tables); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMigrationPreservesDailyNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacyRows := []DualNoteRow{{ID: "old", Left: "既有双栏", Right: "保持原样"}}
	raw, err := json.Marshal(legacyRows)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE dual_notes_days (date TEXT PRIMARY KEY, revision INTEGER NOT NULL CHECK (revision > 0), rows_json TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dual_notes_days (date, revision, rows_json) VALUES (?, ?, ?)`, "2026-10-26", 7, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	notes, err := st.ListDualNoteDays(ctx, "2026-10-26", 1)
	if err != nil || len(notes) != 1 || notes[0].Revision != 7 || !reflect.DeepEqual(notes[0].Rows, legacyRows) {
		t.Fatalf("migration changed legacy notes: %+v, %v", notes, err)
	}
	want, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, []WorkspaceTable{{ID: "new", Header: "新工作区", Rows: []WorkspaceRow{}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	// The main backend can subsequently migrate/open the same database.
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetMonthWorkspace(ctx, "2026-10")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("main migration changed workspace: %+v, %v", got, err)
	}
	notes, err = st.ListDualNoteDays(ctx, "2026-10-26", 1)
	if err != nil || notes[0].Revision != 7 || !reflect.DeepEqual(notes[0].Rows, legacyRows) {
		t.Fatalf("main migration changed daily notes: %+v, %v", notes, err)
	}
}
