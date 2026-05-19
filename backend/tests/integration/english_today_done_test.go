package integration

import (
	"encoding/json"
	"testing"

	"panel/store"
)

// TestEnglish_TodayDone_UsesDailyTotal_NotPerWindow 沉淀 issue-53 复现路径:
//
// bug: 顶部「完成 ✓ / 绿色」(today_done) 之前按"某个单独时段窗口内累计 ≥ 阈值"判定,
// 而进度条 / 「今日新学 X.X / N 分钟」用的是全天总累计 (total_minutes, 含窗口外)。
// 用户把每日精听目标设 1 分钟、练习分散在多个窗口或窗口外时, 进度条已 100% 但
// 没有任何单个窗口单独到阈值 → 要练到约 2 倍时长才变绿。
//
// 期望 (修复后): today_done 与它自己的进度条/标签一致 —— 用全天总累计判定。
// 这里用"空窗口配置"做确定性复现 (与挂钟无关): 老逻辑下空窗口 today_done 永远 false。
func TestEnglish_TodayDone_UsesDailyTotal_NotPerWindow(t *testing.T) {
	e := newTestEnv(t)

	// 阈值 1 分钟, 不配置任何时段窗口 (UI 显示「(未配置窗口)」, 是合法状态)。
	if err := e.store.SetEnglishSettings(store.EnglishSettings{
		Windows:          []store.EnglishWindow{},
		ThresholdMin:     1,
		Enabled:          true,
		ReviewDailyLimit: 15,
	}); err != nil {
		t.Fatalf("set english settings: %v", err)
	}

	id := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "cur", Topic: "finance", LocalPath: "/x/cur.mp3",
	})
	seedThreeSegments(e, id)

	// 勾一段, 计 70 秒 ≈ 1.167 分钟 ≥ 阈值 1。
	status, raw := e.post("/api/english/segments/check", map[string]any{
		"audio_id": id, "seg_idx": 0, "checked": true, "seconds": 70.0,
	})
	e.mustOK("POST", "/check", status, raw)

	status, raw = e.get("/api/english/today")
	e.mustOK("GET", "/today", status, raw)

	var today struct {
		TodayDone    bool    `json:"today_done"`
		ThresholdMin int     `json:"threshold_min"`
		TotalMinutes float64 `json:"total_minutes"`
	}
	if err := json.Unmarshal(raw, &today); err != nil {
		t.Fatalf("unmarshal today: %v", err)
	}

	if today.TotalMinutes < float64(today.ThresholdMin) {
		t.Fatalf("precondition: total_minutes=%.3f should be >= threshold=%d",
			today.TotalMinutes, today.ThresholdMin)
	}
	// 核心不变量: 阈值 > 0 时, today_done 必须等价于 "全天总累计 ≥ 阈值"
	// (与进度条 / 标签同源), 不受时段窗口是否命中影响。
	want := today.ThresholdMin > 0 && today.TotalMinutes >= float64(today.ThresholdMin)
	if today.TodayDone != want {
		t.Fatalf("today_done=%v, want %v (total_minutes=%.3f threshold=%d, windows 空)",
			today.TodayDone, want, today.TotalMinutes, today.ThresholdMin)
	}
}

// TestEnglish_TodayDone_NotDoneBelowThreshold 反向: 全天总累计 < 阈值时不应变绿。
func TestEnglish_TodayDone_NotDoneBelowThreshold(t *testing.T) {
	e := newTestEnv(t)

	if err := e.store.SetEnglishSettings(store.EnglishSettings{
		Windows:          []store.EnglishWindow{},
		ThresholdMin:     5, // 目标 5 分钟
		Enabled:          true,
		ReviewDailyLimit: 15,
	}); err != nil {
		t.Fatalf("set english settings: %v", err)
	}

	id := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "cur", Topic: "finance", LocalPath: "/x/cur.mp3",
	})
	seedThreeSegments(e, id)

	// 仅 30 秒 = 0.5 分钟 < 5 分钟。
	status, raw := e.post("/api/english/segments/check", map[string]any{
		"audio_id": id, "seg_idx": 0, "checked": true, "seconds": 30.0,
	})
	e.mustOK("POST", "/check", status, raw)

	status, raw = e.get("/api/english/today")
	e.mustOK("GET", "/today", status, raw)

	var today struct {
		TodayDone    bool    `json:"today_done"`
		TotalMinutes float64 `json:"total_minutes"`
	}
	if err := json.Unmarshal(raw, &today); err != nil {
		t.Fatalf("unmarshal today: %v", err)
	}
	if today.TodayDone {
		t.Fatalf("today_done=true but total_minutes=%.3f < threshold 5", today.TotalMinutes)
	}
}
