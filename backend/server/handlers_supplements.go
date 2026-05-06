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

// supplementSnoozeModule: records 表里 (module, date, key) 三元组,
// key="{id}:{idx}", value_num = 已 snooze 次数 (整数)。effective end =
// original end + value_num * 30min, 让 SupplementsView 的 window 范围 /
// status / 时间轴 都自动延后, mac 系统通知的 lead range 也跟着延。
const supplementSnoozeModule = "supplement_snooze"

// kickSupplementsRegen 在 supplements CRUD / enable / pause 之后立刻跑一次
// GenerateNotifications, 让新增/改后的通知不必等 60s ticker 才出现。
// 同步调, 失败静默 log (ticker 后续 retry)。
func (s *Server) kickSupplementsRegen() {
	if _, err := supplements.GenerateNotifications(s.store, time.Now()); err != nil {
		// log only; 60s ticker 会兜底
		_ = err
	}
}

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
	s.kickSupplementsRegen()
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
	s.kickSupplementsRegen()
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

// POST /api/supplements/reorder body: {"order": [id1, id2, ...]}
// 按数组顺序重写 sort_order = 索引+1, 未列出 item 保留原 sort_order。
func (s *Server) handleReorderSupplements(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Order []int64 `json:"order"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.ReorderSupplements(p.Order); err != nil {
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
	// 不需 regen — DeleteSupplement 已 cascade 抹通知, item 也没了不会再生成。
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
	ref := fmt.Sprintf("supplement:%d:window:%d:date:%s", id, p.WindowIndex, p.Date)
	if p.Checked {
		one := 1.0
		_, err = s.store.UpsertRecord(store.Record{
			Module: supplementModule, Date: p.Date, Key: key, ValueNum: &one,
		})
		if err == nil {
			// completion_mode="any" 下勾任一 window → 视为整 item 当日完成,
			// 把该 item 当日所有未 dismissed 通知一并清掉, 避免多余提醒。
			// "all" 模式下保持现有行为: 仅 dismiss 当前 window 对应通知。
			now := time.Now().Unix()
			sp, gerr := s.store.GetSupplement(id)
			if gerr == nil && sp.NormalizedCompletionMode() == store.CompletionModeAny {
				_, _ = s.store.DismissSupplementItemNotificationsForDate(id, p.Date, now)
			} else {
				_ = s.store.DismissNotificationByRef(ref, now)
			}
		}
	} else {
		err = s.store.DeleteRecord(supplementModule, p.Date, key)
		if err == nil {
			// 取消勾选 → 通知回归。逻辑跟勾选时对称, 防止 mac UrgentNotifier 状态错位:
			//   "all" 模式: 仅复活该 window 通知 (勾选时也只 dismiss 了这一条)
			//   "any" 模式: 复活整 item 当日通知 (跟 DismissSupplementItemNotificationsForDate 配对)。
			// any 模式如果只复活当前 window, 会出现 "勾时全部 dismiss → 取消勾仅一个复活, 其它 window
			// 通知永久 dismissed, idempotent INSERT 跳过 → 后续 active window 不再发 mac banner" 的 bug。
			// 复活后立即 kickSupplementsRegen → expire 会清掉已过期的, 仍 active 的保留 → 状态对齐。
			sp, gerr := s.store.GetSupplement(id)
			if gerr == nil && sp.NormalizedCompletionMode() == store.CompletionModeAny {
				_ = s.store.UndismissSupplementItemNotificationsForDate(id, p.Date)
				s.kickSupplementsRegen()
			} else {
				_ = s.store.UndismissNotificationByRef(ref)
			}
		}
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
	s.kickSupplementsRegen()
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
	if !p.Enabled {
		// disable 后已生成的待处理通知应抹掉 (item 仍在但暂不发, 通知留着无意义)
		_, _ = s.store.DeleteNotificationsBySupplementID(id)
	}
	s.kickSupplementsRegen()
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

// POST /api/supplements/{id}/snooze
// body: {"date": "YYYY-MM-DD?", "window_index": int, "count"?: int}
//   - 把 (id, idx, date) 的 snooze count 加 count (默认 1, 范围 [1,8])
//   - effective end = original end + count * 30min, GetSupplementsToday 会反映
//   - 用户在 mac 系统通知点 "延后 30 分钟" → count=1; "延后 2 小时" → count=4 (4 个 30min 槽)
//   - cap 8 防客户端 bug 一次性写超大值 (8*30min = 4h, 单次延 4h 已属边界)
func (s *Server) handleSnoozeSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Date        string `json:"date"`
		WindowIndex int    `json:"window_index"`
		Count       int    `json:"count"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if p.Date == "" {
		p.Date = dateutil.LogicalToday(time.Now())
	}
	cnt := normalizeSnoozeCount(p.Count)
	key := fmt.Sprintf("%d:%d", id, p.WindowIndex)
	cur := 0
	recs, _ := s.store.ListRecords(supplementSnoozeModule, p.Date, p.Date)
	for _, rec := range recs {
		if rec.Key == key && rec.ValueNum != nil {
			cur = int(*rec.ValueNum)
			break
		}
	}
	next := float64(cur + cnt)
	if _, err := s.store.UpsertRecord(store.Record{
		Module: supplementSnoozeModule, Date: p.Date, Key: key, ValueNum: &next,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// kick 一次 GenerateNotifications: snooze 后 effective end 推后, 已存在的 panel 通知
	// body 还是老时段字符串 ("时段 21:00–21:30"). inner loop 看 IsWindowActive (active 区间)
	// 仍然 true (snooze 让 endT 推后, now 必然仍在 active), INSERT IGNORE 跳过 → 走
	// UpdateNotificationBodyByRef 把 body 同步到新 endHM. 否则 panel 通知和 chip 时段不一致。
	s.kickSupplementsRegen()
	w.WriteHeader(http.StatusNoContent)
}

// supplementTodayResp / windowStatus 给前端 per-item 视图用。
type windowStatus struct {
	Start         string `json:"start"`          // 原表达式
	End           string `json:"end"`
	StartHM       string `json:"start_hm"`       // 解析后 HH:MM (锚点未触发/解析失败 = "")
	EndHM         string `json:"end_hm"`
	Status        string `json:"status"`         // "passed" | "active" | "upcoming" | "unknown"
	DoneToday     bool   `json:"done_today"`     // 该 window 当日是否已勾
	RequiresMeal  string `json:"requires_meal"`  // "breakfast"/"lunch"/"dinner" — 表达式引用了未触发的餐;前端隐藏
}

type supplementTodayItem struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Dose           string         `json:"dose"`
	Notes          string         `json:"notes"`
	Enabled        bool           `json:"enabled"`
	PausedUntil    *int64         `json:"paused_until"`
	Paused         bool           `json:"paused"` // 当前是否在暂停期
	// DoneToday 语义随 CompletionMode 变化:
	//   "all" → 所有可见 window (RequiresMeal == "" 部分) 都勾完且不存在 pending 才 true
	//   "any" → 任一 window 勾过即 true
	DoneToday      bool           `json:"done_today"`
	CompletionMode string         `json:"completion_mode"` // "all" | "any" — 前端用于显示完成方式 chip
	Windows        []windowStatus `json:"windows"`
	HasActive      bool           `json:"has_active"` // 任意 window 是 active
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
	// 加载当日 snooze 计数: snoozeMap[id][idx] = N → effective end = original end + N*30min
	snoozeRecs, _ := s.store.ListRecords(supplementSnoozeModule, today, today)
	snoozeMap := map[int64]map[int]int{}
	for _, rec := range snoozeRecs {
		id, idx, ok := parseSupplementKey(rec.Key)
		if !ok || rec.ValueNum == nil {
			continue
		}
		if snoozeMap[id] == nil {
			snoozeMap[id] = map[int]int{}
		}
		snoozeMap[id][idx] = int(*rec.ValueNum)
	}

	anchors := supplements.Anchors{
		Wake:      state.WakeTime,
		Breakfast: state.BreakfastAt,
		Lunch:     state.LunchAt,
		Dinner:    state.DinnerAt,
	}

	resp := supplementTodayResp{Date: today, Items: []supplementTodayItem{}}
	nowUnix := now.Unix()

	for _, sp := range supps {
		paused := sp.PausedUntil != nil && *sp.PausedUntil > nowUnix
		mode := sp.NormalizedCompletionMode()
		item := supplementTodayItem{
			ID:             sp.ID,
			Name:           sp.Name,
			Type:           sp.Type,
			Dose:           sp.Dose,
			Notes:          sp.Notes,
			Enabled:        sp.Enabled,
			PausedUntil:    sp.PausedUntil,
			Paused:         paused,
			CompletionMode: mode,
			Windows:        []windowStatus{},
		}
		// "all" 模式下 done_today 必须 (a) 至少 1 个 window 已浮现 (b) 全部已浮现 window 都勾过 (c) 没有等待餐触发的 window;
		// "any" 模式下 done_today = 任一 window 勾过即可 (含未浮现的 — 实际 UI 不能勾, 但语义上一致)。
		visibleCount := 0
		doneVisible := 0
		hasPending := false
		anyDone := false
		for idx, win := range sp.Windows {
			ws := windowStatus{
				Start:     win.Start,
				End:       win.End,
				Status:    "unknown",
				DoneToday: doneSet[sp.ID][idx],
			}
			startT, okS, missS := supplements.ParseTimeExpr(win.Start, anchors, now)
			endT, okE, missE := supplements.ParseTimeExpr(win.End, anchors, now)
			if miss := firstMealMiss(missS, missE); miss != "" {
				ws.RequiresMeal = miss
			}
			effectiveEndT := endT
			if okE {
				if cnt := snoozeMap[sp.ID][idx]; cnt > 0 {
					effectiveEndT = endT.Add(time.Duration(cnt) * 30 * time.Minute)
				}
			}
			if okS {
				ws.StartHM = startT.Format("15:04")
			}
			if okE {
				ws.EndHM = effectiveEndT.Format("15:04")
			}
			if okS && okE {
				switch {
				case now.Before(startT):
					ws.Status = "upcoming"
				case now.Before(effectiveEndT):
					ws.Status = "active"
					if sp.Enabled && !paused {
						item.HasActive = true
					}
				default:
					ws.Status = "passed"
				}
			}
			item.Windows = append(item.Windows, ws)
			if ws.DoneToday {
				anyDone = true
			}
			if ws.RequiresMeal != "" {
				hasPending = true
				continue
			}
			visibleCount++
			if ws.DoneToday {
				doneVisible++
			}
		}

		if mode == store.CompletionModeAny {
			item.DoneToday = anyDone
		} else {
			item.DoneToday = visibleCount > 0 && doneVisible == visibleCount && !hasPending
		}
		resp.Items = append(resp.Items, item)

		if sp.Enabled && !paused {
			resp.TotalActive++
			if item.HasActive {
				resp.ActiveCount++
			}
			// total_doses 语义:
			//   "all" → item 一日要做 N 次 (N = window 数, 含等餐 window)
			//   "any" → item 一日要做 1 次 (任一 window 完成即整 item 完成)
			if mode == store.CompletionModeAny {
				resp.TotalDoses++
				if anyDone {
					resp.DoneDoses++
				}
			} else {
				for _, ws := range item.Windows {
					resp.TotalDoses++
					if ws.DoneToday {
						resp.DoneDoses++
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// firstMealMiss 返回 a/b 里第一个非空且非 wake 的 anchor 名;否则 ""。
func firstMealMiss(a, b string) string {
	for _, x := range []string{a, b} {
		if x == "breakfast" || x == "lunch" || x == "dinner" {
			return x
		}
	}
	return ""
}

func validateSupplement(sp *store.Supplement) error {
	if strings.TrimSpace(sp.Name) == "" {
		return errors.New("name empty")
	}
	if sp.Type != "food" && sp.Type != "supplement" {
		return errors.New("type must be food | supplement")
	}
	// 空字符串 (老 client) 默认 "all"。其它值拒掉, 避免 silent 落库。
	if sp.CompletionMode == "" {
		sp.CompletionMode = store.CompletionModeAll
	}
	if sp.CompletionMode != store.CompletionModeAll && sp.CompletionMode != store.CompletionModeAny {
		return errors.New("completion_mode must be all | any")
	}
	for i, w := range sp.Windows {
		if strings.TrimSpace(w.Start) == "" || strings.TrimSpace(w.End) == "" {
			return errors.New("window has empty start/end at idx " + strconv.Itoa(i))
		}
	}
	return nil
}
