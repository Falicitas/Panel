package server

import (
	"errors"
	"net/http"
	"regexp"
	"time"

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
	today := time.Now().Format("2006-01-02")
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
	today := now.Format("2006-01-02")
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
	writeJSON(w, http.StatusOK, ds)
}

func (s *Server) handleGetStateSettings(w http.ResponseWriter, r *http.Request) {
	ss, err := s.store.GetStateSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ss)
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
