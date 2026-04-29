package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS recurring_todos (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			content    TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			kind       TEXT NOT NULL DEFAULT 'free'
		);
		CREATE TABLE IF NOT EXISTS micro_progress (
			date             TEXT PRIMARY KEY,
			morning_start_at INTEGER,
			morning_end_at   INTEGER
		);
		CREATE TABLE IF NOT EXISTS bills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			monthly_amount INTEGER NOT NULL,
			total_months INTEGER NOT NULL,
			started_month TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS fixed_expenses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			monthly_amount INTEGER NOT NULL,
			started_month TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS free_expenses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL,
			amount INTEGER NOT NULL,
			note TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS incomes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL,
			amount INTEGER NOT NULL,
			note TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS finance_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			initial_balance INTEGER NOT NULL DEFAULT 0,
			free_cap_monthly INTEGER NOT NULL DEFAULT 100000,
			assumed_n_months INTEGER NOT NULL DEFAULT 12,
			assumed_annual_rate REAL NOT NULL DEFAULT 0.12
		);
		INSERT OR IGNORE INTO finance_settings (id, initial_balance) VALUES (1, 0);
		CREATE TABLE IF NOT EXISTS supplements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			windows_json TEXT NOT NULL DEFAULT '[]',
			notes TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS daily_state (
			date TEXT PRIMARY KEY,
			mode TEXT NOT NULL,
			wake_time TEXT,
			mode_set_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS state_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			focus_start TEXT NOT NULL DEFAULT '17:00',
			sleep_start TEXT NOT NULL DEFAULT '22:30'
		);
		INSERT OR IGNORE INTO state_settings (id) VALUES (1);
		CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			source_ref TEXT,
			title TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT 'info',
			created_at INTEGER NOT NULL,
			dismissed_at INTEGER
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_source_ref ON notifications(source_ref) WHERE source_ref IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_notifications_dismissed ON notifications(dismissed_at);
		CREATE TABLE IF NOT EXISTS records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			module TEXT NOT NULL,
			date TEXT NOT NULL,
			key TEXT NOT NULL,
			value_num REAL,
			value_text TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_records_module_date_key ON records(module, date, key);
		CREATE INDEX IF NOT EXISTS idx_records_module_date ON records(module, date);
		CREATE TABLE IF NOT EXISTS bash_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cmd TEXT NOT NULL,
			stdout TEXT NOT NULL DEFAULT '',
			stderr TEXT NOT NULL DEFAULT '',
			exit_code INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_bash_history_created ON bash_history(created_at DESC);
	`)
	if err != nil {
		return err
	}
	// 老数据库补列(SQLite 没有 ADD COLUMN IF NOT EXISTS — duplicate column 错误吞掉即可)
	if _, err := s.db.Exec(`ALTER TABLE recurring_todos ADD COLUMN kind TEXT NOT NULL DEFAULT 'free'`); err != nil {
		if !isDuplicateColumnErr(err) {
			return fmt.Errorf("add recurring_todos.kind: %w", err)
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// modernc.org/sqlite: "duplicate column name: kind (1)"
	return strings.Contains(msg, "duplicate column")
}

const anchorKey = "anchor"

// GetAnchor 返回当前锚定 (nil 表示无锚定)。
func (s *Store) GetAnchor() (*string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, anchorKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	return &v, nil
}

// SetAnchor name=nil 清空锚定。
func (s *Store) SetAnchor(name *string) error {
	v := ""
	if name != nil {
		v = *name
	}
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, anchorKey, v)
	return err
}
