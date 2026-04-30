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
// 数据来源 (Phase 12 起):
//   - expected_days: notifications 表 source='supplement' 的 source_ref 解出的当月日期
//     (= 当月有过 active window 触发的天数)
//   - completed_days: records 表 module='supplement' key='{id}' 的当月日期
//     (= 用户主动勾过"今天吃了"的天数,跟 dismiss notification 解耦)
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

	expectedBySupp := map[int64]map[string]struct{}{}
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
		set, ok := expectedBySupp[id]
		if !ok {
			set = map[string]struct{}{}
			expectedBySupp[id] = set
		}
		set[date] = struct{}{}
	}

	// completed 走 records 表
	from := month + "-01"
	to := month + "-31" // ListRecords 是字符串比较,YYYY-MM-31 足以涵盖所有月
	recs, err := s.ListRecords("supplement", from, to)
	if err != nil {
		return resp, err
	}
	completedBySupp := map[int64]map[string]struct{}{}
	for _, rec := range recs {
		id, perr := parseInt64(rec.Key)
		if perr != nil {
			continue
		}
		set, ok := completedBySupp[id]
		if !ok {
			set = map[string]struct{}{}
			completedBySupp[id] = set
		}
		set[rec.Date] = struct{}{}
	}

	for _, sp := range supps {
		entry := SupplementMonthlyEntry{
			SupplementID: sp.ID,
			Name:         sp.Name,
			Type:         sp.Type,
		}
		if e := expectedBySupp[sp.ID]; e != nil {
			entry.ExpectedDays = len(e)
		}
		if c := completedBySupp[sp.ID]; c != nil {
			entry.CompletedDays = len(c)
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
