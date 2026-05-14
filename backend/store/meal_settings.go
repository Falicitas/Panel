package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// MealSetting 单 meal 的全局设置 (跨日保留)。
// PausedUntil > now → 限时暂停; Enabled = false → 永久禁止 (跟 supplement.enabled 同语义)。
type MealSetting struct {
	Meal        string `json:"meal"`         // "breakfast" | "lunch" | "dinner"
	PausedUntil *int64 `json:"paused_until"` // nil = 未暂停
	Enabled     bool   `json:"enabled"`      // false = 永久禁止 (用户场景: 长期不吃某一餐)
}

// IsActive 在 now (unix 秒) 看, 这个 meal 是否仍受通知跟踪。
// disabled 或 paused_until > now → 不发通知, 跟 Supplement.IsActive 同语义。
func (m MealSetting) IsActive(now int64) bool {
	if !m.Enabled {
		return false
	}
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

// GetMealSettingsAll 返回三餐当前 (paused_until, enabled) 设置 map。
// meal_settings 表初始化已 seed 三行, 总会返回 breakfast/lunch/dinner 三 key。
func (s *Store) GetMealSettingsAll() (map[string]MealSetting, error) {
	rows, err := s.db.Query(`SELECT meal, paused_until, enabled FROM meal_settings`)
	if err != nil {
		return nil, fmt.Errorf("query meal_settings: %w", err)
	}
	defer rows.Close()
	out := map[string]MealSetting{
		"breakfast": {Meal: "breakfast", Enabled: true},
		"lunch":     {Meal: "lunch", Enabled: true},
		"dinner":    {Meal: "dinner", Enabled: true},
	}
	for rows.Next() {
		var meal string
		var paused sql.NullInt64
		var enabledI int
		if err := rows.Scan(&meal, &paused, &enabledI); err != nil {
			return nil, fmt.Errorf("scan meal_settings: %w", err)
		}
		ms := MealSetting{Meal: meal, Enabled: enabledI != 0}
		if paused.Valid {
			v := paused.Int64
			ms.PausedUntil = &v
		}
		out[meal] = ms
	}
	return out, rows.Err()
}

// GetMealPausedAll 兼容封装: 仅返回 paused_until map (供 GetDailyState 用)。
// 等价于 GetMealSettingsAll 取 PausedUntil 字段, 但保留作为 API 减少调用方改动。
func (s *Store) GetMealPausedAll() (map[string]*int64, error) {
	all, err := s.GetMealSettingsAll()
	if err != nil {
		return nil, err
	}
	return map[string]*int64{
		"breakfast": all["breakfast"].PausedUntil,
		"lunch":     all["lunch"].PausedUntil,
		"dinner":    all["dinner"].PausedUntil,
	}, nil
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
// 注意: 不动 enabled 字段, ON CONFLICT 只覆盖 paused_until。
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

// SetMealEnabled 永久禁止/启用某 meal。false = 不发该餐通知 (跟 supplement.enabled 同义)。
// 不动 paused_until — 暂停 / 禁止两轴独立 (跟 supplement 行为对齐)。
func (s *Store) SetMealEnabled(meal string, enabled bool) error {
	if !IsValidMeal(meal) {
		return errors.New("invalid meal")
	}
	v := 1
	if !enabled {
		v = 0
	}
	_, err := s.db.Exec(`
		INSERT INTO meal_settings (meal, enabled) VALUES (?, ?)
		ON CONFLICT(meal) DO UPDATE SET enabled = excluded.enabled
	`, meal, v)
	if err != nil {
		return fmt.Errorf("set meal enabled: %w", err)
	}
	return nil
}
