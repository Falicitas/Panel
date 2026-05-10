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
	OverdueDays int                  `json:"overdue_days"`
	Segment     store.EnglishSegment `json:"segment"`
}

type reviewGroupResp struct {
	Audio store.EnglishAudio `json:"audio"`
	Items []reviewItemResp   `json:"items"`
}

type reviewTodayResp struct {
	Date           string            `json:"date"`
	DailyLimit     int               `json:"daily_limit"`
	DueTotal       int               `json:"due_total"`       // 全部到期数 (不限 cap)
	ShownTotal     int               `json:"shown_total"`     // = min(due_total, daily_limit)
	PostponedTotal int               `json:"postponed_total"` // 因配额溢出本次自动延到次日的条数
	Groups         []reviewGroupResp `json:"groups"`
}

// GET /api/english/review/today
//
// 算法:
//  1. ListDueReviewItems(today) — 已按 next_review_at ASC 排序
//  2. 超出 settings.ReviewDailyLimit 的部分自动 PostponeReviewItem 到次日 (落地"配额溢出 = 类似延后")
//  3. 按 audio 分组 + 富化 segment 文本/timestamp + audio 元数据
//
// 幂等性: 用户上午看到 N 条, 下午再 GET 仍是同一组 N 条 (溢出已落地次日)。
// 跨日积压: ORDER BY next_review_at ASC 让逾期最久的先冒出, UI 用 OverdueDays 角标提示。
func (s *Server) handleEnglishReviewToday(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := dateutil.LogicalToday(now)
	tomorrow, err := nextLogicalDate(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("compute tomorrow: %w", err))
		return
	}

	settings, err := s.store.GetEnglishSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	limit := settings.ReviewDailyLimit
	if limit <= 0 {
		limit = defaultReviewDailyLimit
	}

	dueAll, err := s.store.ListDueReviewItems(today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	dueTotal := len(dueAll)
	postponedTotal := 0
	if dueTotal > limit {
		overflow := dueAll[limit:]
		for _, it := range overflow {
			if _, perr := s.store.PostponeReviewItem(it.ID, tomorrow, now.Unix()); perr != nil {
				log.Printf("auto postpone review item %d: %v", it.ID, perr)
				continue
			}
			postponedTotal++
		}
		dueAll = dueAll[:limit]
	}

	groups, err := buildReviewGroups(s.store, dueAll, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, reviewTodayResp{
		Date:           today,
		DailyLimit:     limit,
		DueTotal:       dueTotal,
		ShownTotal:     len(dueAll),
		PostponedTotal: postponedTotal,
		Groups:         groups,
	})
}

// POST /api/english/review/{id}/done — 推进 stage. 5 stage 后 mastered。
func (s *Server) handleReviewDone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now()
	today := dateutil.LogicalToday(now)
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

// buildReviewGroups 把 due 列表按 audio_id 分组 + 富化 segment 文本/timestamp + audio 元数据。
//
// 富化失败 (audio 或 segment 找不到) 时 skip 该 item 并 log warn — 不阻断响应,
// 让 UI 还能渲染其余条目 (孤儿可能因 ReplaceSegments / DeleteAudio 没及时清理留下)。
func buildReviewGroups(s *store.Store, due []store.ReviewItem, today string) ([]reviewGroupResp, error) {
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
