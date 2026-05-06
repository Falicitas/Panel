package supplements

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
)

// Anchors 是时间表达式可引用的"锚点":HH:MM 字符串,空表示未设。
type Anchors struct {
	Wake      string
	Breakfast string
	Lunch     string
	Dinner    string
}

// 锚点名 → 取值;按这个顺序匹配 (注意 wake / breakfast / lunch / dinner 互不前缀,
// 但 dinner / lunch / breakfast / wake 都是单词开头,直接 HasPrefix 即可)。
func (a Anchors) byName(name string) (string, bool) {
	switch name {
	case "wake":
		return a.Wake, true
	case "breakfast":
		return a.Breakfast, true
	case "lunch":
		return a.Lunch, true
	case "dinner":
		return a.Dinner, true
	}
	return "", false
}

var anchorNames = []string{"wake", "breakfast", "lunch", "dinner"}

// ParseTimeExpr 解析时间表达式,基于 today + anchors。返回 (时刻, ok, missing)。
//   - missing: 若表达式以某锚点起头但锚点未设,填入锚点名 ("wake"/"breakfast"/...);
//     其他失败原因 missing 留空。
//
// 支持:
//   - "HH:MM"
//   - "<anchor>"           取该锚点的当日时刻
//   - "<anchor>+Nh"/-Nh    锚点 ± N 小时
//   - "<anchor>+Nm"/-Nm    锚点 ± N 分钟
func ParseTimeExpr(expr string, anchors Anchors, today time.Time) (time.Time, bool, string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return time.Time{}, false, ""
	}
	loc := today.Location()

	// HH:MM
	if t, err := time.ParseInLocation("15:04", expr, loc); err == nil {
		return time.Date(today.Year(), today.Month(), today.Day(), t.Hour(), t.Minute(), 0, 0, loc), true, ""
	}

	// <anchor>[+/-N{h,m}]
	for _, name := range anchorNames {
		if !strings.HasPrefix(expr, name) {
			continue
		}
		val, _ := anchors.byName(name)
		if val == "" {
			return time.Time{}, false, name
		}
		base, err := time.ParseInLocation("15:04", val, loc)
		if err != nil {
			return time.Time{}, false, ""
		}
		baseT := time.Date(today.Year(), today.Month(), today.Day(), base.Hour(), base.Minute(), 0, 0, loc)
		rest := strings.TrimPrefix(expr, name)
		if rest == "" {
			return baseT, true, ""
		}
		if len(rest) < 2 {
			return time.Time{}, false, ""
		}
		sign := rest[0]
		if sign != '+' && sign != '-' {
			return time.Time{}, false, ""
		}
		rest = rest[1:]
		unit := byte('h')
		if last := rest[len(rest)-1]; last == 'h' || last == 'm' {
			unit = last
			rest = rest[:len(rest)-1]
		}
		n, err := strconv.Atoi(rest)
		if err != nil {
			return time.Time{}, false, ""
		}
		if sign == '-' {
			n = -n
		}
		var d time.Duration
		if unit == 'h' {
			d = time.Duration(n) * time.Hour
		} else {
			d = time.Duration(n) * time.Minute
		}
		return baseT.Add(d), true, ""
	}
	return time.Time{}, false, ""
}

// expireUnfinishedSupplementNotifications 把今日已 effective-end-passed 但仍未完成的
// supplement 通知 dismiss。effective end = original end + snoozeCount * 30min。
// 用户语义: 通知栏不挂过期项 (跟 "完成 / 主动dismiss" 同等清理)。
func expireUnfinishedSupplementNotifications(s *store.Store, now time.Time, today string, anchors Anchors) {
	notifs, err := s.ListNotifications(false)
	if err != nil {
		return
	}
	suppls, err := s.ListSupplements()
	if err != nil {
		return
	}
	spByID := map[int64]store.Supplement{}
	for _, sp := range suppls {
		spByID[sp.ID] = sp
	}
	snoozeRecs, _ := s.ListRecords("supplement_snooze", today, today)
	snoozeMap := map[string]int{}
	for _, rec := range snoozeRecs {
		if rec.ValueNum != nil {
			snoozeMap[rec.Key] = int(*rec.ValueNum)
		}
	}
	doneRecs, _ := s.ListRecords("supplement", today, today)
	doneSet := map[string]bool{}
	for _, rec := range doneRecs {
		doneSet[rec.Key] = true
	}

	for _, n := range notifs {
		if n.Source != "supplement" {
			continue
		}
		// source_ref 形如 "supplement:{id}:window:{idx}:date:{date}", split 后 6 段
		// (date 字符串 "YYYY-MM-DD" 不含冒号)。
		parts := strings.Split(n.SourceRef, ":")
		if len(parts) != 6 || parts[0] != "supplement" || parts[2] != "window" || parts[4] != "date" {
			continue
		}
		if parts[5] != today {
			continue // 不是今天的, 跨日清理那块管
		}
		spID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		winIdx, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}
		sp, ok := spByID[spID]
		if !ok {
			// sp 已被删 (DeleteSupplement 已 cascade hard DELETE 通知, 这里是兜底防 race)
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			continue
		}
		if winIdx >= len(sp.Windows) {
			// 用户 PUT supplement 时 windows 数组缩短 (删了某 window) → 老 ref 永远不会
			// 再 active, dismiss 干净。bug 复现: PUT 加一个覆盖 now 的 window → 通知出现
			// → 立刻 PUT 删该 window → 老逻辑 continue (不 dismiss), 通知 stale 留在
			// 通知栏。
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			continue
		}
		key := fmt.Sprintf("%d:%d", spID, winIdx)
		if doneSet[key] {
			// 已 done — 防御性 dismiss: handleCheckSupplement 在勾的时候 dismiss 过, 但
			// 后续 set meal 路径里的 undismiss 可能误复活同 ref 通知 (也已加 doneSet 防护,
			// 但旧 db 状态需要 ticker 自愈)。done 的 window 通知必须保持 dismissed_at != null。
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			continue
		}
		// 优先看 start 是否引用未触发餐 — 表达式不可解析时也按 "window 已隐藏" 处理。
		// 场景: 用户原先 set dinner=19:00 → "dinner+90m" window 20:30 active 发了通知;
		//       现在 user 取消 dinner (clear) → window UI 因 RequiresMeal 隐藏,
		//       通知却没人 dismiss 留在通知栏 — 这里级联清理。
		startT, sOk, sMiss := ParseTimeExpr(sp.Windows[winIdx].Start, anchors, now)
		endT, eOk, eMiss := ParseTimeExpr(sp.Windows[winIdx].End, anchors, now)
		if !sOk || !eOk {
			// 仅在 missing != "" (引用了未设的 meal) 时 dismiss; 其它解析错误保持原行为不动。
			if sMiss != "" || eMiss != "" {
				_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			}
			continue
		}
		effectiveEndT := endT.Add(time.Duration(snoozeMap[key]) * 30 * time.Minute)
		// 三类该 dismiss 的 case (剩下 1 类 active 留着):
		//   (a) 已过期: now > effectiveEndT (原行为)
		//   (b) 锚点变到 window 之前: now < startT (anchor 改后 window 倒退到 upcoming —
		//       原本是 active 时生成的 notif body 跟新时段不符; 老 notif 该清掉, 等 window
		//       真正 active 时 GenerateNotifications inner loop 会重新 INSERT/UPDATE body)
		switch {
		case now.After(effectiveEndT):
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
		case now.Before(startT):
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
		}
	}
}

// IsWindowActive 当前 now 是否落在 window 内 (左闭右开)。
func IsWindowActive(w store.Window, anchors Anchors, now time.Time) bool {
	start, ok, _ := ParseTimeExpr(w.Start, anchors, now)
	if !ok {
		return false
	}
	end, ok, _ := ParseTimeExpr(w.End, anchors, now)
	if !ok {
		return false
	}
	return !now.Before(start) && now.Before(end)
}

// GenerateNotifications 扫所有 supplements,对当前 active window 幂等写 notification。
// 返回新生成的条数。
func GenerateNotifications(s *store.Store, now time.Time) (int, error) {
	today := dateutil.LogicalToday(now)
	// 跨日清理: 昨天/更早的食物保健品通知自动 dismissed,避免堆到今天
	_, _ = s.DismissStaleSupplementNotifications(dateutil.LogicalDayStartUnix(now), now.Unix())
	state, err := s.GetDailyState(today)
	if err != nil {
		return 0, err
	}
	// 当日过期未完成的 supplement 通知也算 dismiss (用户语义: "完成 / 过期 / 主动dismiss"
	// 都不应在通知栏挂着)
	expireUnfinishedSupplementNotifications(s, now, today, Anchors{
		Wake:      state.WakeTime,
		Breakfast: state.BreakfastAt,
		Lunch:     state.LunchAt,
		Dinner:    state.DinnerAt,
	})

	// 外出模式不弹食物保健品提醒 (PRD: 外出不弹食物保健品到点提醒)
	if state.Mode == "outside" || state.Mode == "sleep" {
		return 0, nil
	}

	suppls, err := s.ListSupplements()
	if err != nil {
		return 0, err
	}

	// 当日已勾过的 (id, idx) 集合 — 用于 completion_mode="any" 短路: 只要任一 window 已勾,
	// 后续不再发该 item 的新通知 (任一即整 item 完成)。
	// doneWinSet 是 per-window 粒度 ("{id}:{idx}"), 用于 inner loop undismiss 路径 skip
	// 已勾完成的 window — 已 done 的通知是用户主动完成 dismiss 的, 不该自动复活。
	doneRecs, _ := s.ListRecords("supplement", today, today)
	itemAnyDone := map[int64]bool{}
	doneWinSet := map[string]bool{}
	for _, rec := range doneRecs {
		parts := strings.SplitN(rec.Key, ":", 2)
		spID, perr := strconv.ParseInt(parts[0], 10, 64)
		if perr != nil {
			continue
		}
		itemAnyDone[spID] = true
		doneWinSet[rec.Key] = true
	}

	// snoozeMap key="{id}:{idx}" → 累计 30min 槽数; 算 effective end 用。
	// inner loop 用 effective active range 判 active, body endHM 也用 effective end,
	// 这样 user snooze 后 (a) original 已过期但 effective 仍 active 时通知不消失
	// (b) panel 通知 body 跟 chip 显示的时段一致。
	snoozeRecs, _ := s.ListRecords("supplement_snooze", today, today)
	snoozeMap := map[string]int{}
	for _, rec := range snoozeRecs {
		if rec.ValueNum != nil {
			snoozeMap[rec.Key] = int(*rec.ValueNum)
		}
	}

	anchors := Anchors{
		Wake:      state.WakeTime,
		Breakfast: state.BreakfastAt,
		Lunch:     state.LunchAt,
		Dinner:    state.DinnerAt,
	}

	created := 0
	for _, sp := range suppls {
		// 禁用 / 暂停期内不弹通知 (历史数据保留)
		if !sp.IsActive(now.Unix()) {
			continue
		}
		// completion_mode="any" + 该 item 当日已有任一 window 勾过 → 不再发新通知
		if sp.NormalizedCompletionMode() == store.CompletionModeAny && itemAnyDone[sp.ID] {
			continue
		}
		for idx, w := range sp.Windows {
			startT, sOk, _ := ParseTimeExpr(w.Start, anchors, now)
			endT, eOk, _ := ParseTimeExpr(w.End, anchors, now)
			if !sOk || !eOk {
				continue
			}
			snoozeKey := fmt.Sprintf("%d:%d", sp.ID, idx)
			effectiveEnd := endT.Add(time.Duration(snoozeMap[snoozeKey]) * 30 * time.Minute)
			// effective active: now ∈ [startT, effectiveEnd)
			if now.Before(startT) || !now.Before(effectiveEnd) {
				continue
			}
			ref := fmt.Sprintf("supplement:%d:window:%d:date:%s", sp.ID, idx, today)
			label := "食物"
			if sp.Type == "supplement" {
				label = "保健品"
			}
			body := fmt.Sprintf("%s 时段 %s–%s", label,
				startT.Format("15:04"), effectiveEnd.Format("15:04"))
			n := store.Notification{
				Source:    "supplement",
				SourceRef: ref,
				Title:     sp.Name,
				Body:      body,
				Level:     "info",
				CreatedAt: now.Unix(),
			}
			ok, err := s.CreateNotificationIdempotent(n)
			if err != nil {
				return created, err
			}
			if ok {
				created++
			} else {
				// ref 已存在 → body 用 effective 时段 (snoozeMap + anchors 重新算) UPDATE,
				// 跟 chip 上 GetSupplementsToday 返回的 endHM 严格一致。覆盖 issue-22 修过的
				// '改 anchor 后 body 没 sync' + 现在的 'snooze 后 body 没 sync' 两个 case。
				_ = s.UpdateNotificationBodyByRef(ref, body)
				// 重 active 路径: user 改 windows / snooze 让原已过期 dismissed 的 ref
				// 又落入 effective active. 通知该回归, 否则 panel/mac 都看不到提醒。
				// skip 已 done 的 window — 那是用户主动完成 dismiss 的, 不自动复活。
				// 用户主动点 mac banner '知道了' dismiss 的也会被这里复活 (60s ticker 内),
				// 这是有意为之的语义: dismiss 只是临时不看, 唯一彻底消除是勾完成 / snooze 出 active。
				if !doneWinSet[snoozeKey] {
					_ = s.UndismissNotificationByRef(ref)
				}
			}
		}
	}
	return created, nil
}
