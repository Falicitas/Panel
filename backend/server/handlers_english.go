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
	// 精听复习队列摘要 (issue-44 引入). UI headerStats 双指标"新学 X.X / Y · 复习 W / Z 条"
	ReviewDoneToday  int `json:"review_done_today"`  // 今日已 advance 的复习条数 (stage > 0 + last_advanced_at 落今天)
	ReviewDailyLimit int `json:"review_daily_limit"` // 配额上限 (用户设置)
	ReviewDueTotal   int `json:"review_due_total"`   // 今日总到期 (含 cap 后被 postpone 的)
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

	reviewLimit := settings.ReviewDailyLimit
	if reviewLimit <= 0 {
		reviewLimit = defaultReviewDailyLimit
	}
	reviewDone, err := s.store.CountReviewDoneToday(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dueAll, err := s.store.ListDueReviewItems(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := englishTodayResp{
		Date:             today,
		Windows:          make([]englishWindowResp, 0, len(settings.Windows)),
		ThresholdMin:     settings.ThresholdMin,
		TotalMinutes:     totalMin,
		Segments:         []store.EnglishSegment{},
		DoneSegIdx:       []int{},
		ReviewDoneToday:  reviewDone,
		ReviewDailyLimit: reviewLimit,
		ReviewDueTotal:   len(dueAll),
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

	// 当前推荐 audio = 最新未归档 (M0: today 不按 topic 过滤, 主动跳音频在 M1 走 candidate API)
	audios, err := s.store.ListAudios(false, "")
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

// GET /api/english/audios?archived=true|false&topic=<free-text> (默认 archived=false, topic 不过滤)
func (s *Server) handleListEnglishAudios(w http.ResponseWriter, r *http.Request) {
	archived := strings.EqualFold(r.URL.Query().Get("archived"), "true") ||
		r.URL.Query().Get("archived") == "1"
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	audios, err := s.store.ListAudios(archived, topic)
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

// DELETE /api/english/audios/{id} — 永久删除 audio (含 segments + 本地 mp3)。
// 不存在的 id 返 404; records 中 module='english_seg' 的练习记录保留 (DeleteAudio 注释)。
func (s *Server) handleDeleteEnglishAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteAudio(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// ---- candidate / skip (Phase 14 M1: 模态框抽奖 + 跳过) ----

type englishCandidateResp struct {
	Audio              *store.EnglishAudio    `json:"audio,omitempty"`
	Segments           []store.EnglishSegment `json:"segments"`
	DescriptionPreview string                 `json:"description_preview"`
}

// GET /api/english/audios/candidate?topic=&exclude=A,B,C&current=N
//
// 模态框"抽一个 audio" 数据源。topic 为空不过滤;current/exclude 都是排除 ID。
// 不归档当前 audio (归档动作走 POST /skip, 用户确认后才归档)。
func (s *Server) handleEnglishCandidate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	topic := strings.TrimSpace(q.Get("topic"))
	excludeIDs := parseInt64CSV(q.Get("exclude"))
	if cur, err := strconv.ParseInt(strings.TrimSpace(q.Get("current")), 10, 64); err == nil && cur > 0 {
		excludeIDs = append(excludeIDs, cur)
	}

	a, err := english.PickCandidate(s.store, topic, excludeIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := englishCandidateResp{Segments: []store.EnglishSegment{}}
	if a != nil {
		resp.Audio = a
		resp.DescriptionPreview = truncateRunes(a.Description, 120)
		segs, err := s.store.ListSegments(a.ID)
		if err == nil {
			resp.Segments = segs
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type englishSkipReq struct {
	Topic            string `json:"topic"`
	SelectedAudioID  int64  `json:"selected_audio_id"`
}

type englishSkipResp struct {
	ArchivedID         int64                  `json:"archived_id"`
	NextAudio          *store.EnglishAudio    `json:"next_audio,omitempty"`
	Segments           []store.EnglishSegment `json:"segments"`
	DescriptionPreview string                 `json:"description_preview"`
}

// POST /api/english/audios/{id}/skip body: {topic?, selected_audio_id}
//
// 归档 path id 的 audio + 切到 selected_audio_id (用户在模态框已经选好)。
// selected_audio_id 必填 (== 0 视为客户端 bug, 返 400)。
func (s *Server) handleSkipEnglishAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req englishSkipReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.SelectedAudioID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("selected_audio_id required"))
		return
	}
	// 验 selected 真存在 + 未归档 (客户端模态框可能停留太久, 期间被归档了)
	sel, err := s.store.GetAudio(req.SelectedAudioID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("selected audio %d not found", req.SelectedAudioID))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if sel.ArchivedAt != nil {
		writeError(w, http.StatusConflict, fmt.Errorf("selected audio %d already archived", req.SelectedAudioID))
		return
	}

	now := time.Now().Unix()
	if err := s.store.SetAudioArchived(id, now); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 让 ListAudios 排序把 selected 顶到首位 (handleEnglishToday 取 [0] 作 currentAudio)。
	// 不更新这里, mac reloadEnglishToday 会取 published_at DESC 最新者 — 但 db 里
	// NPR/Sciam 混合时, 最新 published_at 不一定是 user 选的 → bug regression.
	if err := s.store.SetAudioLastPlayedAt(sel.ID, now); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	segs, err := s.store.ListSegments(sel.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, englishSkipResp{
		ArchivedID:         id,
		NextAudio:          &sel,
		Segments:           segs,
		DescriptionPreview: truncateRunes(sel.Description, 120),
	})
}

// truncateRunes 按 rune 数截断字符串, 防中英混排截到半个 rune (UTF-8 多字节)。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// parseInt64CSV 解析 "1,2,3" 形式 query 参数。空值或无效条目静默丢弃。
func parseInt64CSV(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil || v <= 0 {
			continue
		}
		out = append(out, v)
	}
	return out
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
		// 入队精听复习. best-effort: 失败 log warn 不阻断 200 (核心动作是勾选)。
		// EnsureReviewItem 是 idempotent — 已存在则 no-op, 不会重置 stage。
		tomorrow, err := nextLogicalDate(today)
		if err != nil {
			log.Printf("review enqueue compute tomorrow: %v", err)
		} else if _, err := s.store.EnsureReviewItem(p.AudioID, p.SegIdx, tomorrow, now.Unix()); err != nil {
			log.Printf("review enqueue %d:%d: %v", p.AudioID, p.SegIdx, err)
		}
	} else {
		if err := s.store.DeleteRecord(english.SegModule, today, key); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// 退队 — 取消勾选 = 那段没听懂 / 误勾, 整个 SRS 进度作废。
		// 重新勾选时 EnsureReviewItem 走 INSERT 路径 stage=0 重新开始。
		if err := s.store.DeleteReviewItem(p.AudioID, p.SegIdx); err != nil {
			log.Printf("review dequeue %d:%d: %v", p.AudioID, p.SegIdx, err)
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

	// Phase 14 M2 起 server 不再自动归档 — mac 端检测全勾完会弹"跳过模态框",
	// 用户选下一个 audio 后才走 POST /api/english/audios/{id}/skip 归档。
	// maybeArchiveAudio 函数本身保留(未来 fallback / 其他入口可能用)。

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
	// snooze 后 leadStart / endT 整体推后, 已存在 panel 通知 body 中的时段字符串需要 sync。
	// english.GenerateNotifications 内部跑 syncEnglishNotificationBodies 会 UPDATE body
	// (跟 supplements 的 inner loop UPDATE body 思路一致, 但 english lead range 之外
	// inner loop 不进, 所以单独抽了 sync helper)。
	if _, err := english.GenerateNotifications(s.store, time.Now()); err != nil {
		log.Printf("english notif gen (post-snooze): %v", err)
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
	// review_daily_limit 0/缺省 → 客户端没传时容忍, fallback 默认值 (PUT 全量替换语义下保留老值更安全):
	// 老 mac 客户端 PUT 时不带 review_daily_limit 会让该字段被覆盖为 0, 必须在这里 fallback。
	if es.ReviewDailyLimit <= 0 {
		old, _ := s.store.GetEnglishSettings()
		if old.ReviewDailyLimit > 0 {
			es.ReviewDailyLimit = old.ReviewDailyLimit
		} else {
			es.ReviewDailyLimit = defaultReviewDailyLimit
		}
	}
	if es.ReviewDailyLimit > 200 {
		writeError(w, http.StatusBadRequest, errors.New("review_daily_limit must be <= 200"))
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
