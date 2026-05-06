package english

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
)

// 模块名 / 通知约定 (handlers_english.go 也复用这些常量字符串)。
const (
	SegModule       = "english_seg"
	SnoozeModule    = "english_snooze"
	SnoozeKeyPrefix = "window:" // records.key = "window:{idx}"
	NotifSource     = "english"
	LeadDuration    = 15 * time.Minute
	SnoozeDuration  = 30 * time.Minute
)

// SnoozeKey 格式: "window:{idx}"
func SnoozeKey(windowIdx int) string {
	return fmt.Sprintf("%s%d", SnoozeKeyPrefix, windowIdx)
}

// LoadSnoozeMap 返回 today 各 window 的 snooze 次数 (idx -> count)。
func LoadSnoozeMap(s *store.Store, today string) map[int]int {
	out := map[int]int{}
	recs, err := s.ListRecords(SnoozeModule, today, today)
	if err != nil {
		return out
	}
	for _, rec := range recs {
		if rec.ValueNum == nil {
			continue
		}
		if !strings.HasPrefix(rec.Key, SnoozeKeyPrefix) {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimPrefix(rec.Key, SnoozeKeyPrefix))
		if err != nil {
			continue
		}
		out[idx] = int(*rec.ValueNum)
	}
	return out
}

// EffectiveWindowEnd 返回 window.End 加上 snooze 偏移后的 time.Time (今日 + loc)。
// hhmm 解析失败返 zero / false。
func EffectiveWindowEnd(today, hhmm string, snoozeCount int, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02 15:04", today+" "+hhmm, loc)
	if err != nil {
		return time.Time{}, false
	}
	if snoozeCount > 0 {
		t = t.Add(time.Duration(snoozeCount) * SnoozeDuration)
	}
	return t, true
}

// notifRef 格式: "english:window:{idx}:date:{YYYY-MM-DD}"。
func notifRef(idx int, today string) string {
	return fmt.Sprintf("%s:window:%d:date:%s", NotifSource, idx, today)
}

// parseNotifRef 解析 ref → (idx, date, ok)。
func parseNotifRef(ref string) (int, string, bool) {
	parts := strings.Split(ref, ":")
	if len(parts) != 5 || parts[0] != NotifSource || parts[1] != "window" || parts[3] != "date" {
		return 0, "", false
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, "", false
	}
	return idx, parts[4], true
}

// GenerateNotifications 在每个未达阈值且现在落入 [end-15min, effective_end) 的 window 写一条通知 (幂等)。
// 同时清理今日已 effective_end 但仍未 dismissed 的过期通知。
//
// 模式完全套 supplements.GenerateNotifications:
//   - source = "english"
//   - source_ref = "english:window:{idx}:date:{YYYY-MM-DD}"
//   - 外出 / 睡眠模式不弹新通知, 但仍做过期清理
func GenerateNotifications(s *store.Store, now time.Time) (int, error) {
	today := dateutil.LogicalToday(now)

	// 跨日清理: 昨天/更早的英文精听通知自动 dismissed, 避免堆到今天
	// (与 supplements.GenerateNotifications 同模式)
	_, _ = s.DismissStaleEnglishNotifications(dateutil.LogicalDayStartUnix(now), now.Unix())

	settings, err := s.GetEnglishSettings()
	if err != nil {
		return 0, fmt.Errorf("get english settings: %w", err)
	}
	if !settings.Enabled {
		return 0, nil
	}

	state, err := s.GetDailyState(today)
	if err != nil {
		return 0, fmt.Errorf("get daily state: %w", err)
	}

	perWin, err := s.ComputeWindowMinutes(today, settings.Windows)
	if err != nil {
		return 0, fmt.Errorf("compute window minutes: %w", err)
	}
	snoozeMap := LoadSnoozeMap(s, today)

	// 过期清理 (无 mode 限制)
	expireUnfinishedNotifications(s, now, today, settings)
	// body sync (无 mode 限制) — snooze / 改 settings 都会让 effective 时段变化, 通知 body
	// 必须 sync 到新时段, 否则 panel 通知列表显示的是老时段跟 chip 不一致。
	syncEnglishNotificationBodies(s, now, today, settings, perWin, snoozeMap)

	// 外出 / 睡眠不弹新通知
	if state.Mode == "outside" || state.Mode == "sleep" {
		return 0, nil
	}

	created := 0
	for idx, w := range settings.Windows {
		if perWin[idx] >= float64(settings.ThresholdMin) {
			continue
		}
		endT, ok := EffectiveWindowEnd(today, w.End, snoozeMap[idx], now.Location())
		if !ok {
			continue
		}
		leadStart := endT.Add(-LeadDuration)
		if !now.After(leadStart) || !now.Before(endT) {
			continue
		}
		ref := notifRef(idx, today)
		body := fmt.Sprintf("精听窗口 %s–%s, 还差 %.1f 分钟",
			w.Start, endT.Format("15:04"),
			float64(settings.ThresholdMin)-perWin[idx])
		ok, err := s.CreateNotificationIdempotent(store.Notification{
			Source:    NotifSource,
			SourceRef: ref,
			Title:     "英文精听",
			Body:      body,
			Level:     "info",
			CreatedAt: now.Unix(),
		})
		if err != nil {
			return created, fmt.Errorf("create english notif: %w", err)
		}
		if ok {
			created++
		} else {
			// ref 已存在 — windows/threshold 用户改过, body 跟着 sync (跟 supplements 一致,
			// dismissed_at 不动避免复活用户主动 dismiss 的)
			_ = s.UpdateNotificationBodyByRef(ref, body)
		}
	}
	return created, nil
}

// expireUnfinishedNotifications 把已 effective_end_passed 但未 dismissed 的通知 dismissed,
// 以及已达阈值但 dismissed_at 仍未写入的也补上 dismiss (handler 与 ticker 双保险)。
func expireUnfinishedNotifications(s *store.Store, now time.Time, today string, settings store.EnglishSettings) {
	notifs, err := s.ListNotifications(false)
	if err != nil {
		return
	}
	perWin, err := s.ComputeWindowMinutes(today, settings.Windows)
	if err != nil {
		return
	}
	snoozeMap := LoadSnoozeMap(s, today)

	for _, n := range notifs {
		if n.Source != NotifSource {
			continue
		}
		idx, refDate, ok := parseNotifRef(n.SourceRef)
		if !ok || refDate != today {
			continue
		}
		// 用户删过 windows 后 idx 越界 — 老 ref 永远不会再 active, dismiss 干净
		if idx >= len(settings.Windows) {
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			continue
		}
		// 已达阈值 → 顺手 dismiss (handler 通常已做, 这里是 ticker 兜底)
		if perWin[idx] >= float64(settings.ThresholdMin) {
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
			continue
		}
		endT, ok := EffectiveWindowEnd(today, settings.Windows[idx].End, snoozeMap[idx], now.Location())
		if !ok {
			continue
		}
		// 仅过期 dismiss。lead 之前不 dismiss — 用户 snooze / 改 settings 让 endT 推后时,
		// 通知 ref 该保留 + body sync 到新时段 (syncEnglishNotificationBodies 处理),
		// dismiss 会让通知消失反而造成 panel 通知列表跟 chip 状态错位。
		if now.After(endT) {
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
		}
	}
}

// syncEnglishNotificationBodies 扫今日所有未 dismissed 的 english 通知, 把 body 同步到
// 当前 effective 时段 (snoozeMap + settings.Windows 重新计算)。CreateNotificationIdempotent
// 是 INSERT IGNORE 不覆写已存在 ref 的 body, 所以 snooze / 改 settings 后已挂着的通知
// 会显示老时段字符串, 必须显式 UPDATE 一次。
//
// 跟 supplements GenerateNotifications inner loop 的 UPDATE body 等价, 但 english 通知
// 仅在 lead range [end-15min, end) 内创建, snooze 让 leadStart / endT 整体推后后 inner
// loop 看 now < leadStart 不进, 所以这里独立扫一次。
func syncEnglishNotificationBodies(s *store.Store, now time.Time, today string, settings store.EnglishSettings, perWin map[int]float64, snoozeMap map[int]int) {
	notifs, err := s.ListNotifications(false)
	if err != nil {
		return
	}
	for _, n := range notifs {
		if n.Source != NotifSource {
			continue
		}
		idx, refDate, ok := parseNotifRef(n.SourceRef)
		if !ok || refDate != today || idx >= len(settings.Windows) {
			continue
		}
		if perWin[idx] >= float64(settings.ThresholdMin) {
			continue
		}
		endT, ok := EffectiveWindowEnd(today, settings.Windows[idx].End, snoozeMap[idx], now.Location())
		if !ok {
			continue
		}
		body := fmt.Sprintf("精听窗口 %s–%s, 还差 %.1f 分钟",
			settings.Windows[idx].Start, endT.Format("15:04"),
			float64(settings.ThresholdMin)-perWin[idx])
		_ = s.UpdateNotificationBodyByRef(n.SourceRef, body)
	}
}
