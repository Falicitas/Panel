package integration

import (
	"encoding/json"
	"testing"

	"panel/store"
)

// seedThreeSegments 给 audio_id 注入 3 段 [0-10s, 10-20s, 20-30s] 标准 segments,
// 模拟 mac ASR PUT 完成后的状态 (segments_status 自动转 'ready')。
func seedThreeSegments(e *testEnv, audioID int64) {
	e.t.Helper()
	segs := []store.EnglishSegment{
		{Idx: 0, StartMs: 0, EndMs: 10_000, Text: "Hello."},
		{Idx: 1, StartMs: 10_000, EndMs: 20_000, Text: "World."},
		{Idx: 2, StartMs: 20_000, EndMs: 30_000, Text: "Goodbye."},
	}
	if err := e.store.ReplaceSegments(audioID, segs); err != nil {
		e.t.Fatalf("seed segments for audio %d: %v", audioID, err)
	}
}

// checkAllSegments 把 audio_id 的 3 段全部 POST /check checked=true。
func checkAllSegments(e *testEnv, audioID int64) {
	e.t.Helper()
	for i := 0; i < 3; i++ {
		body := map[string]any{
			"audio_id": audioID,
			"seg_idx":  i,
			"checked":  true,
			"seconds":  10.0,
		}
		status, raw := e.post("/api/english/segments/check", body)
		e.mustOK("POST", "/check", status, raw)
	}
}

// TestEnglish_FullCheck_NoAutoArchive 沉淀 Phase 14 M2 的 breaking change:
//   - Phase 13: 全勾完 → server maybeArchiveAudio 自动归档 → mac reload 静默切下一个
//   - M2 起: server 不自动归档 → mac 弹"跳过模态框" → 用户确认走 POST /skip 才归档
// 这里测后端语义: PUT /check 全勾完后, audio.archived_at 仍 NULL。
func TestEnglish_FullCheck_NoAutoArchive(t *testing.T) {
	e := newTestEnv(t)

	idCur := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "cur", Topic: "finance", PublishedAt: 1700000100, LocalPath: "/x/cur.mp3",
	})
	seedThreeSegments(e, idCur)
	checkAllSegments(e, idCur)

	a, err := e.store.GetAudio(idCur)
	if err != nil {
		t.Fatalf("get audio: %v", err)
	}
	if a.ArchivedAt != nil {
		t.Fatalf("M2 breaking: cur should NOT be auto-archived after full check, got archived_at=%d", *a.ArchivedAt)
	}

	// 再验: doneSegIdx 反映 3 段都已勾 (经 today endpoint)
	status, raw := e.get("/api/english/today")
	e.mustOK("GET", "/today", status, raw)
	var today struct {
		CurrentAudio *struct {
			ID int64 `json:"id"`
		} `json:"current_audio"`
		DoneSegIdx []int `json:"done_seg_idx"`
	}
	if err := json.Unmarshal(raw, &today); err != nil {
		t.Fatalf("unmarshal today: %v", err)
	}
	if today.CurrentAudio == nil || today.CurrentAudio.ID != idCur {
		t.Fatalf("today.current_audio = %+v, want id=%d (cur should still be current, not archived)", today.CurrentAudio, idCur)
	}
	if len(today.DoneSegIdx) != 3 {
		t.Errorf("doneSegIdx count = %d, want 3 (all checked)", len(today.DoneSegIdx))
	}
}

// TestEnglish_LifecycleSkipPath 完整生命周期 (M2 模态框 confirm 路径):
//  1. 全勾完 cur → not auto-archived
//  2. POST /skip with selected=next → cur archived + 返 next 详情
//  3. GET /today current_audio == next
func TestEnglish_LifecycleSkipPath(t *testing.T) {
	e := newTestEnv(t)

	idCur := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "cur", Topic: "finance", PublishedAt: 1700000100, LocalPath: "/x/cur.mp3",
	})
	idNext := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "next", Topic: "science", PublishedAt: 1700000300, LocalPath: "/x/next.mp3",
	})
	seedThreeSegments(e, idCur)
	checkAllSegments(e, idCur)

	// 模拟用户在模态框选 next + 确认
	status, raw := e.post(
		"/api/english/audios/"+intStr(idCur)+"/skip",
		map[string]any{"selected_audio_id": idNext, "topic": "science"},
	)
	e.mustOK("POST", "/skip", status, raw)

	a, _ := e.store.GetAudio(idCur)
	if a.ArchivedAt == nil {
		t.Fatalf("after /skip: cur not archived")
	}

	// /today 现在应该指向 next (cur 已归档, ListAudios(false) 头条切到 next)
	status, raw = e.get("/api/english/today")
	e.mustOK("GET", "/today", status, raw)
	var today struct {
		CurrentAudio *struct {
			ID int64 `json:"id"`
		} `json:"current_audio"`
	}
	_ = json.Unmarshal(raw, &today)
	if today.CurrentAudio == nil || today.CurrentAudio.ID != idNext {
		t.Errorf("after /skip + /today: current_audio = %+v, want id=%d", today.CurrentAudio, idNext)
	}
}
