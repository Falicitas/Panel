package store

import (
	"database/sql"
	"errors"
)

// MicroProgress 当日 micro 晨间任务进度时间戳。
// MorningStartAt: 第一次有 morning recurring 项被勾选的时刻 (Unix 秒)
// MorningEndAt:   第一次所有 morning recurring 项都勾完的时刻
// 一旦写入不会被清掉 (避免来回勾选时反复触发)。
type MicroProgress struct {
	Date           string `json:"date"`
	MorningStartAt *int64 `json:"morning_start_at"`
	MorningEndAt   *int64 `json:"morning_end_at"`
}

// GetMicroProgress 取当日记录;无记录返回零值 (Date 为空)。
func (s *Store) GetMicroProgress(date string) (MicroProgress, error) {
	row := s.db.QueryRow(
		`SELECT date, morning_start_at, morning_end_at FROM micro_progress WHERE date = ?`, date,
	)
	var p MicroProgress
	err := row.Scan(&p.Date, &p.MorningStartAt, &p.MorningEndAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MicroProgress{Date: date}, nil
	}
	if err != nil {
		return MicroProgress{}, err
	}
	return p, nil
}

// UpsertMicroProgress 更新当日记录;调用方负责处理"已写入不再覆盖"逻辑,
// 这层只做无脑 upsert。
func (s *Store) UpsertMicroProgress(p MicroProgress) error {
	_, err := s.db.Exec(`
		INSERT INTO micro_progress (date, morning_start_at, morning_end_at)
		VALUES (?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			morning_start_at = excluded.morning_start_at,
			morning_end_at = excluded.morning_end_at
	`, p.Date, p.MorningStartAt, p.MorningEndAt)
	return err
}
