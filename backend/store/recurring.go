package store

import "time"

type RecurringTodo struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	SortOrder int    `json:"sort_order"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) ListRecurring() ([]RecurringTodo, error) {
	rows, err := s.db.Query(
		`SELECT id, content, sort_order, created_at FROM recurring_todos ORDER BY sort_order ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecurringTodo
	for rows.Next() {
		var r RecurringTodo
		if err := rows.Scan(&r.ID, &r.Content, &r.SortOrder, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRecurring(content string, sortOrder int) (RecurringTodo, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO recurring_todos (content, sort_order, created_at) VALUES (?, ?, ?)`,
		content, sortOrder, now,
	)
	if err != nil {
		return RecurringTodo{}, err
	}
	id, _ := res.LastInsertId()
	return RecurringTodo{ID: id, Content: content, SortOrder: sortOrder, CreatedAt: now}, nil
}

func (s *Store) UpdateRecurring(id int64, content string, sortOrder int) error {
	_, err := s.db.Exec(
		`UPDATE recurring_todos SET content = ?, sort_order = ? WHERE id = ?`,
		content, sortOrder, id,
	)
	return err
}

func (s *Store) DeleteRecurring(id int64) error {
	_, err := s.db.Exec(`DELETE FROM recurring_todos WHERE id = ?`, id)
	return err
}
