package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
	"panel/supplements"
)

var hhmmRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func validMode(m string) bool {
	switch m {
	case "normal", "focus", "sleep", "outside":
		return true
	}
	return false
}

func (s *Server) handleGetTodayState(w http.ResponseWriter, r *http.Request) {
	today := dateutil.LogicalToday(time.Now())
	if at := r.URL.Query().Get("date"); at != "" {
		today = at
	}
	ds, err := s.store.GetDailyState(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

type stateModePayload struct {
	Mode     string `json:"mode"`
	WakeTime string `json:"wake_time"` // 可选,首次进入 normal/outside 时由前端传"现在时刻"
}

func (s *Server) handlePostTodayState(w http.ResponseWriter, r *http.Request) {
	var p stateModePayload
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !validMode(p.Mode) {
		writeError(w, http.StatusBadRequest, errors.New("mode invalid"))
		return
	}
	if p.WakeTime != "" && !hhmmRegex.MatchString(p.WakeTime) {
		writeError(w, http.StatusBadRequest, errors.New("wake_time must be HH:MM"))
		return
	}
	now := time.Now()
	today := dateutil.LogicalToday(now)
	cur, err := s.store.GetDailyState(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	wake := cur.WakeTime
	// 蒙版选普通/外出时,前端传 wake_time;只有首次记录(原 wake 为空)才记录,避免日间切换覆盖
	if p.WakeTime != "" && wake == "" {
		wake = p.WakeTime
	}
	ds := store.DailyState{
		Date:      today,
		Mode:      p.Mode,
		WakeTime:  wake,
		ModeSetAt: now.Unix(),
	}
	if err := s.store.SetDailyState(ds); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 当日首次进入普通模式 -> 注入 free 复发 todo 到 micro 顶部 (PRD: 蒙版选普通时注入,
	// 选外出不注入)。morning/phone 各有 ritual 蒙版承接, 不再注入 micro。
	wasFirstSet := cur.ModeSetAt == 0
	if wasFirstSet && p.Mode == "normal" {
		if err := s.injectRecurringIntoMicro(); err != nil {
			// 不阻断状态切换,只 log
			writeJSON(w, http.StatusOK, map[string]any{"daily_state": ds, "inject_error": err.Error()})
			return
		}
	}
	// wake_time 一旦写入,引用 wake 的 supplement window 可能立刻 active — 立即扫一次通知
	if n, err := supplements.GenerateNotifications(s.store, time.Now()); err != nil {
		log.Printf("supplement notif gen (post-state): %v", err)
	} else if n > 0 {
		log.Printf("supplement notif gen (post-state): %d new", n)
	}
	writeJSON(w, http.StatusOK, ds)
}

// injectRecurringIntoMicro 把 free kind 的复发 todo 注入当日 micro 顶部。
// morning → 晨间 CommitRitualView 蒙版, phone → 睡前 ritual 蒙版, 两者都有蒙版承接
// 不再注入 micro md (早期 morning 也注入, 改蒙版后成冗余, 去掉)。
func (s *Server) injectRecurringIntoMicro() error {
	recur, err := s.store.ListRecurring()
	if err != nil {
		return err
	}
	// 仅 free 注入 micro。
	filtered := recur[:0]
	for _, r := range recur {
		if r.Kind != store.RecurringKindFree {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return nil
	}
	today, err := s.micro.LoadToday()
	if err != nil {
		return err
	}
	var sb strings.Builder
	for _, r := range filtered {
		sb.WriteString("- [ ] ")
		sb.WriteString(r.Content)
		sb.WriteString("\n")
	}
	if today.RawMD != "" {
		sb.WriteString("\n")
		sb.WriteString(today.RawMD)
	}
	return s.micro.WriteToday(sb.String())
}

func (s *Server) handleGetStateSettings(w http.ResponseWriter, r *http.Request) {
	ss, err := s.store.GetStateSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

// handlePostMeal POST /api/state/meal body {meal, time?}
//   - meal ∈ {"breakfast","lunch","dinner"}
//   - time 缺省 = 当前 HH:MM;不缺省必须为 HH:MM
//   - 验证 HH:MM 落在 meal 区间内 (breakfast 07:00-11:59, lunch 12:00-17:29, dinner 17:30-23:59)
//   - meal="" + time="" 视作清空 (取消触发)
func (s *Server) handlePostMeal(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Meal  string `json:"meal"`
		Time  string `json:"time"`
		Clear bool   `json:"clear"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Meal != "breakfast" && p.Meal != "lunch" && p.Meal != "dinner" {
		writeError(w, http.StatusBadRequest, errors.New("meal must be breakfast | lunch | dinner"))
		return
	}
	today := dateutil.LogicalToday(time.Now())
	if p.Clear {
		if err := s.store.SetMealTime(today, p.Meal, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// 用户清空餐时刻 → meal 通知回归 (跟记录时立即 dismiss 对称); 时段已过的会被
		// expireUnrecordedMealNotifications 再次 dismiss, 仍在范围内的保留 → 提醒重新挂起。
		ref := fmt.Sprintf("meal:%s:date:%s", p.Meal, today)
		_ = s.store.UndismissNotificationByRef(ref)
		_, _ = supplements.GenerateMealNotifications(s.store, time.Now())
		// 同时让 supplement 通知重扫 — 引用了刚被 clear 的 meal 的 window (e.g. "dinner+90m")
		// 现在解析失败、UI 隐藏, 之前发的通知必须级联 dismiss
		// (expireUnfinishedSupplementNotifications 已扩展处理 missing-meal case)。
		_, _ = supplements.GenerateNotifications(s.store, time.Now())
		ds, _ := s.store.GetDailyState(today)
		writeJSON(w, http.StatusOK, ds)
		return
	}
	if p.Time == "" {
		p.Time = time.Now().Format("15:04")
	}
	if !hhmmRegex.MatchString(p.Time) {
		writeError(w, http.StatusBadRequest, errors.New("time must be HH:MM"))
		return
	}
	parts := strings.SplitN(p.Time, ":", 2)
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	if !validMealMinute(p.Meal, hour, minute) {
		writeError(w, http.StatusBadRequest,
			errors.New(p.Meal+" out of range (breakfast 07:00-11:59, lunch 12:00-17:29, dinner 17:30-23:59)"))
		return
	}
	if err := s.store.SetMealTime(today, p.Meal, p.Time); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 用户重新 set meal → 之前 clear 时被 cascade dismiss 的 supplement 通知该回归。
	// CreateNotificationIdempotent 是 INSERT OR IGNORE, 不会 undismiss 同 ref 已 dismissed
	// 的旧记录, 所以这里手动 undismiss 引用了该 meal 的 window 的通知, 让 mac UrgentNotifier
	// 下一 tick 看到 dismissed_at = NULL 自动恢复 banner。
	// 但 — 已勾选完成的 window 通知不该复活 (用户场景: 完成晚餐 → 测试 window active 发通知 →
	// 勾完测试 → 取消晚餐 → 重新完成晚餐, 此时测试 window 仍是 done 状态 (records 表里有
	// 记录), 通知不该再挂出来)。所以 undismiss 前查 records 跳过已 done 的 (id, idx)。
	suppls, _ := s.store.ListSupplements()
	doneRecs, _ := s.store.ListRecords(supplementModule, today, today)
	doneSet := map[string]bool{}
	for _, rec := range doneRecs {
		doneSet[rec.Key] = true
	}
	for _, sp := range suppls {
		for idx, w := range sp.Windows {
			if !exprUsesMeal(w.Start, p.Meal) && !exprUsesMeal(w.End, p.Meal) {
				continue
			}
			doneKey := fmt.Sprintf("%d:%d", sp.ID, idx)
			if doneSet[doneKey] {
				continue
			}
			supRef := fmt.Sprintf("supplement:%d:window:%d:date:%s", sp.ID, idx, today)
			_ = s.store.UndismissNotificationByRef(supRef)
		}
	}
	// 触发餐后立刻扫一次通知 — 否则下次生成器跑要等到 60s tick,
	// 引用了这餐的 supplement 当前若处于 active 也得拖到一分钟内才弹
	if n, err := supplements.GenerateNotifications(s.store, time.Now()); err != nil {
		log.Printf("supplement notif gen (post-meal): %v", err)
	} else if n > 0 {
		log.Printf("supplement notif gen (post-meal): %d new", n)
	}
	// 同时让 meal 提醒立刻 dismiss (用户刚记录, 不要让 '还没记录' 的通知挂在那)
	if _, err := supplements.GenerateMealNotifications(s.store, time.Now()); err != nil {
		log.Printf("meal notif gen (post-meal): %v", err)
	}
	ds, _ := s.store.GetDailyState(today)
	writeJSON(w, http.StatusOK, ds)
}

// handlePauseMeal POST /api/state/meal/{meal}/pause
// body: {"until": <unix-seconds> | null}
//   - 设暂停截止 (写 meal_settings.paused_until);null 取消暂停
//   - 立即跑一次 GenerateMealNotifications:
//       暂停场景 → 把该 meal 当前 active 通知 dismiss
//       取消场景 → 先 undismiss 现有 ref, 让 mac 端无需等 INSERT 也能看到通知回归
//   - meal ∈ {"breakfast","lunch","dinner"}
//
// 跟 supplement 的 handlePauseSupplement 同构 (paused_until / null = 取消)。
func (s *Server) handlePauseMeal(w http.ResponseWriter, r *http.Request) {
	meal := r.PathValue("meal")
	if !store.IsValidMeal(meal) {
		writeError(w, http.StatusBadRequest, errors.New("meal must be breakfast | lunch | dinner"))
		return
	}
	var p struct {
		Until *int64 `json:"until"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.SetMealPausedUntil(meal, p.Until); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 取消暂停: 先 undismiss 现有 ref, 让 mac 端 ticker 下一帧看到通知回归 (在合法时段内的话)。
	// CreateNotificationIdempotent 是 INSERT OR IGNORE, 若昨天的 ref 还在则不会重新创建。
	if p.Until == nil {
		today := dateutil.LogicalToday(time.Now())
		ref := fmt.Sprintf("meal:%s:date:%s", meal, today)
		_ = s.store.UndismissNotificationByRef(ref)
	}
	// 暂停场景由 GenerateMealNotifications 内部 dismiss; 取消场景由它内部 INSERT 新条 (老 ref 已 undismiss)。
	if _, err := supplements.GenerateMealNotifications(s.store, time.Now()); err != nil {
		log.Printf("meal notif gen (post-pause): %v", err)
	}
	today := dateutil.LogicalToday(time.Now())
	ds, _ := s.store.GetDailyState(today)
	writeJSON(w, http.StatusOK, ds)
}

// handleEnableMeal POST /api/state/meal/{meal}/enable
// body: {"enabled": true|false}
//   - 永久禁止 / 启用某餐 (跟 supplement.enable 同语义, 跟 paused_until 独立)
//   - 立即跑 GenerateMealNotifications:
//       disable → 把该 meal 当前 active 通知 dismiss
//       enable  → 先 undismiss 现有 ref, ticker 下一帧或本帧立即重新生成
//   - meal ∈ {"breakfast","lunch","dinner"}
func (s *Server) handleEnableMeal(w http.ResponseWriter, r *http.Request) {
	meal := r.PathValue("meal")
	if !store.IsValidMeal(meal) {
		writeError(w, http.StatusBadRequest, errors.New("meal must be breakfast | lunch | dinner"))
		return
	}
	var p struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.SetMealEnabled(meal, p.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if p.Enabled {
		// 启用: 先 undismiss 老 ref, GenerateMealNotifications 在合法时段内会复用同 ref
		// (CreateNotificationIdempotent INSERT IGNORE 跳过, 但 dismissed_at 已重置)
		today := dateutil.LogicalToday(time.Now())
		ref := fmt.Sprintf("meal:%s:date:%s", meal, today)
		_ = s.store.UndismissNotificationByRef(ref)
	}
	if _, err := supplements.GenerateMealNotifications(s.store, time.Now()); err != nil {
		log.Printf("meal notif gen (post-enable): %v", err)
	}
	today := dateutil.LogicalToday(time.Now())
	ds, _ := s.store.GetDailyState(today)
	writeJSON(w, http.StatusOK, ds)
}

// exprUsesMeal 检查 supplement window 时间表达式是否引用了某 meal 锚点。
// 跟 supplements.ParseTimeExpr 的 anchor prefix 匹配规则一致 (HasPrefix), 仅识别
// breakfast/lunch/dinner (wake 不算 meal, 跟 firstMealMiss 一致)。
func exprUsesMeal(expr, meal string) bool {
	expr = strings.TrimSpace(expr)
	if meal != "breakfast" && meal != "lunch" && meal != "dinner" {
		return false
	}
	return strings.HasPrefix(expr, meal)
}

// validMealMinute 餐时间区间 (分钟级):
//   breakfast 07:00-11:59 / lunch 12:00-17:29 / dinner 17:30-23:59
// 跟 mac mealMetas.validRangeMin 保持一致, 防 mac UI 已在范围内但 backend 拒绝。
func validMealMinute(meal string, hour, minute int) bool {
	mins := hour*60 + minute
	switch meal {
	case "breakfast":
		return mins >= 7*60 && mins <= 11*60+59
	case "lunch":
		return mins >= 12*60 && mins <= 17*60+29
	case "dinner":
		return mins >= 17*60+30 && mins <= 23*60+59
	}
	return false
}

func (s *Server) handlePutStateSettings(w http.ResponseWriter, r *http.Request) {
	var ss store.StateSettings
	if err := decodeJSON(r, &ss); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !hhmmRegex.MatchString(ss.FocusStart) || !hhmmRegex.MatchString(ss.SleepStart) {
		writeError(w, http.StatusBadRequest, errors.New("focus_start/sleep_start must be HH:MM"))
		return
	}
	if err := s.store.SetStateSettings(ss); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ss)
}
