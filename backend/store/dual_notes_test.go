package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func newDualNotesStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestDualNotesCalendarRangeDoesNotCreateRecords(t *testing.T) {
	s, _ := newDualNotesStore(t)
	ctx := context.Background()
	days, err := s.ListDualNoteDays(ctx, "2024-03-03", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2024-02-23", "2024-02-24", "2024-02-25", "2024-02-26", "2024-02-27", "2024-02-28", "2024-02-29", "2024-03-01", "2024-03-02", "2024-03-03"}
	if len(days) != len(want) {
		t.Fatalf("got %d days, want %d", len(days), len(want))
	}
	for i, day := range days {
		if day.Date != want[i] || day.Revision != 0 || day.Rows == nil || len(day.Rows) != 0 {
			t.Fatalf("day %d: %+v", i, day)
		}
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM dual_notes_days`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reading empty days wrote records: count=%d err=%v", count, err)
	}
	days, err = s.ListDualNoteDays(ctx, "2026-10-05", 1)
	if err != nil || len(days) != 1 || days[0].Date != "2026-10-05" {
		t.Fatalf("explicit calendar date was changed: %+v, %v", days, err)
	}
}

func TestDualNotesMarkdownPersistsWithOrderAndRevisions(t *testing.T) {
	s, path := newDualNotesStore(t)
	ctx := context.Background()
	anchor := "existing user setting"
	if err := s.SetAnchor(&anchor); err != nil {
		t.Fatal(err)
	}
	rows := []DualNoteRow{
		{ID: "z-last-alphabetically", Left: "第一段。\n\n\n\n- 列表\n\n```go\nfunc main() {\n\tfmt.Println(\"<tag> & | 中文\")\n}\n```\n", Right: "## 标题\n\n> 引用\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n"},
		{ID: "a-first-alphabetically", Left: "\r\n\t保留空白  \n", Right: ""},
	}
	want := DualNoteDay{Date: "2026-10-05", Revision: 1, Rows: rows}
	got, err := s.SaveDualNoteDay(ctx, want.Date, 0, rows)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("save: got %+v, err=%v", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	days, err := reopened.ListDualNoteDays(ctx, want.Date, 1)
	if err != nil || len(days) != 1 || !reflect.DeepEqual(days[0], want) {
		t.Fatalf("reopen: got %+v, err=%v", days, err)
	}
	if gotAnchor, err := reopened.GetAnchor(); err != nil || gotAnchor == nil || *gotAnchor != anchor {
		t.Fatalf("migration changed existing data: %v, %v", gotAnchor, err)
	}
	if _, err := reopened.SaveDualNoteDay(ctx, want.Date, 0, []DualNoteRow{}); !errors.Is(err, ErrDualNotesConflict) {
		t.Fatalf("stale initial save: %v", err)
	}
	empty, err := reopened.SaveDualNoteDay(ctx, want.Date, 1, []DualNoteRow{})
	if err != nil || empty.Revision != 2 || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("empty save: %+v, %v", empty, err)
	}
	if _, err := reopened.SaveDualNoteDay(ctx, want.Date, 1, rows); !errors.Is(err, ErrDualNotesConflict) {
		t.Fatalf("stale save resurrected deleted rows: %v", err)
	}
	days, err = reopened.ListDualNoteDays(ctx, want.Date, 1)
	if err != nil || !reflect.DeepEqual(days[0], empty) {
		t.Fatalf("conflict overwrote content: %+v, %v", days, err)
	}
}

func TestDualNotesConcurrentSaveHasOneWinner(t *testing.T) {
	s, path := newDualNotesStore(t)
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	for revision := int64(0); revision <= 1; revision++ {
		const writers = 12
		start := make(chan struct{})
		results := make(chan struct {
			day DualNoteDay
			err error
		}, writers)
		var wg sync.WaitGroup
		for i := 0; i < writers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				target := s
				if i%2 == 1 {
					target = other
				}
				day, err := target.SaveDualNoteDay(ctx, "2026-10-05", revision,
					[]DualNoteRow{{ID: fmt.Sprintf("writer-%d", i), Left: fmt.Sprintf("draft %d", i)}})
				results <- struct {
					day DualNoteDay
					err error
				}{day, err}
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		var winner DualNoteDay
		successes, conflicts := 0, 0
		for result := range results {
			switch {
			case result.err == nil:
				successes++
				winner = result.day
			case errors.Is(result.err, ErrDualNotesConflict):
				conflicts++
			default:
				t.Fatalf("concurrent write: %v", result.err)
			}
		}
		if successes != 1 || conflicts != writers-1 {
			t.Fatalf("revision %d: %d successes, %d conflicts", revision, successes, conflicts)
		}
		days, err := s.ListDualNoteDays(ctx, "2026-10-05", 1)
		if err != nil || winner.Revision != revision+1 || !reflect.DeepEqual(days[0], winner) {
			t.Fatalf("winner was overwritten: %+v, winner=%+v, err=%v", days, winner, err)
		}
	}
}

func TestDualNotesRejectInvalidInputs(t *testing.T) {
	s, _ := newDualNotesStore(t)
	ctx := context.Background()
	for _, date := range []string{"", "2026-2-03", "2026-02-30", "2023-02-29", "0000-01-01", "2026-10-05T00:00:00Z"} {
		if _, err := s.ListDualNoteDays(ctx, date, 1); !errors.Is(err, ErrDualNotesInvalid) {
			t.Errorf("date %q: %v", date, err)
		}
	}
	for _, count := range []int{-1, 0, 2, 11} {
		if _, err := s.ListDualNoteDays(ctx, "2026-10-05", count); !errors.Is(err, ErrDualNotesInvalid) {
			t.Errorf("days %d: %v", count, err)
		}
	}
	if _, err := s.ListDualNoteDays(ctx, "0001-01-01", 10); !errors.Is(err, ErrDualNotesInvalid) {
		t.Errorf("out-of-range start: %v", err)
	}
	for _, rows := range [][]DualNoteRow{
		nil, {{ID: ""}}, {{ID: "  "}}, {{ID: "same"}, {ID: "same"}},
		{{ID: strings.Repeat("x", 129)}}, make([]DualNoteRow, MaxDualNotesRows+1),
		{{ID: "large", Left: strings.Repeat("x", MaxDualNotesBytes)}},
	} {
		if _, err := s.SaveDualNoteDay(ctx, "2026-10-05", 0, rows); !errors.Is(err, ErrDualNotesInvalid) {
			t.Errorf("invalid rows accepted: %v", err)
		}
	}
	if _, err := s.SaveDualNoteDay(ctx, "2026-10-05", -1, []DualNoteRow{}); !errors.Is(err, ErrDualNotesInvalid) {
		t.Errorf("negative revision: %v", err)
	}
	if _, err := s.SaveDualNoteDay(ctx, "2026-10-05", 42, []DualNoteRow{}); !errors.Is(err, ErrDualNotesConflict) {
		t.Errorf("nonexistent revision: %v", err)
	}
}
