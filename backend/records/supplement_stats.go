package records

import (
	"strings"
	"time"

	"panel/store"
)

// SupplementMonthlyEntry 单个 (name, type) 的月度完成情况。
// expected_days: 当月有 supplement 通知触发的天数 (data-driven 分母)。
// completed_days: 当月内有 dismiss 动作的天数 (= 用户响应了通知)。
type SupplementMonthlyEntry struct {
	SupplementID  int64  `json:"supplement_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	CompletedDays int    `json:"completed_days"`
	ExpectedDays  int    `json:"expected_days"`
}

// SupplementMonthly 完整月度响应。
type SupplementMonthly struct {
	Month   string                   `json:"month"` // YYYY-MM
	Entries []SupplementMonthlyEntry `json:"entries"`
}

// ComputeSupplementMonthly 列出 month (YYYY-MM) 内每个 supplement 的完成度。
//
// 数据来源: notifications 表。source='supplement', source_ref 形如
//   "supplement:{id}:window:{idx}:date:{YYYY-MM-DD}"
// 这样我们能从 source_ref 解出日期与 supplement_id。
func ComputeSupplementMonthly(s *store.Store, month string) (SupplementMonthly, error) {
	resp := SupplementMonthly{Month: month, Entries: []SupplementMonthlyEntry{}}

	supps, err := s.ListSupplements()
	if err != nil {
		return resp, err
	}

	all, err := s.ListNotifications(true)
	if err != nil {
		return resp, err
	}

	type dayPair struct {
		expected  map[string]struct{}
		completed map[string]struct{}
	}
	bySupp := map[int64]*dayPair{}
	for _, n := range all {
		if n.Source != "supplement" || n.SourceRef == "" {
			continue
		}
		id, date, ok := parseSupplementRef(n.SourceRef)
		if !ok {
			continue
		}
		if !strings.HasPrefix(date, month+"-") {
			continue
		}
		dp, ok := bySupp[id]
		if !ok {
			dp = &dayPair{
				expected:  map[string]struct{}{},
				completed: map[string]struct{}{},
			}
			bySupp[id] = dp
		}
		dp.expected[date] = struct{}{}
		if n.DismissedAt != nil {
			dp.completed[date] = struct{}{}
		}
	}

	for _, sp := range supps {
		dp := bySupp[sp.ID]
		entry := SupplementMonthlyEntry{
			SupplementID: sp.ID,
			Name:         sp.Name,
			Type:         sp.Type,
		}
		if dp != nil {
			entry.ExpectedDays = len(dp.expected)
			entry.CompletedDays = len(dp.completed)
		}
		resp.Entries = append(resp.Entries, entry)
	}
	return resp, nil
}

// parseSupplementRef 从 "supplement:{id}:window:{idx}:date:{YYYY-MM-DD}" 提取 id 和 date。
func parseSupplementRef(ref string) (int64, string, bool) {
	parts := strings.Split(ref, ":")
	// ["supplement", "{id}", "window", "{idx}", "date", "{date}"]
	if len(parts) < 6 || parts[0] != "supplement" || parts[2] != "window" || parts[4] != "date" {
		return 0, "", false
	}
	id, err := parseInt64(parts[1])
	if err != nil {
		return 0, "", false
	}
	date := parts[5]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return 0, "", false
	}
	return id, date, true
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidInt
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var errInvalidInt = stringError("invalid int")

type stringError string

func (e stringError) Error() string { return string(e) }
