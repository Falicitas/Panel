package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenDualNotesOnlyCreatesNotesSchema(t *testing.T) {
	st, err := OpenDualNotes(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rows, err := st.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !reflect.DeepEqual(tables, []string{"dual_notes_days", "short_term_workspace_copies", "short_term_workspace_months"}) {
		t.Fatalf("unrelated schema was initialized: %v", tables)
	}
	if _, err := st.SaveDualNoteDay(context.Background(), "2026-10-21", 0, []DualNoteRow{}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDualNotesIgnoresUnrelatedMigrationFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	st, err := OpenDualNotes(path)
	if err != nil {
		t.Fatal(err)
	}
	// This incompatible legacy schema would fail the main store's migration.
	if _, err := st.db.Exec(`CREATE TABLE supplements (legacy_value TEXT)`); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = OpenDualNotes(path)
	if err != nil {
		t.Fatalf("unrelated feature prevented dual notes startup: %v", err)
	}
	defer st.Close()
	if _, err := st.SaveDualNoteDay(context.Background(), "2026-10-21", 0, []DualNoteRow{{ID: "row", Left: "saved"}}); err != nil {
		t.Fatal(err)
	}
}
