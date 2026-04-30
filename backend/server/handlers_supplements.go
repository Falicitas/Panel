package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
	"panel/supplements"
)

// parseSupplementKey 解析 records.key:
//   - "1:2" → (id=1, idx=2, ok=true)
//   - "1"   → (id=1, idx=0, ok=true) (老格式,等价 idx=0)
//   - 否则 (0, 0, false)
func parseSupplementKey(key string) (int64, int, bool) {
	parts := strings.SplitN(key, ":", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 1 {
		return id, 0, true
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return id, idx, true
}

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
// body: {"date": "YYYY-MM-DD?", "window_index": int, "checked": true}
//   - 每个 window 独立 done 信号,key 格式 "{id}:{window_idx}"
//   - 兼容老格式 "{id}" (不带 idx,等价于 idx=0)
//   - checked=true 写 records (module=supplement, value_num=1)
//   - checked=false 删除该记录
//   - date 缺省 = LogicalToday;window_index 缺省 = 0
func (s *Server) handleCheckSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Date        string `json:"date"`
		WindowIndex int    `json:"window_index"`
		Checked     bool   `json:"checked"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Date == "" {
		p.Date = dateutil.LogicalToday(time.Now())
	}
	key := fmt.Sprintf("%d:%d", id, p.WindowIndex)
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

// supplementTodayResp / windowStatus 给前端 per-item 视图用。
type windowStatus struct {
	Start     string `json:"start"`      // 原表达式
	End       string `json:"end"`
	StartHM   string `json:"start_hm"`   // 解析后 HH:MM (wake 解析失败 = "")
	EndHM     string `json:"end_hm"`
	Status    string `json:"status"`     // "passed" | "active" | "upcoming" | "unknown"
	DoneToday bool   `json:"done_today"` // 该 window 当日是否已勾
}

type supplementTodayItem struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Dose        string         `json:"dose"`
	Notes       string         `json:"notes"`
	Enabled     bool           `json:"enabled"`
	PausedUntil *int64         `json:"paused_until"`
	Paused      bool           `json:"paused"`     // 当前是否在暂停期
	DoneToday   bool           `json:"done_today"` // 所有 window 都勾完才算 (无 window 时 false)
	Windows     []windowStatus `json:"windows"`
	HasActive   bool           `json:"has_active"` // 任意 window 是 active
}

type supplementTodayResp struct {
	Date        string                `json:"date"`
	Items       []supplementTodayItem `json:"items"`
	ActiveCount int                   `json:"active_count"` // 当前正在窗口内的项数 (enabled & 未暂停 & has_active)
	TotalActive int                   `json:"total_active"` // enabled & 未暂停 的总项数
	TotalDoses  int                   `json:"total_doses"`  // 所有 enabled & 未暂停 项的 window 数之和
	DoneDoses   int                   `json:"done_doses"`   // 上面里已勾过的 window 数
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
	// id -> set(window_idx) 当日已勾
	doneSet := map[int64]map[int]bool{}
	for _, rec := range doneRecs {
		id, idx, ok := parseSupplementKey(rec.Key)
		if !ok {
			continue
		}
		if doneSet[id] == nil {
			doneSet[id] = map[int]bool{}
		}
		doneSet[id][idx] = true
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
			Windows:     []windowStatus{},
		}
		allDone := len(sp.Windows) > 0
		for idx, win := range sp.Windows {
			ws := windowStatus{
				Start:     win.Start,
				End:       win.End,
				Status:    "unknown",
				DoneToday: doneSet[sp.ID][idx],
			}
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
			if !ws.DoneToday {
				allDone = false
			}
			if sp.Enabled && !paused {
				resp.TotalDoses++
				if ws.DoneToday {
					resp.DoneDoses++
				}
			}
		}
		item.DoneToday = allDone
		resp.Items = append(resp.Items, item)

		if sp.Enabled && !paused {
			resp.TotalActive++
			if item.HasActive {
				resp.ActiveCount++
			}
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
