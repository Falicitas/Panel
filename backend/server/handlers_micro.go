package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"panel/store"
	"panel/todolist"
)

func (s *Server) handleGetMicroToday(w http.ResponseWriter, r *http.Request) {
	tl, err := s.micro.LoadToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

func (s *Server) handlePutMicroToday(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var payload struct {
		RawMD string `json:"raw_md"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.micro.WriteToday(payload.RawMD); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tl, err := s.micro.LoadToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 写入后立刻 recompute 进度时间戳。失败仅记日志,不阻塞返回。
	if _, err := s.recomputeMicroProgress(tl); err != nil {
		// 静默吞 — 进度时间戳不该挡用户保存
		_ = err
	}
	writeJSON(w, http.StatusOK, tl)
}

// handleGetMicroProgress 返回当日进度 + 计数。
type microProgressResp struct {
	Date           string `json:"date"`
	MorningTotal   int    `json:"morning_total"`
	MorningDone    int    `json:"morning_done"`
	MorningStartAt *int64 `json:"morning_start_at"`
	MorningEndAt   *int64 `json:"morning_end_at"`
}

func (s *Server) handleGetMicroProgress(w http.ResponseWriter, r *http.Request) {
	tl, err := s.micro.LoadToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp, err := s.recomputeMicroProgress(tl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// recomputeMicroProgress 根据当前 micro items + morning recurring 计算 done/total,
// 必要时写入 start_at / end_at,**永不清空已存在的时间戳**。
func (s *Server) recomputeMicroProgress(tl *todolist.Todolist) (microProgressResp, error) {
	date := strings.TrimSuffix(tl.Name, ".md")

	morningRec, err := s.store.ListRecurringByKind(store.RecurringKindMorning)
	if err != nil {
		return microProgressResp{}, err
	}
	total := len(morningRec)

	// content match (trim 过的 recurring content == item.text 即视作同一项)
	morningSet := make(map[string]bool, total)
	for _, r := range morningRec {
		morningSet[strings.TrimSpace(r.Content)] = true
	}
	doneSet := make(map[string]bool, total)
	for _, it := range tl.Items {
		if !it.Checked || it.Skip {
			continue
		}
		if morningSet[strings.TrimSpace(it.Text)] {
			doneSet[strings.TrimSpace(it.Text)] = true
		}
	}
	done := len(doneSet)

	prog, err := s.store.GetMicroProgress(date)
	if err != nil {
		return microProgressResp{}, err
	}
	prog.Date = date

	now := time.Now().Unix()
	dirty := false
	if done > 0 && prog.MorningStartAt == nil {
		v := now
		prog.MorningStartAt = &v
		dirty = true
	}
	if total > 0 && done == total && prog.MorningEndAt == nil {
		v := now
		prog.MorningEndAt = &v
		// 保险:如果一进 micro 就是全勾 (start 还没写),也补 start
		if prog.MorningStartAt == nil {
			prog.MorningStartAt = &v
		}
		dirty = true
	}
	if dirty {
		if err := s.store.UpsertMicroProgress(prog); err != nil {
			return microProgressResp{}, err
		}
	}

	return microProgressResp{
		Date:           date,
		MorningTotal:   total,
		MorningDone:    done,
		MorningStartAt: prog.MorningStartAt,
		MorningEndAt:   prog.MorningEndAt,
	}, nil
}
