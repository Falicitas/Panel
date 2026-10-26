package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// OpenDualNotes opens the existing notes table without running migrations for
// supplements, English practice, or any other Panel feature.
func OpenDualNotes(path string) (*Store, error) {
	return open(path, func(s *Store) error {
		// Keep the timeout on the sole connection used by reads, writes, and
		// backups. Another process may briefly hold the shared database lock.
		s.db.SetMaxOpenConns(1)
		if _, err := s.db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
			return err
		}
		return s.migrateDualNotes()
	})
}

func (s *Store) migrateDualNotes() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS dual_notes_days (
		date      TEXT PRIMARY KEY,
		revision  INTEGER NOT NULL CHECK (revision > 0),
		rows_json TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS short_term_workspace_months (
		month       TEXT PRIMARY KEY,
		revision    INTEGER NOT NULL CHECK (revision > 0),
		tables_json TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS short_term_workspace_copies (
		operation_id          TEXT PRIMARY KEY,
		fingerprint           TEXT NOT NULL,
		target_month          TEXT NOT NULL,
		copied_table_ids_json TEXT NOT NULL
	)`)
	return err
}

const (
	MaxDualNotesBytes = 4 << 20
	MaxDualNotesRows  = 1000
)

var (
	ErrDualNotesInvalid  = errors.New("双栏笔记内容无效")
	ErrDualNotesConflict = errors.New("此日笔记已更新，请保留当前内容并重新载入")
)

type DualNoteRow struct {
	ID    string `json:"id"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

type DualNoteDay struct {
	Date     string        `json:"date"`
	Revision int64         `json:"revision"`
	Rows     []DualNoteRow `json:"rows"`
}

// ParseDualNoteDate uses the client's calendar day, independent of the 06:00
// logical-day boundary used by other Panel modules.
func ParseDualNoteDate(date string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil || len(date) != 10 || t.Year() < 1 || t.Format("2006-01-02") != date {
		return time.Time{}, fmt.Errorf("%w：日期必须为有效的 YYYY-MM-DD", ErrDualNotesInvalid)
	}
	return t, nil
}

// ListDualNoteDays returns every calendar date in ascending order, including
// empty days. Reading an empty day never creates a database record.
func (s *Store) ListDualNoteDays(ctx context.Context, end string, count int) ([]DualNoteDay, error) {
	endDate, err := ParseDualNoteDate(end)
	if err != nil {
		return nil, err
	}
	if count != 1 && count != 10 {
		return nil, fmt.Errorf("%w：days 只能是 1 或 10", ErrDualNotesInvalid)
	}
	start := endDate.AddDate(0, 0, 1-count)
	if start.Year() < 1 {
		return nil, fmt.Errorf("%w：日期范围超出支持范围", ErrDualNotesInvalid)
	}
	days := make([]DualNoteDay, count)
	indices := make(map[string]int, count)
	for i := range days {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		days[i] = DualNoteDay{Date: date, Rows: []DualNoteRow{}}
		indices[date] = i
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, revision, rows_json FROM dual_notes_days
		 WHERE date >= ? AND date <= ? ORDER BY date`, days[0].Date, end)
	if err != nil {
		return nil, fmt.Errorf("read dual notes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var date, raw string
		var revision int64
		if err := rows.Scan(&date, &revision, &raw); err != nil {
			return nil, err
		}
		i := indices[date]
		if err := json.Unmarshal([]byte(raw), &days[i].Rows); err != nil {
			return nil, fmt.Errorf("decode dual notes %s: %w", date, err)
		}
		days[i].Revision = revision
	}
	return days, rows.Err()
}

// SaveDualNoteDay atomically replaces one day's ordered Markdown cells only
// when the caller still holds the latest revision. An empty rows array is a
// versioned save as well, so stale clients cannot resurrect deleted content.
func (s *Store) SaveDualNoteDay(ctx context.Context, date string, revision int64, rows []DualNoteRow) (DualNoteDay, error) {
	if _, err := ParseDualNoteDate(date); err != nil {
		return DualNoteDay{}, err
	}
	if revision < 0 || revision == math.MaxInt64 {
		return DualNoteDay{}, fmt.Errorf("%w：revision 无效", ErrDualNotesInvalid)
	}
	if rows == nil || len(rows) > MaxDualNotesRows {
		return DualNoteDay{}, fmt.Errorf("%w：rows 必须为数组且不超过 %d 行", ErrDualNotesInvalid, MaxDualNotesRows)
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" || len(row.ID) > 128 || seen[row.ID] {
			return DualNoteDay{}, fmt.Errorf("%w：行 ID 不能为空、重复或超过 128 字节", ErrDualNotesInvalid)
		}
		seen[row.ID] = true
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return DualNoteDay{}, err
	}
	if len(raw) > MaxDualNotesBytes {
		return DualNoteDay{}, fmt.Errorf("%w：每日内容超过 4 MiB", ErrDualNotesInvalid)
	}

	// The timeout is configured on the exact connection executing the atomic
	// statement. Concurrent autosaves wait for SQLite's short write lock before
	// checking revision, including when another Store/process opens this file.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return DualNoteDay{}, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return DualNoteDay{}, err
	}
	var nextRevision int64
	if revision == 0 {
		err = conn.QueryRowContext(ctx,
			`INSERT INTO dual_notes_days (date, revision, rows_json) VALUES (?, 1, ?)
			 ON CONFLICT(date) DO NOTHING RETURNING revision`, date, string(raw)).Scan(&nextRevision)
	} else {
		err = conn.QueryRowContext(ctx,
			`UPDATE dual_notes_days SET revision = revision + 1, rows_json = ?
			 WHERE date = ? AND revision = ? RETURNING revision`, string(raw), date, revision).Scan(&nextRevision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return DualNoteDay{}, ErrDualNotesConflict
	}
	if err != nil {
		return DualNoteDay{}, fmt.Errorf("save dual notes: %w", err)
	}
	return DualNoteDay{Date: date, Revision: nextRevision, Rows: rows}, nil
}
