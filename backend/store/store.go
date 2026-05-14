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
			notes TEXT NOT NULL DEFAULT '',
			dose TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			paused_until INTEGER,
			completion_mode TEXT NOT NULL DEFAULT 'all'
		);
		CREATE TABLE IF NOT EXISTS daily_state (
			date TEXT PRIMARY KEY,
			mode TEXT NOT NULL,
			wake_time TEXT,
			mode_set_at INTEGER NOT NULL,
			breakfast_at TEXT,
			lunch_at TEXT,
			dinner_at TEXT
		);
		CREATE TABLE IF NOT EXISTS state_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			focus_start TEXT NOT NULL DEFAULT '17:00',
			sleep_start TEXT NOT NULL DEFAULT '22:30'
		);
		INSERT OR IGNORE INTO state_settings (id) VALUES (1);
		CREATE TABLE IF NOT EXISTS meal_settings (
			meal         TEXT PRIMARY KEY CHECK (meal IN ('breakfast', 'lunch', 'dinner')),
			paused_until INTEGER
		);
		INSERT OR IGNORE INTO meal_settings (meal) VALUES ('breakfast'), ('lunch'), ('dinner');
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
		CREATE TABLE IF NOT EXISTS english_audios (
			id                     INTEGER PRIMARY KEY AUTOINCREMENT,
			source                 TEXT NOT NULL,
			source_ref             TEXT NOT NULL,
			title                  TEXT NOT NULL,
			description            TEXT NOT NULL DEFAULT '',
			published_at           INTEGER NOT NULL,
			duration_ms            INTEGER NOT NULL DEFAULT 0,
			audio_url              TEXT NOT NULL,
			local_path             TEXT,
			transcript             TEXT NOT NULL DEFAULT '',
			segments_status        TEXT NOT NULL DEFAULT 'pending',
			segments_failed_reason TEXT NOT NULL DEFAULT '',
			fetched_at             INTEGER NOT NULL,
			last_played_at         INTEGER,
			archived_at            INTEGER
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_english_audios_src_ref
		  ON english_audios(source, source_ref);
		CREATE INDEX IF NOT EXISTS idx_english_audios_pub
		  ON english_audios(published_at DESC);
		CREATE TABLE IF NOT EXISTS english_segments (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			audio_id  INTEGER NOT NULL,
			idx       INTEGER NOT NULL,
			start_ms  INTEGER NOT NULL,
			end_ms    INTEGER NOT NULL,
			text      TEXT NOT NULL DEFAULT ''
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_english_segments_audio_idx
		  ON english_segments(audio_id, idx);
		CREATE TABLE IF NOT EXISTS english_settings (
			id                 INTEGER PRIMARY KEY CHECK (id = 1),
			windows_json       TEXT NOT NULL DEFAULT '[]',
			threshold_min      INTEGER NOT NULL DEFAULT 10,
			enabled            INTEGER NOT NULL DEFAULT 1,
			review_daily_limit INTEGER NOT NULL DEFAULT 15
		);
		INSERT OR IGNORE INTO english_settings (id, windows_json)
		  VALUES (1, '[{"start":"07:30","end":"09:00"},{"start":"21:00","end":"22:30"}]');
		CREATE TABLE IF NOT EXISTS english_review_queue (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			audio_id          INTEGER NOT NULL,
			seg_idx           INTEGER NOT NULL,
			stage             INTEGER NOT NULL DEFAULT 0,
			next_review_at    TEXT    NOT NULL,
			first_done_at     INTEGER NOT NULL,
			last_advanced_at  INTEGER NOT NULL,
			postponed_count   INTEGER NOT NULL DEFAULT 0,
			finished_at       INTEGER,
			mastered_at       INTEGER
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_review_queue_audio_seg
		  ON english_review_queue(audio_id, seg_idx);
		CREATE INDEX IF NOT EXISTS idx_review_queue_due
		  ON english_review_queue(next_review_at);
	`)
	if err != nil {
		return err
	}
	// 老数据库补列(SQLite 没有 ADD COLUMN IF NOT EXISTS — duplicate column 错误吞掉即可)
	for _, alter := range []struct {
		desc string
		sql  string
	}{
		{"recurring_todos.kind", `ALTER TABLE recurring_todos ADD COLUMN kind TEXT NOT NULL DEFAULT 'free'`},
		{"supplements.dose", `ALTER TABLE supplements ADD COLUMN dose TEXT NOT NULL DEFAULT ''`},
		{"supplements.enabled", `ALTER TABLE supplements ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`},
		{"supplements.paused_until", `ALTER TABLE supplements ADD COLUMN paused_until INTEGER`},
		{"daily_state.breakfast_at", `ALTER TABLE daily_state ADD COLUMN breakfast_at TEXT`},
		{"daily_state.lunch_at", `ALTER TABLE daily_state ADD COLUMN lunch_at TEXT`},
		{"daily_state.dinner_at", `ALTER TABLE daily_state ADD COLUMN dinner_at TEXT`},
		// supplements 用户自定义排序: 老数据 sort_order = id 保持现顺序;
		// 新建 item sort_order = max(sort_order)+1 push 到末尾。
		{"supplements.sort_order", `ALTER TABLE supplements ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`},
		// 一次性 backfill: 老 supplements 的 sort_order 默认 0, 全为 0 时 ORDER BY 退化到 id, 视觉无变化。
		// 但接下来 reorder 操作会写非 0 值, 此时混 0 行排在最前会乱。先把所有 0 行设成 id 保持初始顺序。
		{"supplements.sort_order_backfill", `UPDATE supplements SET sort_order = id WHERE sort_order = 0`},
		// completion_mode: "all"(默认 -> 所有 window 都勾才算 item 完成) | "any"(任一 window 勾上即算)。
		// 老数据补列默认 'all', 行为不变。
		{"supplements.completion_mode", `ALTER TABLE supplements ADD COLUMN completion_mode TEXT NOT NULL DEFAULT 'all'`},
		// english_audios.topic: 音频主题分类 (free-text, 例: finance/tech/science/current_affairs/general)。
		// 老数据默认空串 = "未分类"。fetcher 写入新数据时会带上 source 默认 topic。
		{"english_audios.topic", `ALTER TABLE english_audios ADD COLUMN topic TEXT NOT NULL DEFAULT ''`},
		// 一次性 backfill: 老 npr / voa audio 没 topic, 按 source 默认值补; 仅 topic='' 行受影响, 幂等。
		{"english_audios.topic_backfill_npr", `UPDATE english_audios SET topic='finance' WHERE source='npr' AND topic=''`},
		{"english_audios.topic_backfill_voa", `UPDATE english_audios SET topic='general' WHERE source='voa' AND topic=''`},
		// english_settings.review_daily_limit: 精听复习队列每日配额上限 (issue-44 引入)。
		// 老库默认 15 条/天; 用户可在 EnglishSettingsSheet 改。配额溢出的复习项自动 postpone 到次日。
		{"english_settings.review_daily_limit", `ALTER TABLE english_settings ADD COLUMN review_daily_limit INTEGER NOT NULL DEFAULT 15`},
	} {
		if _, err := s.db.Exec(alter.sql); err != nil {
			if !isDuplicateColumnErr(err) {
				return fmt.Errorf("migrate %s: %w", alter.desc, err)
			}
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
