package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"panel/exec"
	"panel/store"
)

type execPayload struct {
	Cmd       string `json:"cmd"`
	TimeoutMs int    `json:"timeout_ms"`
}

func (s *Server) handlePostExec(w http.ResponseWriter, r *http.Request) {
	var p execPayload
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p.Cmd = strings.TrimSpace(p.Cmd)
	if p.Cmd == "" {
		writeError(w, http.StatusBadRequest, errors.New("cmd is empty"))
		return
	}
	timeout := exec.DefaultTimeout
	if p.TimeoutMs > 0 {
		timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}
	wd, _ := os.UserHomeDir()
	res, err := exec.Run(r.Context(), p.Cmd, timeout, wd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	entry, err := s.store.CreateBashHistory(store.BashHistoryEntry{
		Cmd:        p.Cmd,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		ExitCode:   res.ExitCode,
		DurationMs: res.DurationMs,
		CreatedAt:  time.Now().Unix(),
	})
	if err != nil {
		// 落库失败不应该阻断结果返回 — 把错误塞进 stderr,客户端仍能看到执行结果
		res.Stderr += "\n[panel: history persist failed: " + err.Error() + "]"
		writeJSON(w, http.StatusOK, mapExecResp(res, store.BashHistoryEntry{}))
		return
	}
	writeJSON(w, http.StatusOK, mapExecResp(res, entry))
}

func (s *Server) handleListBashHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := s.store.ListBashHistory(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// mapExecResp 把 exec.Result + history entry 合并成统一前端结构 (复用 BashHistoryEntry shape + timed_out flag)。
type execResp struct {
	store.BashHistoryEntry
	TimedOut bool `json:"timed_out"`
}

func mapExecResp(r exec.Result, entry store.BashHistoryEntry) execResp {
	if entry.ID == 0 {
		entry = store.BashHistoryEntry{
			Cmd:        "",
			Stdout:     r.Stdout,
			Stderr:     r.Stderr,
			ExitCode:   r.ExitCode,
			DurationMs: r.DurationMs,
			CreatedAt:  time.Now().Unix(),
		}
	}
	return execResp{BashHistoryEntry: entry, TimedOut: r.TimedOut}
}
