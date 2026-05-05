package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// 食物 / 保健品
type Supplement struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`            // "food" | "supplement"
	Windows        []Window `json:"windows"`         // 可空
	Notes          string   `json:"notes"`
	Dose           string   `json:"dose"`            // "1 颗" / "5 滴" 等,纯字符串
	Enabled        bool     `json:"enabled"`         // false 时通知不弹,统计不计 expected
	PausedUntil    *int64   `json:"paused_until"`    // unix 秒;> now 时不弹通知
	CompletionMode string   `json:"completion_mode"` // "all" (默认, 全部 window 都勾才算 item 完成) | "any" (任一勾上就算)
}

// CompletionModeAll / CompletionModeAny 是 Supplement.CompletionMode 的合法值。
const (
	CompletionModeAll = "all"
	CompletionModeAny = "any"
)

// NormalizedCompletionMode 把空 / 未知值规范成 "all" (默认)。
// 老数据写库时 completion_mode 字段不存在 → DB DEFAULT 'all'; 但 JSON 反序列化
// 时若 client 传空字符串需要在 handler 里调一次。
func (sp Supplement) NormalizedCompletionMode() string {
	if sp.CompletionMode == CompletionModeAny {
		return CompletionModeAny
	}
	return CompletionModeAll
}

// Window 是单个 (start, end) 段。
// 时间表达式: "HH:MM" 或 "wake" / "wake+Nh" / "wake-Nh" / "wake+Nm"
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// IsActive 在 now (Unix 秒) 看,这个 supplement 是否仍受跟踪 (enabled & 未暂停)。
// disabled 或 paused_until > now → 不算 active。
func (sp Supplement) IsActive(now int64) bool {
	if !sp.Enabled {
		return false
	}
	if sp.PausedUntil != nil && *sp.PausedUntil > now {
		return false
	}
	return true
}

func (s *Store) ListSupplements() ([]Supplement, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, windows_json, notes, dose, enabled, paused_until, completion_mode
		   FROM supplements ORDER BY sort_order, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Supplement{}
	for rows.Next() {
		var sp Supplement
		var raw string
		var enabledI int
		var paused sql.NullInt64
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Type, &raw, &sp.Notes, &sp.Dose, &enabledI, &paused, &sp.CompletionMode); err != nil {
			return nil, err
		}
		sp.Windows = []Window{}
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &sp.Windows)
		}
		sp.Enabled = enabledI != 0
		if paused.Valid {
			v := paused.Int64
			sp.PausedUntil = &v
		}
		sp.CompletionMode = sp.NormalizedCompletionMode()
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) GetSupplement(id int64) (Supplement, error) {
	var sp Supplement
	var raw string
	var enabledI int
	var paused sql.NullInt64
	err := s.db.QueryRow(
		`SELECT id, name, type, windows_json, notes, dose, enabled, paused_until, completion_mode
		   FROM supplements WHERE id=?`, id,
	).Scan(&sp.ID, &sp.Name, &sp.Type, &raw, &sp.Notes, &sp.Dose, &enabledI, &paused, &sp.CompletionMode)
	if errors.Is(err, sql.ErrNoRows) {
		return Supplement{}, sql.ErrNoRows
	}
	if err != nil {
		return Supplement{}, err
	}
	sp.Windows = []Window{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &sp.Windows)
	}
	sp.Enabled = enabledI != 0
	if paused.Valid {
		v := paused.Int64
		sp.PausedUntil = &v
	}
	sp.CompletionMode = sp.NormalizedCompletionMode()
	return sp, nil
}

func (s *Store) CreateSupplement(sp Supplement) (Supplement, error) {
	if sp.Windows == nil {
		sp.Windows = []Window{}
	}
	sp.CompletionMode = sp.NormalizedCompletionMode()
	raw, _ := json.Marshal(sp.Windows)
	enabledI := 1
	if !sp.Enabled {
		enabledI = 0
	}
	var paused any
	if sp.PausedUntil != nil {
		paused = *sp.PausedUntil
	}
	// 新 item push 到末尾: sort_order = max(sort_order)+1, 没行时从 1 起。
	var nextOrder int64
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM supplements`).Scan(&nextOrder)
	res, err := s.db.Exec(
		`INSERT INTO supplements (name, type, windows_json, notes, dose, enabled, paused_until, sort_order, completion_mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sp.Name, sp.Type, string(raw), sp.Notes, sp.Dose, enabledI, paused, nextOrder, sp.CompletionMode,
	)
	if err != nil {
		return Supplement{}, err
	}
	id, _ := res.LastInsertId()
	sp.ID = id
	return sp, nil
}

// ReorderSupplements 按传入 ID 顺序重写 sort_order = 索引 (1-based, 留 0 给未列出的)。
// 未列出的 item 保留原 sort_order — 不参与重排, 它们继续排在后面。
// 用于 mac UI 上下移动 item 后的全列表保存。
func (s *Store) ReorderSupplements(orderedIDs []int64) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	for i, id := range orderedIDs {
		// 1-based 防 sort_order = 0 被 backfill 误覆盖
		if _, err := tx.Exec(`UPDATE supplements SET sort_order=? WHERE id=?`, i+1, id); err != nil {
			return fmt.Errorf("reorder id=%d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateSupplement(sp Supplement) error {
	if sp.Windows == nil {
		sp.Windows = []Window{}
	}
	sp.CompletionMode = sp.NormalizedCompletionMode()
	raw, _ := json.Marshal(sp.Windows)
	enabledI := 1
	if !sp.Enabled {
		enabledI = 0
	}
	var paused any
	if sp.PausedUntil != nil {
		paused = *sp.PausedUntil
	}
	_, err := s.db.Exec(
		`UPDATE supplements SET name=?, type=?, windows_json=?, notes=?, dose=?, enabled=?, paused_until=?, completion_mode=?
		   WHERE id=?`,
		sp.Name, sp.Type, string(raw), sp.Notes, sp.Dose, enabledI, paused, sp.CompletionMode, sp.ID,
	)
	return err
}

// SetSupplementEnabled 仅切 enabled。其他字段保持。
func (s *Store) SetSupplementEnabled(id int64, enabled bool) error {
	v := 1
	if !enabled {
		v = 0
	}
	_, err := s.db.Exec(`UPDATE supplements SET enabled=? WHERE id=?`, v, id)
	return err
}

// SetSupplementPausedUntil 设置/清除暂停截止 (unix 秒)。nil = 取消暂停。
func (s *Store) SetSupplementPausedUntil(id int64, until *int64) error {
	if until == nil {
		_, err := s.db.Exec(`UPDATE supplements SET paused_until=NULL WHERE id=?`, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE supplements SET paused_until=? WHERE id=?`, *until, id)
	return err
}

// DeleteNotificationsBySupplementID 硬 DELETE 所有 source='supplement' 且 source_ref
// 前缀 'supplement:{id}:' 的通知 (含 dismissed)。返回受影响行数。
// 用于: (1) 删 item 时 cascade (DeleteSupplement) (2) disable item 时清待处理通知。
func (s *Store) DeleteNotificationsBySupplementID(id int64) (int64, error) {
	prefix := fmt.Sprintf("supplement:%d:", id)
	res, err := s.db.Exec(
		`DELETE FROM notifications WHERE source='supplement' AND source_ref LIKE ?`,
		prefix+"%",
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DismissSupplementItemNotificationsForDate 把 supplement id 在 date 当日的所有未 dismissed
// window 通知一并 dismiss。用于 completion_mode="any" 勾选任一 window 后视为整 item 当日已完成,
// 后续其它 window 的通知不再有意义。返回受影响行数。
func (s *Store) DismissSupplementItemNotificationsForDate(id int64, date string, atUnix int64) (int64, error) {
	pattern := fmt.Sprintf("supplement:%d:window:%%:date:%s", id, date)
	res, err := s.db.Exec(
		`UPDATE notifications SET dismissed_at=?
		  WHERE source='supplement' AND dismissed_at IS NULL AND source_ref LIKE ?`,
		atUnix, pattern,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UndismissSupplementItemNotificationsForDate 把 supplement id 在 date 当日所有 window 通知
// undismiss (dismissed_at = NULL), 跟 DismissSupplementItemNotificationsForDate 对称。
// 用于 completion_mode="any" 取消勾选时让 item 整组通知重新 active, mac UrgentNotifier
// 下一 tick 会基于 dismissed_at IS NULL 自动恢复 banner / cancelPending 的反向。
// 已过期 window 通知由调用方紧随其后调 GenerateNotifications -> expireUnfinishedSupplementNotifications
// 再次 dismiss 干净, 不会留过期通知在通知栏。
func (s *Store) UndismissSupplementItemNotificationsForDate(id int64, date string) error {
	pattern := fmt.Sprintf("supplement:%d:window:%%:date:%s", id, date)
	_, err := s.db.Exec(
		`UPDATE notifications SET dismissed_at=NULL
		  WHERE source='supplement' AND source_ref LIKE ?`,
		pattern,
	)
	return err
}

// DeleteSupplement 硬删 supplement + cascade 抹除所有指向它的通知 (硬 DELETE 不是 dismiss)。
// 用户主动删 item → 通知与 (item, window) 二元组双向绑定, item 不存在则通知不再有意义。
// 用 source_ref LIKE 'supplement:{id}:%' 锁定该 item 全部 window 的通知 (无关 dismissed_at)。
func (s *Store) DeleteSupplement(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	prefix := fmt.Sprintf("supplement:%d:", id)
	if _, err := tx.Exec(
		`DELETE FROM notifications WHERE source='supplement' AND source_ref LIKE ?`,
		prefix+"%",
	); err != nil {
		return fmt.Errorf("cascade delete notifications: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM supplements WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete supplement: %w", err)
	}
	return tx.Commit()
}

// ---- daily_state ----

type DailyState struct {
	Date         string `json:"date"`         // YYYY-MM-DD
	Mode         string `json:"mode"`         // "normal" | "focus" | "sleep" | "outside"
	WakeTime     string `json:"wake_time"`    // "HH:MM" or ""
	ModeSetAt    int64  `json:"mode_set_at"`
	BreakfastAt  string `json:"breakfast_at"` // "HH:MM" or "" — 用户主动触发的早餐时刻
	LunchAt      string `json:"lunch_at"`
	DinnerAt     string `json:"dinner_at"`
}

func (s *Store) GetDailyState(date string) (DailyState, error) {
	var ds DailyState
	var wake, bf, lu, di sql.NullString
	err := s.db.QueryRow(
		`SELECT date, mode, wake_time, mode_set_at, breakfast_at, lunch_at, dinner_at
		   FROM daily_state WHERE date=?`, date,
	).Scan(&ds.Date, &ds.Mode, &wake, &ds.ModeSetAt, &bf, &lu, &di)
	if errors.Is(err, sql.ErrNoRows) {
		return DailyState{Date: date, Mode: "normal", WakeTime: "", ModeSetAt: 0}, nil
	}
	if err != nil {
		return DailyState{}, err
	}
	if wake.Valid {
		ds.WakeTime = wake.String
	}
	if bf.Valid {
		ds.BreakfastAt = bf.String
	}
	if lu.Valid {
		ds.LunchAt = lu.String
	}
	if di.Valid {
		ds.DinnerAt = di.String
	}
	return ds, nil
}

// SetDailyState upsert mode + wake_time。不动 meal_at 字段 (餐由 SetMealTime 单独写)。
func (s *Store) SetDailyState(ds DailyState) error {
	var wake any
	if ds.WakeTime == "" {
		wake = nil
	} else {
		wake = ds.WakeTime
	}
	_, err := s.db.Exec(`
		INSERT INTO daily_state (date, mode, wake_time, mode_set_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET mode=excluded.mode, wake_time=excluded.wake_time, mode_set_at=excluded.mode_set_at
	`, ds.Date, ds.Mode, wake, ds.ModeSetAt)
	return err
}

// SetMealTime 单列写 meal_at;hhmm="" 表示清空。
// meal ∈ {"breakfast","lunch","dinner"}。
func (s *Store) SetMealTime(date, meal, hhmm string) error {
	col, ok := mealColumn(meal)
	if !ok {
		return errors.New("invalid meal")
	}
	var v any
	if hhmm == "" {
		v = nil
	} else {
		v = hhmm
	}
	// 没有 daily_state 行时先用默认 mode/normal,wake/null 占位再 update meal 列
	_, err := s.db.Exec(`
		INSERT INTO daily_state (date, mode, wake_time, mode_set_at, `+col+`)
		VALUES (?, 'normal', NULL, 0, ?)
		ON CONFLICT(date) DO UPDATE SET `+col+` = excluded.`+col+`
	`, date, v)
	return err
}

func mealColumn(meal string) (string, bool) {
	switch meal {
	case "breakfast":
		return "breakfast_at", true
	case "lunch":
		return "lunch_at", true
	case "dinner":
		return "dinner_at", true
	}
	return "", false
}

// ---- state_settings ----

type StateSettings struct {
	FocusStart string `json:"focus_start"` // HH:MM
	SleepStart string `json:"sleep_start"`
}

func (s *Store) GetStateSettings() (StateSettings, error) {
	var ss StateSettings
	err := s.db.QueryRow(
		`SELECT focus_start, sleep_start FROM state_settings WHERE id=1`,
	).Scan(&ss.FocusStart, &ss.SleepStart)
	if errors.Is(err, sql.ErrNoRows) {
		return StateSettings{FocusStart: "17:00", SleepStart: "22:30"}, nil
	}
	return ss, err
}

func (s *Store) SetStateSettings(ss StateSettings) error {
	_, err := s.db.Exec(
		`UPDATE state_settings SET focus_start=?, sleep_start=? WHERE id=1`,
		ss.FocusStart, ss.SleepStart,
	)
	return err
}

// ---- notifications ----

type Notification struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`       // "supplement" / "finance" / "records" / "manual"
	SourceRef   string `json:"source_ref"`   // 幂等键,空表示无幂等约束
	Title       string `json:"title"`
	Body        string `json:"body"`
	Level       string `json:"level"`        // "info" | "warn" | "alert"
	CreatedAt   int64  `json:"created_at"`
	DismissedAt *int64 `json:"dismissed_at,omitempty"`
}

// ListNotifications 列出通知,默认只返回未 dismissed 的;all=true 返回全部 (倒序)。
func (s *Store) ListNotifications(all bool) ([]Notification, error) {
	q := `SELECT id, source, COALESCE(source_ref, ''), title, body, level, created_at, dismissed_at FROM notifications`
	if !all {
		q += ` WHERE dismissed_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT 200`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var dismissed sql.NullInt64
		if err := rows.Scan(&n.ID, &n.Source, &n.SourceRef, &n.Title, &n.Body, &n.Level, &n.CreatedAt, &dismissed); err != nil {
			return nil, err
		}
		if dismissed.Valid {
			v := dismissed.Int64
			n.DismissedAt = &v
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateNotificationIdempotent 写入通知,source_ref 冲突时静默跳过。
// 返回 true 表示新写入,false 表示已存在被跳过。
func (s *Store) CreateNotificationIdempotent(n Notification) (bool, error) {
	var ref any
	if n.SourceRef == "" {
		ref = nil
	} else {
		ref = n.SourceRef
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO notifications (source, source_ref, title, body, level, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, n.Source, ref, n.Title, n.Body, n.Level, n.CreatedAt)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// UpdateNotificationBodyByRef 更新 source_ref 对应通知的 body, 不动 dismissed_at。
// 用于通知 body 中的时段字符串随 anchor 变化时同步 — 例如用户先 set dinner=20:30
// 让 body 变成 "时段 21:00-21:30", 再 set dinner=21:00 时, body 应更新到新时刻。
// INSERT IGNORE 不会覆写已存在行的 body, 必须显式 UPDATE。
// dismissed_at 故意不动: 用户主动 dismiss 的通知不该因为 anchor 变化又自动复活。
func (s *Store) UpdateNotificationBodyByRef(ref, body string) error {
	if ref == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE notifications SET body = ? WHERE source_ref = ?`, body, ref)
	return err
}

func (s *Store) DismissNotification(id int64, at int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET dismissed_at=? WHERE id=? AND dismissed_at IS NULL`, at, id)
	return err
}

// DismissNotificationByRef 按 source_ref 把对应未 dismissed 的通知标 dismissed。
// 用于"用户在 UI 完成某 window → 自动清掉对应通知"。
// ref 为空时静默跳过 (避免误清所有 source_ref IS NULL 的通知)。
func (s *Store) DismissNotificationByRef(ref string, atUnix int64) error {
	if ref == "" {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE notifications SET dismissed_at=?
		  WHERE source_ref=? AND dismissed_at IS NULL`,
		atUnix, ref,
	)
	return err
}

// UndismissNotificationByRef 把对应 source_ref 的通知 dismissed_at 复位为 NULL,
// 让它重新出现在通知列表。用于"用户取消勾选 → 通知回归"。
// ref 为空时静默跳过。
func (s *Store) UndismissNotificationByRef(ref string) error {
	if ref == "" {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE notifications SET dismissed_at=NULL
		  WHERE source_ref=?`,
		ref,
	)
	return err
}

// DismissStaleSupplementNotifications 把 source='supplement' 且 created_at < beforeUnix
// 且未 dismissed 的通知一并 dismissed。返回受影响行数。
// 用于 GenerateNotifications 入口,避免昨天的食物保健品提醒堆到今天。
func (s *Store) DismissStaleSupplementNotifications(beforeUnix, atUnix int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE notifications
		    SET dismissed_at=?
		  WHERE source='supplement' AND dismissed_at IS NULL AND created_at < ?`,
		atUnix, beforeUnix,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DismissStaleMealNotifications 跨日清理: 把 source='meal' 且 created_at < beforeUnix
// 且未 dismissed 的通知一并 dismissed, 避免昨天的早午晚餐提醒堆到今天。
// 用法跟 DismissStaleSupplementNotifications 一致, 在 GenerateMealNotifications 入口调一次。
func (s *Store) DismissStaleMealNotifications(beforeUnix, atUnix int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE notifications
		    SET dismissed_at=?
		  WHERE source='meal' AND dismissed_at IS NULL AND created_at < ?`,
		atUnix, beforeUnix,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
