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

	// 外出模式不弹食物保健品提醒 (PRD: 外出不弹食物保健品到点提醒)
	if state.Mode == "outside" || state.Mode == "sleep" {
		return 0, nil
	}

	suppls, err := s.ListSupplements()
	if err != nil {
		return 0, err
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
		for idx, w := range sp.Windows {
			if !IsWindowActive(w, anchors, now) {
				continue
			}
			ref := fmt.Sprintf("supplement:%d:window:%d:date:%s", sp.ID, idx, today)
			label := "食物"
			if sp.Type == "supplement" {
				label = "保健品"
			}
			// 表达式 (dinner+0m 等) 解析成具体 HH:MM, panel 内通知 + mac 系统通知
			// 都读这个 body, 不再向用户暴露原始 anchor 表达式
			startStr := w.Start
			endStr := w.End
			if t, ok, _ := ParseTimeExpr(w.Start, anchors, now); ok {
				startStr = t.Format("15:04")
			}
			if t, ok, _ := ParseTimeExpr(w.End, anchors, now); ok {
				endStr = t.Format("15:04")
			}
			body := fmt.Sprintf("%s 时段 %s–%s", label, startStr, endStr)
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
			}
		}
	}
	return created, nil
}
