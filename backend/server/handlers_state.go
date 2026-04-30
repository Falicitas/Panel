package server

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
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
//   - 验证小时落在 meal 区间内 (breakfast 7-11, lunch 12-15, dinner 16-23)
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
	hour, _ := strconv.Atoi(strings.SplitN(p.Time, ":", 2)[0])
	if !validMealHour(p.Meal, hour) {
		writeError(w, http.StatusBadRequest,
			errors.New(p.Meal+" out of range (breakfast 7-11, lunch 12-15, dinner 16-23)"))
		return
	}
	if err := s.store.SetMealTime(today, p.Meal, p.Time); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ds, _ := s.store.GetDailyState(today)
	writeJSON(w, http.StatusOK, ds)
}

func validMealHour(meal string, hour int) bool {
	switch meal {
	case "breakfast":
		return hour >= 7 && hour <= 11
	case "lunch":
		return hour >= 12 && hour <= 15
	case "dinner":
		return hour >= 16 && hour <= 23
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
