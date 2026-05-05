package supplements

import (
	"fmt"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
)

// mealRange 单餐的提醒规则: key 跟 daily_state 列名 / source_ref 一致, 区间是分钟级闭区间。
// 时段必须跟 mac UI mealMetas.validRangeMin 和 server.validMealMinute 保持一致。
type mealRange struct {
	key      string
	label    string
	startMin int
	endMin   int
}

var mealRanges = []mealRange{
	{"breakfast", "早餐", 7 * 60, 11*60 + 59},
	{"lunch", "午餐", 12 * 60, 17*60 + 29},
	{"dinner", "晚餐", 17*60 + 30, 23*60 + 59},
}

// GenerateMealNotifications 扫今日三餐, 在合法时段内仍未记录的餐发 panel 通知,
// 已记录的餐自动 dismiss 对应通知, 已过 valid range 仍未记的也 dismiss (过期清理)。
//
// source = "meal", source_ref = "meal:{key}:date:{date}" — 跟 supplement 的 ref 格式同构,
// CreateNotificationIdempotent 唯一约束保证一天每餐最多一条。
//
// 调用点:
//   - main.go ticker 60s
//   - handlePostMeal 写入后立即 (用户记完餐后立刻清掉对应通知)
func GenerateMealNotifications(s *store.Store, now time.Time) (int, error) {
	today := dateutil.LogicalToday(now)
	// 跨日清理: 昨天没记的早午晚餐通知不要堆到今天
	_, _ = s.DismissStaleMealNotifications(dateutil.LogicalDayStartUnix(now), now.Unix())

	state, err := s.GetDailyState(today)
	if err != nil {
		return 0, err
	}
	// 外出 / 睡眠 模式不弹三餐提醒, 跟 supplement 通知策略一致
	if state.Mode == "outside" || state.Mode == "sleep" {
		return 0, nil
	}

	recordedAt := map[string]string{
		"breakfast": state.BreakfastAt,
		"lunch":     state.LunchAt,
		"dinner":    state.DinnerAt,
	}
	nowMin := now.Hour()*60 + now.Minute()
	created := 0

	for _, m := range mealRanges {
		ref := fmt.Sprintf("meal:%s:date:%s", m.key, today)
		// 已记录 → dismiss (idempotent, 没生成过 / 已 dismissed 都安全)
		if recordedAt[m.key] != "" {
			_ = s.DismissNotificationByRef(ref, now.Unix())
			continue
		}
		// 不在合法时段 — 范围前不发, 范围后由下面 expire 清理
		if nowMin < m.startMin || nowMin > m.endMin {
			continue
		}
		body := fmt.Sprintf("还没记录 — 时段 %02d:%02d–%02d:%02d, 去面板点 '现在记'",
			m.startMin/60, m.startMin%60, m.endMin/60, m.endMin%60)
		n := store.Notification{
			Source:    "meal",
			SourceRef: ref,
			Title:     m.label,
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
		}
	}

	expireUnrecordedMealNotifications(s, now, today)
	return created, nil
}

// expireUnrecordedMealNotifications 把今日已过 valid-range-end 但仍未 dismissed 的 meal 通知 dismissed。
// 用户在合法时段内若一直没记, 时段过了就不该再挂在通知栏。已记录的 (recordedAt != "") 在主循环已 dismiss。
func expireUnrecordedMealNotifications(s *store.Store, now time.Time, today string) {
	notifs, err := s.ListNotifications(false)
	if err != nil {
		return
	}
	endMin := map[string]int{}
	for _, m := range mealRanges {
		endMin[m.key] = m.endMin
	}
	nowMin := now.Hour()*60 + now.Minute()

	for _, n := range notifs {
		if n.Source != "meal" {
			continue
		}
		// ref = "meal:{key}:date:{date}", split 4 段
		parts := strings.Split(n.SourceRef, ":")
		if len(parts) != 4 || parts[0] != "meal" || parts[2] != "date" {
			continue
		}
		if parts[3] != today {
			continue // 跨日清理那块管
		}
		end, ok := endMin[parts[1]]
		if !ok {
			continue
		}
		if nowMin > end {
			_ = s.DismissNotificationByRef(n.SourceRef, now.Unix())
		}
	}
}
