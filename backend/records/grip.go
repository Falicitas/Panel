// Package records 提供"指标录入 + 滚动统计 + 状态分类"的小框架。
// 首个特化: grip (晨起握力)。
package records

import (
	"math"
	"time"

	"panel/store"
)

const (
	GripModule       = "grip_strength"
	BaselineWindow   = 14
	KeyLeft          = "left"
	KeyRight         = "right"

	ClassNoData     = "no_data"     // 今日没录
	ClassNoBaseline = "no_baseline" // 数据点不够,无 baseline
	ClassNormal     = "normal"      // |z| <= 1
	ClassReduce     = "reduce"      // -2 <= z < -1
	ClassRest       = "rest"        // z < -2
)

// GripDayValue 单日握力 (左右手 + 平均)。
type GripDayValue struct {
	Date  string  `json:"date"`
	Left  float64 `json:"left"`
	Right float64 `json:"right"`
	Avg   float64 `json:"avg"`
}

// GripSummary 今日值 + 14 天 baseline + 偏离与分类。
type GripSummary struct {
	Today          *GripDayValue  `json:"today"`         // null 表示今日未录
	BaselineM      float64        `json:"baseline_m"`    // 不含今日的过去 N 天均值
	SD             float64        `json:"sd"`            // 不含今日的过去 N 天 SD
	BaselineDays   int            `json:"baseline_days"` // baseline 用了几天数据
	Deviation      float64        `json:"deviation"`     // (today.avg - M) / SD,SD=0 时为 0
	Classification string         `json:"classification"`
	Label          string         `json:"label"`
	History        []GripDayValue `json:"history"` // 最近 14 天 (含今日,如有),从老到新
}

// ComputeGripSummary 拉取最近 14 天 (含今日) 的所有 grip 记录,聚合 + 统计 + 分类。
func ComputeGripSummary(s *store.Store, today time.Time) (GripSummary, error) {
	todayStr := today.Format("2006-01-02")
	from := today.AddDate(0, 0, -(BaselineWindow - 1)).Format("2006-01-02")

	rs, err := s.ListRecords(GripModule, from, todayStr)
	if err != nil {
		return GripSummary{}, err
	}

	// 按 date 聚合 left/right
	byDate := map[string]*GripDayValue{}
	for _, r := range rs {
		if r.ValueNum == nil {
			continue
		}
		dv, ok := byDate[r.Date]
		if !ok {
			dv = &GripDayValue{Date: r.Date}
			byDate[r.Date] = dv
		}
		switch r.Key {
		case KeyLeft:
			dv.Left = *r.ValueNum
		case KeyRight:
			dv.Right = *r.ValueNum
		}
	}

	// 计算每日 avg (要求 left/right 都有值才算入 baseline,否则不可比)
	history := []GripDayValue{}
	baseline := []float64{}
	for d := today.AddDate(0, 0, -(BaselineWindow - 1)); !d.After(today); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		dv, ok := byDate[key]
		if !ok {
			continue
		}
		if dv.Left == 0 && dv.Right == 0 {
			continue
		}
		dv.Avg = (dv.Left + dv.Right) / 2
		history = append(history, *dv)
		if key != todayStr {
			baseline = append(baseline, dv.Avg)
		}
	}

	sum := GripSummary{History: history}

	// today
	if dv, ok := byDate[todayStr]; ok && (dv.Left != 0 || dv.Right != 0) {
		dv.Avg = (dv.Left + dv.Right) / 2
		copy := *dv
		sum.Today = &copy
	}

	// baseline M / SD (需要 >= 2 天才算 SD)
	if len(baseline) > 0 {
		sum.BaselineDays = len(baseline)
		sum.BaselineM = mean(baseline)
		if len(baseline) >= 2 {
			sum.SD = stdDev(baseline, sum.BaselineM)
		}
	}

	sum.Classification, sum.Label, sum.Deviation = classify(sum.Today, sum.BaselineM, sum.SD, len(baseline))
	return sum, nil
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// stdDev 样本标准差 (n-1 分母)。
func stdDev(xs []float64, m float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

func classify(today *GripDayValue, m, sd float64, baselineDays int) (string, string, float64) {
	if today == nil {
		return ClassNoData, "今日未录", 0
	}
	if baselineDays < 3 || sd == 0 {
		return ClassNoBaseline, "数据不足,继续录入累积 baseline", 0
	}
	z := (today.Avg - m) / sd
	switch {
	case z < -2:
		return ClassRest, "改有氧或休息", z
	case z < -1:
		return ClassReduce, "降强度", z
	default:
		return ClassNormal, "正常,照常训", z
	}
}
