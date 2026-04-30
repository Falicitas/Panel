package supplements

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"panel/dateutil"
	"panel/store"
)

// ParseTimeExpr 解析时间表达式,基于 today/wakeTime,返回当日的具体时刻。
// 支持:
//   - "HH:MM"          固定时刻
//   - "wake"           wakeTime 当时刻
//   - "wake+Nh"/-Nh    wakeTime ± N 小时
//   - "wake+Nm"/-Nm    wakeTime ± N 分钟
//
// 若 wakeTime 为空但表达式以 wake 开头,返回 ok=false。
func ParseTimeExpr(expr, wakeTime string, today time.Time) (time.Time, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return time.Time{}, false
	}
	loc := today.Location()

	// HH:MM
	if t, err := time.ParseInLocation("15:04", expr, loc); err == nil {
		return time.Date(today.Year(), today.Month(), today.Day(), t.Hour(), t.Minute(), 0, 0, loc), true
	}

	// wake[+/-N{h,m}]
	if !strings.HasPrefix(expr, "wake") {
		return time.Time{}, false
	}
	if wakeTime == "" {
		return time.Time{}, false
	}
	base, err := time.ParseInLocation("15:04", wakeTime, loc)
	if err != nil {
		return time.Time{}, false
	}
	baseT := time.Date(today.Year(), today.Month(), today.Day(), base.Hour(), base.Minute(), 0, 0, loc)
	rest := strings.TrimPrefix(expr, "wake")
	if rest == "" {
		return baseT, true
	}
	if len(rest) < 2 {
		return time.Time{}, false
	}
	sign := rest[0]
	if sign != '+' && sign != '-' {
		return time.Time{}, false
	}
	rest = rest[1:]
	unit := byte('h')
	if last := rest[len(rest)-1]; last == 'h' || last == 'm' {
		unit = last
		rest = rest[:len(rest)-1]
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return time.Time{}, false
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
	return baseT.Add(d), true
}

// IsWindowActive 当前 now 是否落在 window 内 (左闭右开)。
func IsWindowActive(w store.Window, wakeTime string, now time.Time) bool {
	start, ok := ParseTimeExpr(w.Start, wakeTime, now)
	if !ok {
		return false
	}
	end, ok := ParseTimeExpr(w.End, wakeTime, now)
	if !ok {
		return false
	}
	return !now.Before(start) && now.Before(end)
}

// GenerateNotifications 扫所有 supplements,对当前 active window 幂等写 notification。
// 返回新生成的条数。
func GenerateNotifications(s *store.Store, now time.Time) (int, error) {
	today := dateutil.LogicalToday(now)
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

	created := 0
	for _, sp := range suppls {
		for idx, w := range sp.Windows {
			if !IsWindowActive(w, state.WakeTime, now) {
				continue
			}
			ref := fmt.Sprintf("supplement:%d:window:%d:date:%s", sp.ID, idx, today)
			label := "食物"
			if sp.Type == "supplement" {
				label = "保健品"
			}
			body := fmt.Sprintf("%s 时段 %s–%s", label, w.Start, w.End)
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
