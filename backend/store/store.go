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
