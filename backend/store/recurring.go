package store

import (
	"errors"
	"fmt"
	"time"
)

type RecurringTodo struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	SortOrder int    `json:"sort_order"`
	CreatedAt int64  `json:"created_at"`
	Kind      string `json:"kind"` // "morning" | "phone" | "free"
}

const (
	RecurringKindMorning = "morning"
	RecurringKindPhone   = "phone"
	RecurringKindFree    = "free"
)

func ValidRecurringKind(k string) bool {
	return k == RecurringKindMorning || k == RecurringKindPhone || k == RecurringKindFree
}

// ListRecurring 返回所有复发 todo,排序: morning → phone → free, 同 kind 内按 sort_order → id。
func (s *Store) ListRecurring() ([]RecurringTodo, error) {
	rows, err := s.db.Query(
		`SELECT id, content, sort_order, created_at, kind
		 FROM recurring_todos
		 ORDER BY (CASE kind WHEN 'morning' THEN 0 WHEN 'phone' THEN 1 ELSE 2 END),
		          sort_order ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecurringTodo
	for rows.Next() {
		var r RecurringTodo
		if err := rows.Scan(&r.ID, &r.Content, &r.SortOrder, &r.CreatedAt, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRecurringByKind 仅取指定 kind 的项。
func (s *Store) ListRecurringByKind(kind string) ([]RecurringTodo, error) {
	rows, err := s.db.Query(
		`SELECT id, content, sort_order, created_at, kind
		 FROM recurring_todos WHERE kind = ?
		 ORDER BY sort_order ASC, id ASC`,
		kind,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecurringTodo
	for rows.Next() {
		var r RecurringTodo
		if err := rows.Scan(&r.ID, &r.Content, &r.SortOrder, &r.CreatedAt, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// nextSortOrder 取 kind 内最大 sort_order + 1。无项时返回 0。
func (s *Store) nextSortOrder(kind string) (int, error) {
	var v *int
	if err := s.db.QueryRow(
		`SELECT MAX(sort_order) FROM recurring_todos WHERE kind = ?`, kind,
	).Scan(&v); err != nil {
		return 0, err
	}
	if v == nil {
		return 0, nil
	}
	return *v + 1, nil
}

// CreateRecurring 在指定 kind 末尾追加一条 (sort_order 自动 = max+1)。
func (s *Store) CreateRecurring(content, kind string) (RecurringTodo, error) {
	if !ValidRecurringKind(kind) {
		return RecurringTodo{}, errors.New("invalid kind")
	}
	order, err := s.nextSortOrder(kind)
	if err != nil {
		return RecurringTodo{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO recurring_todos (content, sort_order, created_at, kind) VALUES (?, ?, ?, ?)`,
		content, order, now, kind,
	)
	if err != nil {
		return RecurringTodo{}, err
	}
	id, _ := res.LastInsertId()
	return RecurringTodo{ID: id, Content: content, SortOrder: order, CreatedAt: now, Kind: kind}, nil
}

// UpdateRecurring 改 content / kind。kind 变化时自动放到目标段末尾。
func (s *Store) UpdateRecurring(id int64, content, kind string) error {
	if !ValidRecurringKind(kind) {
		return errors.New("invalid kind")
	}
	var oldKind string
	if err := s.db.QueryRow(`SELECT kind FROM recurring_todos WHERE id = ?`, id).Scan(&oldKind); err != nil {
		return fmt.Errorf("load existing recurring: %w", err)
	}
	if oldKind == kind {
		_, err := s.db.Exec(
			`UPDATE recurring_todos SET content = ? WHERE id = ?`, content, id,
		)
		return err
	}
	order, err := s.nextSortOrder(kind)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`UPDATE recurring_todos SET content = ?, kind = ?, sort_order = ? WHERE id = ?`,
		content, kind, order, id,
	)
	return err
}

// ReorderRecurring 按 ids 顺序重置指定 kind 内 sort_order = 0..N-1。
// 仅认定 ids 中属于该 kind 的项,其他 id 安全忽略。
func (s *Store) ReorderRecurring(kind string, ids []int64) error {
	if !ValidRecurringKind(kind) {
		return errors.New("invalid kind")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE recurring_todos SET sort_order = ? WHERE id = ? AND kind = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i, id, kind); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteRecurring(id int64) error {
	_, err := s.db.Exec(`DELETE FROM recurring_todos WHERE id = ?`, id)
	return err
}
