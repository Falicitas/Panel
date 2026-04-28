package store

import (
	"database/sql"
	"errors"
	"time"
)

// Record 通用记录 (一行 = 一个 (module, date, key) 三元组)。
// 例如握力: module=grip_strength, date=2026-04-28, key=left|right, value_num=kg。
type Record struct {
	ID        int64    `json:"id"`
	Module    string   `json:"module"`
	Date      string   `json:"date"` // YYYY-MM-DD
	Key       string   `json:"key"`
	ValueNum  *float64 `json:"value_num"`
	ValueText string   `json:"value_text"`
	CreatedAt int64    `json:"created_at"`
}

// UpsertRecord 按 (module, date, key) upsert,返回更新后的行。
func (s *Store) UpsertRecord(r Record) (Record, error) {
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().Unix()
	}
	var num any
	if r.ValueNum != nil {
		num = *r.ValueNum
	}
	_, err := s.db.Exec(`
		INSERT INTO records (module, date, key, value_num, value_text, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(module, date, key) DO UPDATE SET
			value_num = excluded.value_num,
			value_text = excluded.value_text,
			created_at = excluded.created_at
	`, r.Module, r.Date, r.Key, num, r.ValueText, r.CreatedAt)
	if err != nil {
		return Record{}, err
	}
	return s.GetRecord(r.Module, r.Date, r.Key)
}

func (s *Store) GetRecord(module, date, key string) (Record, error) {
	var r Record
	var num sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT id, module, date, key, value_num, value_text, created_at
		   FROM records WHERE module=? AND date=? AND key=?`,
		module, date, key,
	).Scan(&r.ID, &r.Module, &r.Date, &r.Key, &num, &r.ValueText, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, err
	}
	if num.Valid {
		v := num.Float64
		r.ValueNum = &v
	}
	return r, nil
}

// ListRecords 按 module + date 区间 (含端点) 列出,date 升序、key 升序。
// fromDate / toDate 留空表示不限。
func (s *Store) ListRecords(module, fromDate, toDate string) ([]Record, error) {
	q := `SELECT id, module, date, key, value_num, value_text, created_at
	        FROM records WHERE module=?`
	args := []any{module}
	if fromDate != "" {
		q += ` AND date >= ?`
		args = append(args, fromDate)
	}
	if toDate != "" {
		q += ` AND date <= ?`
		args = append(args, toDate)
	}
	q += ` ORDER BY date ASC, key ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var num sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Module, &r.Date, &r.Key, &num, &r.ValueText, &r.CreatedAt); err != nil {
			return nil, err
		}
		if num.Valid {
			v := num.Float64
			r.ValueNum = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRecordsForDate(module, date string) error {
	_, err := s.db.Exec(`DELETE FROM records WHERE module=? AND date=?`, module, date)
	return err
}
