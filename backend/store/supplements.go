package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// 食物 / 保健品
type Supplement struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`    // "food" | "supplement"
	Windows []Window `json:"windows"` // 可空
	Notes   string   `json:"notes"`
}

// Window 是单个 (start, end) 段。
// 时间表达式: "HH:MM" 或 "wake" / "wake+Nh" / "wake-Nh" / "wake+Nm"
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func (s *Store) ListSupplements() ([]Supplement, error) {
	rows, err := s.db.Query(`SELECT id, name, type, windows_json, notes FROM supplements ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Supplement{}
	for rows.Next() {
		var sp Supplement
		var raw string
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Type, &raw, &sp.Notes); err != nil {
			return nil, err
		}
		sp.Windows = []Window{}
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &sp.Windows)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) CreateSupplement(sp Supplement) (Supplement, error) {
	if sp.Windows == nil {
		sp.Windows = []Window{}
	}
	raw, _ := json.Marshal(sp.Windows)
	res, err := s.db.Exec(
		`INSERT INTO supplements (name, type, windows_json, notes) VALUES (?, ?, ?, ?)`,
		sp.Name, sp.Type, string(raw), sp.Notes,
	)
	if err != nil {
		return Supplement{}, err
	}
	id, _ := res.LastInsertId()
	sp.ID = id
	return sp, nil
}

func (s *Store) UpdateSupplement(sp Supplement) error {
	if sp.Windows == nil {
		sp.Windows = []Window{}
	}
	raw, _ := json.Marshal(sp.Windows)
	_, err := s.db.Exec(
		`UPDATE supplements SET name=?, type=?, windows_json=?, notes=? WHERE id=?`,
		sp.Name, sp.Type, string(raw), sp.Notes, sp.ID,
	)
	return err
}

func (s *Store) DeleteSupplement(id int64) error {
	_, err := s.db.Exec(`DELETE FROM supplements WHERE id=?`, id)
	return err
}

// ---- daily_state ----

type DailyState struct {
	Date      string `json:"date"`       // YYYY-MM-DD
	Mode      string `json:"mode"`       // "normal" | "focus" | "sleep" | "outside"
	WakeTime  string `json:"wake_time"`  // "HH:MM" or ""
	ModeSetAt int64  `json:"mode_set_at"`
}

func (s *Store) GetDailyState(date string) (DailyState, error) {
	var ds DailyState
	var wake sql.NullString
	err := s.db.QueryRow(
		`SELECT date, mode, wake_time, mode_set_at FROM daily_state WHERE date=?`, date,
	).Scan(&ds.Date, &ds.Mode, &wake, &ds.ModeSetAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DailyState{Date: date, Mode: "normal", WakeTime: "", ModeSetAt: 0}, nil
	}
	if wake.Valid {
		ds.WakeTime = wake.String
	}
	return ds, err
}

// SetDailyState upsert 当日状态。
func (s *Store) SetDailyState(ds DailyState) error {
	var wake any
	if ds.WakeTime == "" {
		wake = nil
	} else {
		wake = ds.WakeTime
	}
	_, err := s.db.Exec(`
		INSERT INTO daily_state (date, mode, wake_time, mode_set_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET mode=excluded.mode, wake_time=excluded.wake_time, mode_set_at=excluded.mode_set_at
	`, ds.Date, ds.Mode, wake, ds.ModeSetAt)
	return err
}

// ---- state_settings ----

type StateSettings struct {
	FocusStart string `json:"focus_start"` // HH:MM
	SleepStart string `json:"sleep_start"`
}

func (s *Store) GetStateSettings() (StateSettings, error) {
	var ss StateSettings
	err := s.db.QueryRow(
		`SELECT focus_start, sleep_start FROM state_settings WHERE id=1`,
	).Scan(&ss.FocusStart, &ss.SleepStart)
	if errors.Is(err, sql.ErrNoRows) {
		return StateSettings{FocusStart: "17:00", SleepStart: "22:30"}, nil
	}
	return ss, err
}

func (s *Store) SetStateSettings(ss StateSettings) error {
	_, err := s.db.Exec(
		`UPDATE state_settings SET focus_start=?, sleep_start=? WHERE id=1`,
		ss.FocusStart, ss.SleepStart,
	)
	return err
}

// ---- notifications ----

type Notification struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`      // "supplement" / "finance" / "records" / "manual"
	SourceRef   string `json:"source_ref"`  // 幂等键,空表示无幂等约束
	Title       string `json:"title"`
	Body        string `json:"body"`
	Level       string `json:"level"`       // "info" | "warn" | "alert"
	CreatedAt   int64  `json:"created_at"`
	DismissedAt *int64 `json:"dismissed_at,omitempty"`
}

// ListNotifications 列出通知,默认只返回未 dismissed 的;all=true 返回全部 (倒序)。
func (s *Store) ListNotifications(all bool) ([]Notification, error) {
	q := `SELECT id, source, COALESCE(source_ref, ''), title, body, level, created_at, dismissed_at FROM notifications`
	if !all {
		q += ` WHERE dismissed_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT 200`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var dismissed sql.NullInt64
		if err := rows.Scan(&n.ID, &n.Source, &n.SourceRef, &n.Title, &n.Body, &n.Level, &n.CreatedAt, &dismissed); err != nil {
			return nil, err
		}
		if dismissed.Valid {
			v := dismissed.Int64
			n.DismissedAt = &v
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateNotificationIdempotent 写入通知,source_ref 冲突时静默跳过。
// 返回 true 表示新写入,false 表示已存在被跳过。
func (s *Store) CreateNotificationIdempotent(n Notification) (bool, error) {
	var ref any
	if n.SourceRef == "" {
		ref = nil
	} else {
		ref = n.SourceRef
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO notifications (source, source_ref, title, body, level, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, n.Source, ref, n.Title, n.Body, n.Level, n.CreatedAt)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

func (s *Store) DismissNotification(id int64, at int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET dismissed_at=? WHERE id=? AND dismissed_at IS NULL`, at, id)
	return err
}
