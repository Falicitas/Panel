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
	// 当日首次进入普通模式 -> 注入复发 todo 到 micro 顶部 (PRD: 蒙版选普通时注入,选外出不注入)
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

func (s *Server) injectRecurringIntoMicro() error {
	recur, err := s.store.ListRecurring()
	if err != nil {
		return err
	}
	if len(recur) == 0 {
		return nil
	}
	today, err := s.micro.LoadToday()
	if err != nil {
		return err
	}
	var sb strings.Builder
	for _, r := range recur {
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
	suppls, _ := s.store.ListSupplements()
	for _, sp := range suppls {
		for idx, w := range sp.Windows {
			if !exprUsesMeal(w.Start, p.Meal) && !exprUsesMeal(w.End, p.Meal) {
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
