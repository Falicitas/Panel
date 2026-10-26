package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func seedCopyMonths(t *testing.T, st *Store) (MonthWorkspace, MonthWorkspace, WorkspaceCopyRequest) {
	t.Helper()
	ctx := context.Background()
	source, err := st.SaveMonthWorkspace(ctx, "2026-09", 0, []WorkspaceTable{
		{ID: "first", Header: "同名标题", Rows: []WorkspaceRow{{ID: "source-row", Content: "## 原始内容\n\n- 待迁移"}}},
		{ID: "skip", Header: "不迁移", Rows: []WorkspaceRow{{ID: "skip-row", Content: "源月保留"}}},
		{ID: "last", Header: "同名标题", Rows: []WorkspaceRow{{ID: "last-row", Content: ""}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The destination can already use the source's table or row identities.
	target, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, []WorkspaceTable{{ID: "first", Header: "目标月原有表", Rows: []WorkspaceRow{{ID: "source-row", Content: "已有内容"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return source, target, WorkspaceCopyRequest{OperationID: "copy-one", TargetMonth: target.Month, SourceRevision: source.Revision, TargetRevision: target.Revision, TableIDs: []string{"last", "first"}}
}

func assertCopyUnchanged(t *testing.T, st *Store, want MonthWorkspace) {
	t.Helper()
	got, err := st.GetMonthWorkspace(context.Background(), want.Month)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("month %s changed unexpectedly: got revision=%d, tables=%d, want revision=%d, tables=%d, err=%v", want.Month, got.Revision, len(got.Tables), want.Revision, len(want.Tables), err)
	}
}

func assertCopyCount(t *testing.T, st *Store, want int) {
	t.Helper()
	var got int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM short_term_workspace_copies`).Scan(&got); err != nil || got != want {
		t.Fatalf("copy records=%d, want=%d, err=%v", got, want, err)
	}
}

func TestWorkspaceCopyAppendsInSourceOrderWithFreshIdentities(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	source, target, request := seedCopyMonths(t, st)
	result, err := st.CopyWorkspaceTables(context.Background(), source.Month, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != request.OperationID || result.Target.Month != target.Month || result.Target.Revision != 2 || len(result.Target.Tables) != 3 || len(result.CopiedTableIDs) != 2 {
		t.Fatalf("wrong copy result: %+v", result)
	}
	if !reflect.DeepEqual(result.Target.Tables[0], target.Tables[0]) {
		t.Fatal("existing target table was changed")
	}
	seenTables, seenRows := map[string]bool{}, map[string]bool{}
	for _, workspace := range []MonthWorkspace{source, target} {
		for _, table := range workspace.Tables {
			seenTables[table.ID] = true
			for _, row := range table.Rows {
				seenRows[row.ID] = true
			}
		}
	}
	for i, original := range []WorkspaceTable{source.Tables[0], source.Tables[2]} {
		copied := result.Target.Tables[i+1]
		if copied.ID != result.CopiedTableIDs[i] || seenTables[copied.ID] || copied.Header != original.Header || len(copied.Rows) != len(original.Rows) {
			t.Fatalf("wrong ordered copy %d: %+v", i, copied)
		}
		seenTables[copied.ID] = true
		for j, row := range copied.Rows {
			if seenRows[row.ID] || row.Content != original.Rows[j].Content {
				t.Fatalf("row was not independently copied: %+v", row)
			}
			seenRows[row.ID] = true
		}
	}
	assertCopyUnchanged(t, st, source)
	assertCopyUnchanged(t, st, result.Target)
	assertCopyCount(t, st, 1)
}

func TestWorkspaceCopyReplayUsesCurrentTargetAfterEditsAndRestart(t *testing.T) {
	st, path := newWorkspaceStore(t)
	source, _, request := seedCopyMonths(t, st)
	ctx := context.Background()
	first, err := st.CopyWorkspaceTables(ctx, source.Month, request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		replay, err := st.CopyWorkspaceTables(ctx, source.Month, request)
		if err != nil || !reflect.DeepEqual(replay, first) {
			t.Fatalf("retry changed result: %+v, %v", replay, err)
		}
	}
	// Later source edits, including deleting all selected tables, must not
	// invalidate an already committed operation's retry.
	if _, err := st.SaveMonthWorkspace(ctx, source.Month, source.Revision, []WorkspaceTable{}); err != nil {
		t.Fatal(err)
	}
	first.Target.Tables[0].Header = "迁入之后修改的目标标题"
	first.Target.Tables[1].Rows[0].Content = "迁入之后修改的内容"
	current, err := st.SaveMonthWorkspace(ctx, first.Target.Month, first.Target.Revision, first.Target.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	replay, err := st.CopyWorkspaceTables(ctx, source.Month, request)
	if err != nil {
		t.Fatal(err)
	}
	if replay.OperationID != first.OperationID || !reflect.DeepEqual(replay.CopiedTableIDs, first.CopiedTableIDs) || !reflect.DeepEqual(replay.Target, current) {
		t.Fatalf("replay returned stale state or copied again: %+v", replay)
	}
	assertCopyUnchanged(t, st, current)
	assertCopyCount(t, st, 1)
}

func TestWorkspaceCopyRejectsReusedOperationForDifferentPayload(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	source, _, request := seedCopyMonths(t, st)
	result, err := st.CopyWorkspaceTables(context.Background(), source.Month, request)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, source string
		change       func(*WorkspaceCopyRequest)
	}{
		{"source-month", "2026-08", func(*WorkspaceCopyRequest) {}},
		{"target-month", source.Month, func(r *WorkspaceCopyRequest) { r.TargetMonth = "2026-11" }},
		{"source-revision", source.Month, func(r *WorkspaceCopyRequest) { r.SourceRevision++ }},
		{"target-revision", source.Month, func(r *WorkspaceCopyRequest) { r.TargetRevision++ }},
		{"selection", source.Month, func(r *WorkspaceCopyRequest) { r.TableIDs = []string{"first"} }},
		{"selection-order", source.Month, func(r *WorkspaceCopyRequest) { r.TableIDs = []string{"first", "last"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modified := request
			tc.change(&modified)
			if _, err := st.CopyWorkspaceTables(context.Background(), tc.source, modified); !errors.Is(err, ErrWorkspaceInvalid) {
				t.Fatalf("reused operation accepted: %v", err)
			}
		})
	}
	assertCopyUnchanged(t, st, source)
	assertCopyUnchanged(t, st, result.Target)
	assertCopyCount(t, st, 1)
}

func TestWorkspaceCopyRejectsInvalidRequestsWithoutPartialWrites(t *testing.T) {
	st, _ := newWorkspaceStore(t)
	source, target, request := seedCopyMonths(t, st)
	cases := []struct {
		name   string
		change func(*WorkspaceCopyRequest)
		want   error
	}{
		{"empty-operation", func(r *WorkspaceCopyRequest) { r.OperationID = "" }, ErrWorkspaceInvalid},
		{"blank-operation", func(r *WorkspaceCopyRequest) { r.OperationID = "  " }, ErrWorkspaceInvalid},
		{"long-operation", func(r *WorkspaceCopyRequest) { r.OperationID = strings.Repeat("a", 129) }, ErrWorkspaceInvalid},
		{"same-month", func(r *WorkspaceCopyRequest) { r.TargetMonth = source.Month }, ErrWorkspaceInvalid},
		{"invalid-month", func(r *WorkspaceCopyRequest) { r.TargetMonth = "2026-1" }, ErrWorkspaceInvalid},
		{"negative-source-revision", func(r *WorkspaceCopyRequest) { r.SourceRevision = -1 }, ErrWorkspaceInvalid},
		{"negative-target-revision", func(r *WorkspaceCopyRequest) { r.TargetRevision = -1 }, ErrWorkspaceInvalid},
		{"overflow-target-revision", func(r *WorkspaceCopyRequest) { r.TargetRevision = math.MaxInt64 }, ErrWorkspaceInvalid},
		{"empty-selection", func(r *WorkspaceCopyRequest) { r.TableIDs = []string{} }, ErrWorkspaceInvalid},
		{"nil-selection", func(r *WorkspaceCopyRequest) { r.TableIDs = nil }, ErrWorkspaceInvalid},
		{"blank-id", func(r *WorkspaceCopyRequest) { r.TableIDs = []string{" "} }, ErrWorkspaceInvalid},
		{"long-id", func(r *WorkspaceCopyRequest) { r.TableIDs = []string{strings.Repeat("a", 129)} }, ErrWorkspaceInvalid},
		{"duplicate-id", func(r *WorkspaceCopyRequest) { r.TableIDs = []string{"first", "first"} }, ErrWorkspaceInvalid},
		{"missing-id", func(r *WorkspaceCopyRequest) { r.TableIDs = []string{"first", "missing"} }, ErrWorkspaceInvalid},
		{"too-many-ids", func(r *WorkspaceCopyRequest) { r.TableIDs = make([]string, MaxWorkspaceTables+1) }, ErrWorkspaceInvalid},
		{"source-cas", func(r *WorkspaceCopyRequest) { r.SourceRevision = 0 }, ErrWorkspaceConflict},
		{"target-cas", func(r *WorkspaceCopyRequest) { r.TargetRevision = 0 }, ErrWorkspaceConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modified := request
			tc.change(&modified)
			if _, err := st.CopyWorkspaceTables(context.Background(), source.Month, modified); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			assertCopyUnchanged(t, st, source)
			assertCopyUnchanged(t, st, target)
			assertCopyCount(t, st, 0)
		})
	}
	if _, err := st.CopyWorkspaceTables(context.Background(), "2026-9", request); !errors.Is(err, ErrWorkspaceInvalid) {
		t.Fatalf("bad source month: %v", err)
	}
}

func TestWorkspaceCopyCapacityFailurePreservesBothMonths(t *testing.T) {
	for _, limit := range []string{"tables", "rows", "bytes"} {
		t.Run(limit, func(t *testing.T) {
			st, _ := newWorkspaceStore(t)
			sourceTables := []WorkspaceTable{{ID: "copy", Rows: []WorkspaceRow{{ID: "source-row", Content: "copy"}}}}
			targetTables := []WorkspaceTable{{ID: "keep", Rows: []WorkspaceRow{}}}
			switch limit {
			case "tables":
				targetTables = make([]WorkspaceTable, MaxWorkspaceTables)
				for i := range targetTables {
					targetTables[i] = WorkspaceTable{ID: fmt.Sprintf("table-%d", i), Rows: []WorkspaceRow{}}
				}
			case "rows":
				for i := 0; i < MaxWorkspaceRows; i++ {
					targetTables[0].Rows = append(targetTables[0].Rows, WorkspaceRow{ID: fmt.Sprintf("row-%d", i)})
				}
			case "bytes":
				sourceTables[0].Rows[0].Content = strings.Repeat("x", MaxWorkspaceBytes/2)
				targetTables[0].Header = strings.Repeat("y", MaxWorkspaceBytes/2)
			}
			ctx := context.Background()
			source, err := st.SaveMonthWorkspace(ctx, "2026-09", 0, sourceTables)
			if err != nil {
				t.Fatal(err)
			}
			target, err := st.SaveMonthWorkspace(ctx, "2026-10", 0, targetTables)
			if err != nil {
				t.Fatal(err)
			}
			request := WorkspaceCopyRequest{OperationID: "over-limit", TargetMonth: target.Month, SourceRevision: 1, TargetRevision: 1, TableIDs: []string{"copy"}}
			if _, err := st.CopyWorkspaceTables(ctx, source.Month, request); !errors.Is(err, ErrWorkspaceInvalid) {
				t.Fatalf("capacity was not enforced: %v", err)
			}
			assertCopyUnchanged(t, st, source)
			assertCopyUnchanged(t, st, target)
			assertCopyCount(t, st, 0)
		})
	}
}

func TestWorkspaceCopyRollsBackTargetWhenOperationRecordFails(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			st, _ := newWorkspaceStore(t)
			source, target, request := seedCopyMonths(t, st)
			if !existing {
				request.TargetMonth = "2026-11"
				request.TargetRevision = 0
				target = MonthWorkspace{Month: request.TargetMonth, Tables: []WorkspaceTable{}}
			}
			if _, err := st.db.Exec(`CREATE TRIGGER fail_copy_record BEFORE INSERT ON short_term_workspace_copies BEGIN SELECT RAISE(ABORT, 'forced record failure'); END`); err != nil {
				t.Fatal(err)
			}
			if _, err := st.CopyWorkspaceTables(context.Background(), source.Month, request); err == nil {
				t.Fatal("copy succeeded despite failing operation record")
			}
			assertCopyUnchanged(t, st, source)
			assertCopyUnchanged(t, st, target)
			assertCopyCount(t, st, 0)
			if !existing {
				var count int
				if err := st.db.QueryRow(`SELECT COUNT(*) FROM short_term_workspace_months WHERE month = ?`, request.TargetMonth).Scan(&count); err != nil || count != 0 {
					t.Fatalf("failed copy created target: %d, %v", count, err)
				}
			}
			if _, err := st.db.Exec(`DROP TRIGGER fail_copy_record`); err != nil {
				t.Fatal(err)
			}
			if _, err := st.CopyWorkspaceTables(context.Background(), source.Month, request); err != nil {
				t.Fatalf("rollback left transaction or operation stuck: %v", err)
			}
		})
	}
}

func TestWorkspaceCopyConcurrentRetriesAppendOnce(t *testing.T) {
	st, path := newWorkspaceStore(t)
	source, _, request := seedCopyMonths(t, st)
	other, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	const count = 8
	start := make(chan struct{})
	type result struct {
		value WorkspaceCopyResult
		err   error
	}
	results := make(chan result, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			<-start
			target := st
			if i%2 != 0 {
				target = other
			}
			value, err := target.CopyWorkspaceTables(context.Background(), source.Month, request)
			results <- result{value, err}
		}(i)
	}
	close(start)
	var first WorkspaceCopyResult
	for i := 0; i < count; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if i == 0 {
			first = r.value
		} else if !reflect.DeepEqual(first, r.value) {
			t.Fatal("concurrent retry duplicated tables")
		}
	}
	if first.Target.Revision != 2 || len(first.Target.Tables) != 3 {
		t.Fatalf("more than one append: %+v", first)
	}
	assertCopyUnchanged(t, st, source)
	assertCopyCount(t, st, 1)
}

func TestWorkspaceCopyConcurrentDistinctOperationsHaveOneWinner(t *testing.T) {
	st, path := newWorkspaceStore(t)
	source, _, request := seedCopyMonths(t, st)
	other, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, target := range []*Store{st, other} {
		go func(i int, target *Store) {
			<-start
			input := request
			input.OperationID = fmt.Sprintf("operation-%d", i)
			_, err := target.CopyWorkspaceTables(context.Background(), source.Month, input)
			results <- err
		}(i, target)
	}
	close(start)
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, ErrWorkspaceConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	assertCopyUnchanged(t, st, source)
	assertCopyCount(t, st, 1)
}
