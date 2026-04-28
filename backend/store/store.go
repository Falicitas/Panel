package store

import (
	"database/sql"
	"errors"
	"fmt"

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
			created_at INTEGER NOT NULL
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
	`)
	return err
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
