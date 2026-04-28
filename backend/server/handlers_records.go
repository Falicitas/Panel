package server

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"panel/records"
	"panel/store"
)

var monthOnlyRegex = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

type gripPayload struct {
	Date  string  `json:"date"`
	Left  float64 `json:"left"`
	Right float64 `json:"right"`
	Note  string  `json:"note"`
}

func (s *Server) handlePostGrip(w http.ResponseWriter, r *http.Request) {
	var p gripPayload
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Date == "" {
		p.Date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", p.Date); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("date must be YYYY-MM-DD"))
		return
	}
	if p.Left < 0 || p.Right < 0 {
		writeError(w, http.StatusBadRequest, errors.New("left/right must be non-negative"))
		return
	}
	if p.Left == 0 && p.Right == 0 {
		writeError(w, http.StatusBadRequest, errors.New("left and right cannot both be zero"))
		return
	}
	now := time.Now().Unix()
	for _, kv := range []struct {
		key   string
		value float64
	}{
		{records.KeyLeft, p.Left},
		{records.KeyRight, p.Right},
	} {
		v := kv.value
		if _, err := s.store.UpsertRecord(store.Record{
			Module:    records.GripModule,
			Date:      p.Date,
			Key:       kv.key,
			ValueNum:  &v,
			ValueText: p.Note,
			CreatedAt: now,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	sum, err := records.ComputeGripSummary(s.store, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleGetGripToday(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	if at := r.URL.Query().Get("at"); at != "" {
		if t, err := time.Parse("2006-01-02", at); err == nil {
			now = t
		}
	}
	sum, err := records.ComputeGripSummary(s.store, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleGetSupplementsMonth(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if !monthOnlyRegex.MatchString(month) {
		writeError(w, http.StatusBadRequest, errors.New("month must be YYYY-MM"))
		return
	}
	resp, err := records.ComputeSupplementMonthly(s.store, month)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
