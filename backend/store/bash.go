package store

// BashHistoryEntry 一条 zsh 命令执行记录。
type BashHistoryEntry struct {
	ID         int64  `json:"id"`
	Cmd        string `json:"cmd"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	CreatedAt  int64  `json:"created_at"`
}

// CreateBashHistory 写一条记录,返回带 ID 的副本。
func (s *Store) CreateBashHistory(e BashHistoryEntry) (BashHistoryEntry, error) {
	res, err := s.db.Exec(
		`INSERT INTO bash_history (cmd, stdout, stderr, exit_code, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.Cmd, e.Stdout, e.Stderr, e.ExitCode, e.DurationMs, e.CreatedAt,
	)
	if err != nil {
		return BashHistoryEntry{}, err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return e, nil
}

// ListBashHistory 倒序拿最近 limit 条 (limit<=0 用默认 100)。
func (s *Store) ListBashHistory(limit int) ([]BashHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, cmd, stdout, stderr, exit_code, duration_ms, created_at
		   FROM bash_history ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BashHistoryEntry{}
	for rows.Next() {
		var e BashHistoryEntry
		if err := rows.Scan(&e.ID, &e.Cmd, &e.Stdout, &e.Stderr, &e.ExitCode, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
