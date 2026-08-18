package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"panel/dateutil"
	"panel/store"
)

// 默认每日复习配额. 当 settings.ReviewDailyLimit <= 0 (老库初始 / 用户改为 0) 时 fallback 用此值。
const defaultReviewDailyLimit = 15

type reviewItemResp struct {
	store.ReviewItem
	// OverdueDays = today - next_review_at (天). 0 = 当天, >0 = 已经过去 N 天没复习。
	OverdueDays int `json:"overdue_days"`
	// DoneToday = 今天已经复习完的行. 常驻队列前缀展示, UI 灰掉 + 不给操作按钮。
	DoneToday bool                 `json:"done_today"`
	Segment   store.EnglishSegment `json:"segment"`
}

type reviewGroupResp struct {
	Audio store.EnglishAudio `json:"audio"`
	Items []reviewItemResp   `json:"items"`
}

type reviewTodayResp struct {
	Date       string `json:"date"`
	DailyLimit int    `json:"daily_limit"`
	// DoneToday = 今日已完成条数. 不受配额裁剪, 永远全量留在队列前缀。
	DoneToday int `json:"done_today"`
	// DueTotal = 还没复习的到期总数 (不限 cap)。
	DueTotal int `json:"due_total"`
	// PendingShown = 本次派发的待复习条数 = min(due_total, max(0, daily_limit-done_today))。
	PendingShown int `json:"pending_shown"`
	// DeferredTotal = due_total - pending_shown, 超出今日配额没派的, 明天再来 (库里不动)。
	DeferredTotal int               `json:"deferred_total"`
	Groups        []reviewGroupResp `json:"groups"`
}

// reviewLimitOf 取每日配额, <= 0 (老库初始 / 字段缺省) 时 fallback 默认值。
func reviewLimitOf(settings store.EnglishSettings) int {
	if settings.ReviewDailyLimit <= 0 {
		return defaultReviewDailyLimit
	}
	return settings.ReviewDailyLimit
}

// reviewPendingQuota 算今天还能派几条待复习: 配额是"当日总量", 已完成的先扣掉。
// 已完成 >= 配额时返回 0 (队列只剩已完成的前缀)。
func reviewPendingQuota(limit, doneCount int) int {
	if q := limit - doneCount; q > 0 {
		return q
	}
	return 0
}

// GET /api/english/review/today
//
// 算法:
//  1. ListReviewItemsDoneToday(today) — 今天已复习完的, 全量, 排队列前缀
//  2. ListDueReviewItems(today) — 还没复习的到期项, 已按 next_review_at ASC 排序
//  3. 待复习只派 max(0, daily_limit - 已完成) 条, 剩下的留在库里等明天
//  4. 已完成 ++ 待派 一起按 audio 分组 + 富化 segment 文本/timestamp + audio 元数据
//
// 配额是纯展示上限 — 这里绝不写库。历史上溢出部分会被 PostponeReviewItem 推到次日,
// 结果配额调小再调大条目回不来 (10→5→10 只剩 5 条), 现在改成不落地。
//
// 已完成的行 next_review_at 已经是未来日期, 不在 due 里, 所以复习完不会从队列消失;
// 完成一条 → 已完成 +1 / 待派 -1, 总行数不变, 队列不跳。
func (s *Server) handleEnglishReviewToday(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := dateutil.LogicalToday(now)

	settings, err := s.store.GetEnglishSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	limit := reviewLimitOf(settings)

	doneToday, err := s.store.ListReviewItemsDoneToday(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	due, err := s.store.ListDueReviewItems(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	pending := due
	if quota := reviewPendingQuota(limit, len(doneToday)); quota < len(pending) {
		pending = pending[:quota]
	}

	doneIDs := make(map[int64]bool, len(doneToday))
	for _, it := range doneToday {
		doneIDs[it.ID] = true
	}
	display := make([]store.ReviewItem, 0, len(doneToday)+len(pending))
	display = append(display, doneToday...)
	display = append(display, pending...)

	groups, err := buildReviewGroups(s.store, display, today, doneIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, reviewTodayResp{
		Date:          today,
		DailyLimit:    limit,
		DoneToday:     len(doneToday),
		DueTotal:      len(due),
		PendingShown:  len(pending),
		DeferredTotal: len(due) - len(pending),
		Groups:        groups,
	})
}

// POST /api/english/review/{id}/done — 推进 stage. 5 stage 后 mastered。
//
// 同一逻辑日内只允许推进一次, 重复请求 409。已完成的行现在常驻队列 (灰掉不给按钮),
// 但陈旧 UI / 直接打 API 仍可能再点一次 — 放过去会白跳一个 stage。
func (s *Server) handleReviewDone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now()
	today := dateutil.LogicalToday(now)
	dup, err := s.store.IsReviewItemDoneToday(id, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if dup {
		writeError(w, http.StatusConflict, fmt.Errorf("review item %d already reviewed today", id))
		return
	}
	if _, err := s.store.AdvanceReviewItem(id, today, now.Unix()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/english/review/done-all — 一键把今日派发的待复习项全部推进 stage。
//
// 语义对齐 GET today 派发的那批: ListDueReviewItems(today) 取前
// max(0, daily_limit - 今日已完成) 条 — 保证绝不 advance 超过用户看到过的条数,
// 也保证连点两次第二次是 no-op (第一次做完已完成数就顶满配额了)。
//
// 逐条复用 AdvanceReviewItem; 单条失败 (已 finished/mastered/被并发删) 时 skip 并
// log, 不中断整批。全部处理完返回 204。
func (s *Server) handleReviewDoneAll(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := dateutil.LogicalToday(now)

	settings, err := s.store.GetEnglishSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	doneCount, err := s.store.CountReviewDoneToday(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	due, err := s.store.ListDueReviewItems(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if quota := reviewPendingQuota(reviewLimitOf(settings), doneCount); quota < len(due) {
		due = due[:quota]
	}

	for _, it := range due {
		if _, err := s.store.AdvanceReviewItem(it.ID, today, now.Unix()); err != nil {
			log.Printf("done-all advance review %d: %v", it.ID, err)
			continue
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/english/review/{id}/postpone
// body: {to_date?: YYYY-MM-DD} (默认 +1 天)
func (s *Server) handleReviewPostpone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		ToDate string `json:"to_date"`
	}
	_ = decodeJSON(r, &p) // body 可空
	now := time.Now()
	target := p.ToDate
	if target == "" {
		today := dateutil.LogicalToday(now)
		target, err = nextLogicalDate(today)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else if _, err := time.Parse("2006-01-02", target); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid to_date %q: %w", target, err))
		return
	}
	if _, err := s.store.PostponeReviewItem(id, target, now.Unix()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/english/review/{id}/finish — 永久跳过 (用户二次确认后调用)。
func (s *Server) handleReviewFinish(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now()
	if err := s.store.FinishReviewItem(id, now.Unix()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// buildReviewGroups 把展示列表按 audio_id 分组 + 富化 segment 文本/timestamp + audio 元数据。
//
// items 顺序 = 已完成在前 / 待复习在后, 分组保留 audio 首次出现的次序, 组内也保留原顺序 —
// 已完成的行因此稳定停在队列前缀, 不会因为 next_review_at 被推到未来而排到末尾。
// doneIDs 标出哪些是今日已完成 (UI 据此灰掉 + 收起操作按钮)。
//
// 富化失败 (audio 或 segment 找不到) 时 skip 该 item 并 log warn — 不阻断响应,
// 让 UI 还能渲染其余条目 (孤儿可能因 ReplaceSegments / DeleteAudio 没及时清理留下)。
func buildReviewGroups(
	s *store.Store, due []store.ReviewItem, today string, doneIDs map[int64]bool,
) ([]reviewGroupResp, error) {
	if len(due) == 0 {
		return []reviewGroupResp{}, nil
	}
	// 收集涉及到的 audio_id, 一次性预取 audio 与 segments 避免 N+1
	audioIDs := make([]int64, 0, len(due))
	seenAudio := map[int64]bool{}
	for _, it := range due {
		if !seenAudio[it.AudioID] {
			audioIDs = append(audioIDs, it.AudioID)
			seenAudio[it.AudioID] = true
		}
	}

	audioMap := map[int64]store.EnglishAudio{}
	segMap := map[int64]map[int]store.EnglishSegment{} // audio_id -> idx -> segment
	for _, aid := range audioIDs {
		a, err := s.GetAudio(aid)
		if err != nil {
			log.Printf("review enrich: skip audio %d: %v", aid, err)
			continue
		}
		audioMap[aid] = a
		segs, err := s.ListSegments(aid)
		if err != nil {
			log.Printf("review enrich: skip segments audio %d: %v", aid, err)
			continue
		}
		m := map[int]store.EnglishSegment{}
		for _, seg := range segs {
			m[seg.Idx] = seg
		}
		segMap[aid] = m
	}

	// 按 audio 分组, 保留组内 due 自带的顺序 (next_review_at ASC, seg_idx ASC)
	groupMap := map[int64]*reviewGroupResp{}
	groupOrder := make([]int64, 0, len(audioIDs))
	for _, it := range due {
		audio, ok := audioMap[it.AudioID]
		if !ok {
			continue
		}
		segs, ok := segMap[it.AudioID]
		if !ok {
			continue
		}
		seg, ok := segs[it.SegIdx]
		if !ok {
			log.Printf("review enrich: skip orphan seg %d:%d", it.AudioID, it.SegIdx)
			continue
		}
		overdue := overdueDays(it.NextReviewAt, today)
		item := reviewItemResp{
			ReviewItem:  it,
			OverdueDays: overdue,
			DoneToday:   doneIDs[it.ID],
			Segment:     seg,
		}
		g, exists := groupMap[it.AudioID]
		if !exists {
			g = &reviewGroupResp{Audio: audio, Items: []reviewItemResp{}}
			groupMap[it.AudioID] = g
			groupOrder = append(groupOrder, it.AudioID)
		}
		g.Items = append(g.Items, item)
	}

	// 输出按 groupOrder (= 第一次出现 audio_id 的顺序, 跟 due 排序一致)
	sort.SliceStable(groupOrder, func(i, j int) bool { return false }) // no-op, 保留出现顺序
	out := make([]reviewGroupResp, 0, len(groupOrder))
	for _, aid := range groupOrder {
		out = append(out, *groupMap[aid])
	}
	return out, nil
}

// overdueDays 计算 today - nextReviewAt 的天数 (>= 0); 解析失败返回 0。
func overdueDays(nextReviewAt, today string) int {
	if nextReviewAt == "" {
		return 0
	}
	next, err := time.Parse("2006-01-02", nextReviewAt)
	if err != nil {
		return 0
	}
	tod, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	diff := int(tod.Sub(next).Hours() / 24)
	if diff < 0 {
		return 0
	}
	return diff
}

// nextLogicalDate 把 YYYY-MM-DD 加一天, 返回新日期串。
func nextLogicalDate(date string) (string, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("parse date %q: %w", date, err)
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02"), nil
}
