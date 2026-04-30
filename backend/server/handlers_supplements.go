package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
	"panel/supplements"
)

const supplementModule = "supplement"

func (s *Server) handleListSupplements(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSupplements()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateSupplement(w http.ResponseWriter, r *http.Request) {
	var sp store.Supplement
	if err := decodeJSON(r, &sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateSupplement(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateSupplement(sp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var sp store.Supplement
	if err := decodeJSON(r, &sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sp.ID = id
	if err := validateSupplement(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateSupplement(sp); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteSupplement(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/supplements/{id}/check
// body: {"date": "YYYY-MM-DD?", "checked": true}
//   - checked=true 写 records (module=supplement, key="{id}", value_num=1)
//   - checked=false 删除该记录
//   - date 缺省 = LogicalToday
func (s *Server) handleCheckSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Date    string `json:"date"`
		Checked bool   `json:"checked"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Date == "" {
		p.Date = dateutil.LogicalToday(time.Now())
	}
	key := strconv.FormatInt(id, 10)
	if p.Checked {
		one := 1.0
		_, err = s.store.UpsertRecord(store.Record{
			Module: supplementModule, Date: p.Date, Key: key, ValueNum: &one,
		})
	} else {
		err = s.store.DeleteRecord(supplementModule, p.Date, key)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/supplements/{id}/pause
// body: {"until": <unix-seconds> | null}
//   - 设暂停截止;null 取消暂停
func (s *Server) handlePauseSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Until *int64 `json:"until"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.SetSupplementPausedUntil(id, p.Until); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

// POST /api/supplements/{id}/enable
// body: {"enabled": true|false}
func (s *Server) handleEnableSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.SetSupplementEnabled(id, p.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

// supplementTodayResp / windowStatus 给前端"今日时间线"+"现在该做"用。
type windowStatus struct {
	Start    string `json:"start"`     // 原表达式
	End      string `json:"end"`
	StartHM  string `json:"start_hm"`  // 解析后的 HH:MM (无 wake / 解析失败 = "")
	EndHM    string `json:"end_hm"`
	Status   string `json:"status"`    // "passed" | "active" | "upcoming" | "unknown"
}

type supplementTodayItem struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Dose        string         `json:"dose"`
	Notes       string         `json:"notes"`
	Enabled     bool           `json:"enabled"`
	PausedUntil *int64         `json:"paused_until"`
	Paused      bool           `json:"paused"`       // 当前是否在暂停期
	DoneToday   bool           `json:"done_today"`
	Windows     []windowStatus `json:"windows"`
	HasActive   bool           `json:"has_active"`   // 任意 window 是 active
}

type supplementTodayResp struct {
	Date  string                `json:"date"`
	Items []supplementTodayItem `json:"items"`
	// 汇总
	ActiveCount int `json:"active_count"` // 当前正在窗口内的项数 (enabled & 未暂停 & has_active)
	DoneCount   int `json:"done_count"`   // 当日已勾过的总数
	TotalActive int `json:"total_active"` // enabled & 未暂停 的总项数
}

// GET /api/supplements/today
func (s *Server) handleGetSupplementsToday(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := dateutil.LogicalToday(now)
	supps, err := s.store.ListSupplements()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := s.store.GetDailyState(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	doneRecs, err := s.store.ListRecords(supplementModule, today, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	doneSet := make(map[int64]bool, len(doneRecs))
	for _, rec := range doneRecs {
		if id, err := strconv.ParseInt(rec.Key, 10, 64); err == nil {
			doneSet[id] = true
		}
	}

	resp := supplementTodayResp{Date: today, Items: []supplementTodayItem{}}
	nowUnix := now.Unix()

	for _, sp := range supps {
		paused := sp.PausedUntil != nil && *sp.PausedUntil > nowUnix
		item := supplementTodayItem{
			ID:          sp.ID,
			Name:        sp.Name,
			Type:        sp.Type,
			Dose:        sp.Dose,
			Notes:       sp.Notes,
			Enabled:     sp.Enabled,
			PausedUntil: sp.PausedUntil,
			Paused:      paused,
			DoneToday:   doneSet[sp.ID],
			Windows:     []windowStatus{},
		}
		for _, win := range sp.Windows {
			ws := windowStatus{Start: win.Start, End: win.End, Status: "unknown"}
			startT, okS := supplements.ParseTimeExpr(win.Start, state.WakeTime, now)
			endT, okE := supplements.ParseTimeExpr(win.End, state.WakeTime, now)
			if okS {
				ws.StartHM = startT.Format("15:04")
			}
			if okE {
				ws.EndHM = endT.Format("15:04")
			}
			if okS && okE {
				switch {
				case now.Before(startT):
					ws.Status = "upcoming"
				case now.Before(endT):
					ws.Status = "active"
					if sp.Enabled && !paused {
						item.HasActive = true
					}
				default:
					ws.Status = "passed"
				}
			}
			item.Windows = append(item.Windows, ws)
		}
		resp.Items = append(resp.Items, item)

		if sp.Enabled && !paused {
			resp.TotalActive++
			if item.HasActive {
				resp.ActiveCount++
			}
		}
		if item.DoneToday {
			resp.DoneCount++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func validateSupplement(sp *store.Supplement) error {
	if strings.TrimSpace(sp.Name) == "" {
		return errors.New("name empty")
	}
	if sp.Type != "food" && sp.Type != "supplement" {
		return errors.New("type must be food | supplement")
	}
	for i, w := range sp.Windows {
		if strings.TrimSpace(w.Start) == "" || strings.TrimSpace(w.End) == "" {
			return errors.New("window has empty start/end at idx " + strconv.Itoa(i))
		}
	}
	return nil
}
