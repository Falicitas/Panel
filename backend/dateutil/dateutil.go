// Package dateutil 集中"逻辑当日"的判定。
//
// 默认日界是当地 06:00:[06:00, 次日 06:00) 视作同一日历日。
// 例如 5-1 03:00 仍属 4-30;5-1 06:00 起属 5-1。
// 这样夜里 12 点后还在写 micro / 勾保健品的行为不会被错误算到次日。
package dateutil

import (
	"fmt"
	"time"
)

// DayStartHour 一日开始的小时 (本地时区)。改这里即可调日界。
const DayStartHour = 6

// LogicalToday 返回 now 对应的逻辑当日日期串 (YYYY-MM-DD,本地时区)。
func LogicalToday(now time.Time) string {
	return LogicalDate(now).Format("2006-01-02")
}

// LogicalDate 返回 now 对应逻辑当日的 time.Time(00:00 当地)。
// 仅供需要继续做时间运算的调用方;只要日期串用 LogicalToday。
func LogicalDate(now time.Time) time.Time {
	if now.Hour() < DayStartHour {
		now = now.AddDate(0, 0, -1)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// LogicalDayStartUnix 返回 now 所属逻辑当日的开始时刻 (Unix 秒)。
// 用于"跨日清理"等需要 logical day 边界的调用方。
func LogicalDayStartUnix(now time.Time) int64 {
	return LogicalDate(now).Add(time.Duration(DayStartHour) * time.Hour).Unix()
}

// LogicalDayRangeUnix 返回逻辑日 date (YYYY-MM-DD, 本地时区) 的 [start, end) Unix 秒区间:
// start = date 当地 DayStartHour:00, end = 次日当地 DayStartHour:00。
//
// 用于按 unix 时间戳列筛"落在某逻辑日内"的行。直接在 SQL 里写
// date(col, 'unixepoch', 'localtime') = ? 是按日历日切的, 跟 LogicalToday 的
// 06:00 日界对不上 — 凌晨那几小时会错位一天。
func LogicalDayRangeUnix(date string) (int64, int64, error) {
	d, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0, 0, fmt.Errorf("parse logical date %q: %w", date, err)
	}
	start := d.Add(time.Duration(DayStartHour) * time.Hour)
	return start.Unix(), start.AddDate(0, 0, 1).Unix(), nil
}
