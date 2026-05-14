package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// MealPausedUntil 单个 meal 的暂停截止 (unix 秒);nil 表示未暂停。
// 跟 Supplement.PausedUntil 同语义: > now 时不弹通知 / UI 显示暂停状态。
// 全局设置 (不按日期划分), 跨日保留 — 用户场景: 出差 / 短期不想被催早午晚餐通知。
type MealPausedUntil struct {
	Meal        string `json:"meal"`         // "breakfast" | "lunch" | "dinner"
	PausedUntil *int64 `json:"paused_until"` // nil = 未暂停
}

// IsActive 在 now (unix 秒) 看, 这个 meal 是否仍受通知跟踪 (未暂停)。
// paused_until > now 即处于暂停期, 不发提醒。
func (m MealPausedUntil) IsActive(now int64) bool {
	if m.PausedUntil != nil && *m.PausedUntil > now {
		return false
	}
	return true
}

// IsValidMeal breakfast / lunch / dinner 之一。其它字符串拒掉, 防写脏 meal 名。
func IsValidMeal(meal string) bool {
	switch meal {
	case "breakfast", "lunch", "dinner":
		return true
	}
	return false
}

// GetMealPausedAll 返回三餐当前 paused_until map (key = meal name)。
// meal_settings 表初始化已 seed 三行, 总会返回 breakfast/lunch/dinner 三 key。
// paused_until 为 NULL → map value 是 nil。
func (s *Store) GetMealPausedAll() (map[string]*int64, error) {
	rows, err := s.db.Query(`SELECT meal, paused_until FROM meal_settings`)
	if err != nil {
		return nil, fmt.Errorf("query meal_settings: %w", err)
	}
	defer rows.Close()
	out := map[string]*int64{
		"breakfast": nil,
		"lunch":     nil,
		"dinner":    nil,
	}
	for rows.Next() {
		var meal string
		var paused sql.NullInt64
		if err := rows.Scan(&meal, &paused); err != nil {
			return nil, fmt.Errorf("scan meal_settings: %w", err)
		}
		if paused.Valid {
			v := paused.Int64
			out[meal] = &v
		} else {
			out[meal] = nil
		}
	}
	return out, rows.Err()
}

// GetMealPausedUntil 返回单 meal 的 paused_until;不存在 (理论上不该发生, seed 已 insert)
// 当作 nil 处理。invalid meal 名返错误。
func (s *Store) GetMealPausedUntil(meal string) (*int64, error) {
	if !IsValidMeal(meal) {
		return nil, errors.New("invalid meal")
	}
	var paused sql.NullInt64
	err := s.db.QueryRow(
		`SELECT paused_until FROM meal_settings WHERE meal=?`, meal,
	).Scan(&paused)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query meal paused: %w", err)
	}
	if !paused.Valid {
		return nil, nil
	}
	v := paused.Int64
	return &v, nil
}

// SetMealPausedUntil 设置/清除某 meal 的暂停截止 (unix 秒)。nil = 取消暂停。
// invalid meal 名返错误。UPSERT (ON CONFLICT) 保证 seed 缺失时也能写入。
func (s *Store) SetMealPausedUntil(meal string, until *int64) error {
	if !IsValidMeal(meal) {
		return errors.New("invalid meal")
	}
	var v any
	if until != nil {
		v = *until
	}
	_, err := s.db.Exec(`
		INSERT INTO meal_settings (meal, paused_until) VALUES (?, ?)
		ON CONFLICT(meal) DO UPDATE SET paused_until = excluded.paused_until
	`, meal, v)
	if err != nil {
		return fmt.Errorf("set meal paused: %w", err)
	}
	return nil
}
