package server

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/english"
	"panel/store"
)

// ---- Today (核心响应) ----

type englishWindowResp struct {
	Index       int     `json:"index"`
	StartHM     string  `json:"start_hm"`
	EndHM       string  `json:"end_hm"` // = original + snoozeCount * 30min
	Status      string  `json:"status"` // upcoming | active | passed
	MinutesDone float64 `json:"minutes_done"`
	Reached     bool    `json:"reached"`
	SnoozeCount int     `json:"snooze_count"`
}

type englishTodayResp struct {
	Date         string                 `json:"date"`
	Windows      []englishWindowResp    `json:"windows"`
	TodayDone    bool                   `json:"today_done"`
	ThresholdMin int                    `json:"threshold_min"`
	TotalMinutes float64                `json:"total_minutes"`           // 当日总累计 (含窗口外)
	CurrentAudio *store.EnglishAudio    `json:"current_audio,omitempty"` // 最新未归档
	Segments     []store.EnglishSegment `json:"segments"`                // 当前 audio 的段
	DoneSegIdx   []int                  `json:"done_seg_idx"`            // 当前 audio 当日已勾的 idx
}

// GET /api/english/today
func (s *Server) handleEnglishToday(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := dateutil.LogicalToday(now)

	settings, err := s.store.GetEnglishSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	perWin, err := s.store.ComputeWindowMinutes(today, settings.Windows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	snoozeMap := english.LoadSnoozeMap(s.store, today)

	// 总累计 (含窗口外)
	totalMin, err := computeTotalMinutes(s.store, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := englishTodayResp{
		Date:         today,
		Windows:      make([]englishWindowResp, 0, len(settings.Windows)),
		ThresholdMin: settings.ThresholdMin,
		TotalMinutes: totalMin,
		Segments:     []store.EnglishSegment{},
		DoneSegIdx:   []int{},
	}

	for idx, win := range settings.Windows {
		startT, okS := parseHMToday(today, win.Start, now.Location())
		endT, okE := english.EffectiveWindowEnd(today, win.End, snoozeMap[idx], now.Location())
		wr := englishWindowResp{
			Index:       idx,
			StartHM:     win.Start,
			MinutesDone: perWin[idx],
			Reached:     perWin[idx] >= float64(settings.ThresholdMin),
			SnoozeCount: snoozeMap[idx],
		}
		if okE {
			wr.EndHM = endT.Format("15:04")
		} else {
			wr.EndHM = win.End
		}
		if okS && okE {
			switch {
			case now.Before(startT):
				wr.Status = "upcoming"
			case now.Before(endT):
				wr.Status = "active"
			default:
				wr.Status = "passed"
			}
		} else {
			wr.Status = "unknown"
		}
		resp.Windows = append(resp.Windows, wr)
		if wr.Reached {
			resp.TodayDone = true
		}
	}

	// 当前推荐 audio = 最新未归档
	audios, err := s.store.ListAudios(false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if len(audios) > 0 {
		cur := audios[0]
		resp.CurrentAudio = &cur
		segs, err := s.store.ListSegments(cur.ID)
		if err == nil {
			resp.Segments = segs
		}
		resp.DoneSegIdx = doneSegIdxForAudio(s.store, today, cur.ID)
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---- 列表 / 详情 / 文件流 ----

// GET /api/english/audios?archived=true|false (默认 false)
func (s *Server) handleListEnglishAudios(w http.ResponseWriter, r *http.Request) {
	archived := strings.EqualFold(r.URL.Query().Get("archived"), "true") ||
		r.URL.Query().Get("archived") == "1"
	audios, err := s.store.ListAudios(archived)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, audios)
}

type englishAudioDetailResp struct {
	Audio    store.EnglishAudio     `json:"audio"`
	Segments []store.EnglishSegment `json:"segments"`
}

// GET /api/english/audios/{id}
func (s *Server) handleGetEnglishAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a, err := s.store.GetAudio(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	segs, err := s.store.ListSegments(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, englishAudioDetailResp{Audio: a, Segments: segs})
}

// GET /api/english/audios/{id}/file — 流式返回本地 mp3。无 Range 支持。
func (s *Server) handleGetEnglishAudioFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a, err := s.store.GetAudio(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if a.LocalPath == "" {
		writeError(w, http.StatusNotFound, errors.New("audio not yet downloaded"))
		return
	}
	f, err := os.Open(a.LocalPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "audio/mpeg")
	if _, err := io.Copy(w, f); err != nil {
		// log only, header 已发出
		return
	}
}

// PUT /api/english/audios/{id}/segments  body: [{idx, start_ms, end_ms, text}, ...]
// mac ASR 完成后回写;事务替换全部段 + 把 segments_status 设为 ready。
func (s *Server) handlePutEnglishSegments(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var segs []store.EnglishSegment
	if err := decodeJSON(r, &segs); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.ReplaceSegments(id, segs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- check segment ----

// POST /api/english/segments/check
// body: {audio_id, seg_idx, checked, seconds}
//   - checked=true: UpsertRecord (module=english_seg, key="{aid}:{idx}", value_num=seconds)
//   - checked=false: DeleteRecord
//   - 重算所在窗口 minutes; 跨阈值 → DismissNotificationByRef;跌破 → UndismissNotificationByRef
//   - 检查 audio 全 segments 是否勾完 → SetAudioArchived
func (s *Server) handleCheckEnglishSegment(w http.ResponseWriter, r *http.Request) {
	var p struct {
		AudioID int64   `json:"audio_id"`
		SegIdx  int     `json:"seg_idx"`
		Checked bool    `json:"checked"`
		Seconds float64 `json:"seconds"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.AudioID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("audio_id required"))
		return
	}
	now := time.Now()
	today := dateutil.LogicalToday(now)
	key := fmt.Sprintf("%d:%d", p.AudioID, p.SegIdx)

	if p.Checked {
		if p.Seconds <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("seconds must be > 0 when checked"))
			return
		}
		v := p.Seconds
		if _, err := s.store.UpsertRecord(store.Record{
			Module: english.SegModule, Date: today, Key: key, ValueNum: &v,
			CreatedAt: now.Unix(),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		if err := s.store.DeleteRecord(english.SegModule, today, key); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	// 同步窗口通知 dismiss / undismiss (无脑同步: 每个窗口看现在是否达阈值)
	settings, err := s.store.GetEnglishSettings()
	if err == nil {
		perWin, err := s.store.ComputeWindowMinutes(today, settings.Windows)
		if err == nil {
			for idx := range settings.Windows {
				ref := fmt.Sprintf("english:window:%d:date:%s", idx, today)
				if perWin[idx] >= float64(settings.ThresholdMin) {
					_ = s.store.DismissNotificationByRef(ref, now.Unix())
				} else {
					_ = s.store.UndismissNotificationByRef(ref)
				}
			}
		}
	}

	// 自动归档判断: audio 所有 segments 跨日累计是否都勾过
	maybeArchiveAudio(s.store, p.AudioID, now.Unix())

	w.WriteHeader(http.StatusNoContent)
}

// ---- snooze window ----

// POST /api/english/windows/{idx}/snooze
// body: {date?: YYYY-MM-DD, count?: int}
//   - count 默认 1, 范围 [1,8]; 30min/槽 (4 = 2 小时, 8 = 4 小时)
//   - effective end = original + total_count * 30min
func (s *Server) handleSnoozeEnglishWindow(w http.ResponseWriter, r *http.Request) {
	idxStr := r.PathValue("idx")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid idx %q", idxStr))
		return
	}
	var p struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	_ = decodeJSON(r, &p) // body 可空
	if p.Date == "" {
		p.Date = dateutil.LogicalToday(time.Now())
	}
	cnt := normalizeSnoozeCount(p.Count)
	key := english.SnoozeKey(idx)
	cur := 0
	recs, _ := s.store.ListRecords(english.SnoozeModule, p.Date, p.Date)
	for _, rec := range recs {
		if rec.Key == key && rec.ValueNum != nil {
			cur = int(*rec.ValueNum)
			break
		}
	}
	next := float64(cur + cnt)
	if _, err := s.store.UpsertRecord(store.Record{
		Module: english.SnoozeModule, Date: p.Date, Key: key, ValueNum: &next,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeSnoozeCount 把 client 传的 count 收敛到 [1,8]:
//   - 0 / 缺省 → 1 (向后兼容老 mac client)
//   - 1..8 → 不变
//   - >8 → 8 (cap, 防客户端 bug 一次性 snooze 太久)
//   - 负数 → 1 (保护性处理)
func normalizeSnoozeCount(n int) int {
	if n <= 0 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

// ---- settings ----

// GET /api/english/settings
func (s *Server) handleGetEnglishSettings(w http.ResponseWriter, r *http.Request) {
	es, err := s.store.GetEnglishSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, es)
}

// PUT /api/english/settings  body: store.EnglishSettings
func (s *Server) handlePutEnglishSettings(w http.ResponseWriter, r *http.Request) {
	var es store.EnglishSettings
	if err := decodeJSON(r, &es); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// HH:MM 严格校验, 顺手验 start < end
	for i, win := range es.Windows {
		if !hhmmRegex.MatchString(strings.TrimSpace(win.Start)) ||
			!hhmmRegex.MatchString(strings.TrimSpace(win.End)) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("window %d start/end must be HH:MM", i))
			return
		}
		sm := hhmmToMin(win.Start)
		em := hhmmToMin(win.End)
		if sm >= em {
			writeError(w, http.StatusBadRequest, fmt.Errorf("window %d start must be before end", i))
			return
		}
	}
	if es.ThresholdMin <= 0 || es.ThresholdMin > 240 {
		writeError(w, http.StatusBadRequest, errors.New("threshold_min must be between 1 and 240"))
		return
	}
	if err := s.store.SetEnglishSettings(es); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 改 settings 后立刻 regen — 让 inner loop 里的 UpdateNotificationBodyByRef
	// 把已存在的 ref 同步到新 windows 边界, expire 函数把 idx 越界 / 退到 leadStart 之前的 dismiss。
	if _, err := english.GenerateNotifications(s.store, time.Now()); err != nil {
		log.Printf("english notif gen (post-settings): %v", err)
	}
	out, _ := s.store.GetEnglishSettings()
	writeJSON(w, http.StatusOK, out)
}

// hhmmToMin "HH:MM" → 分钟 (0..1439), 已经过 hhmmRegex 校验, 失败返 0。
func hhmmToMin(s string) int {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

// ---- helpers ----

// parseHMToday "HH:MM" → today + loc 的 time.Time。
func parseHMToday(today, hhmm string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02 15:04", today+" "+hhmm, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// computeTotalMinutes 当日所有 english_seg records 的 value_num 求和 / 60。
func computeTotalMinutes(s *store.Store, today string) (float64, error) {
	recs, err := s.ListRecords(english.SegModule, today, today)
	if err != nil {
		return 0, err
	}
	var sum float64
	for _, rec := range recs {
		if rec.ValueNum != nil {
			sum += *rec.ValueNum
		}
	}
	return sum / 60.0, nil
}

// doneSegIdxForAudio 拿当日勾过的 segment idx (audio 当前页用)。
func doneSegIdxForAudio(s *store.Store, today string, audioID int64) []int {
	recs, _ := s.ListRecords(english.SegModule, today, today)
	prefix := fmt.Sprintf("%d:", audioID)
	out := []int{}
	for _, rec := range recs {
		if !strings.HasPrefix(rec.Key, prefix) {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(rec.Key, prefix))
		if err != nil {
			continue
		}
		out = append(out, idx)
	}
	return out
}

// maybeArchiveAudio 跨日累计:audio 所有 segments idx 是否都勾过。是 → archive。
// 否则 (例如反勾) 取消归档。
func maybeArchiveAudio(s *store.Store, audioID, atUnix int64) {
	segs, err := s.ListSegments(audioID)
	if err != nil || len(segs) == 0 {
		return
	}
	allRecs, err := s.ListRecords(english.SegModule, "", "")
	if err != nil {
		return
	}
	prefix := fmt.Sprintf("%d:", audioID)
	checkedIdx := map[int]bool{}
	for _, rec := range allRecs {
		if !strings.HasPrefix(rec.Key, prefix) {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(rec.Key, prefix))
		if err != nil {
			continue
		}
		checkedIdx[idx] = true
	}
	a, err := s.GetAudio(audioID)
	if err != nil {
		return
	}
	if len(checkedIdx) >= len(segs) {
		if a.ArchivedAt == nil {
			_ = s.SetAudioArchived(audioID, atUnix)
		}
	} else if a.ArchivedAt != nil {
		_ = s.SetAudioArchived(audioID, 0)
	}
}
