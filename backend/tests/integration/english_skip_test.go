package integration

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"panel/store"
)

// candidateResp / skipResp 跟 server/handlers_english.go 内部 type 同 shape。
// 这里独立定义而不 import server 包, 避免循环依赖 (server 已 import store)。
type candidateResp struct {
	Audio              *store.EnglishAudio    `json:"audio,omitempty"`
	Segments           []store.EnglishSegment `json:"segments"`
	DescriptionPreview string                 `json:"description_preview"`
}

type skipResp struct {
	ArchivedID         int64                  `json:"archived_id"`
	NextAudio          *store.EnglishAudio    `json:"next_audio,omitempty"`
	Segments           []store.EnglishSegment `json:"segments"`
	DescriptionPreview string                 `json:"description_preview"`
}

func TestEnglishCandidate_TopicFilterAndExclude(t *testing.T) {
	e := newTestEnv(t)

	// 3 个 finance + 2 个 science + 1 个 archived finance + 1 个 没下完
	e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "f1", Topic: "finance", PublishedAt: 1700000100, LocalPath: "/x/f1.mp3"})
	idF2 := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "f2", Topic: "finance", PublishedAt: 1700000300, LocalPath: "/x/f2.mp3"})
	e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "f3", Topic: "finance", PublishedAt: 1700000200, LocalPath: "/x/f3.mp3"})
	e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "fA", Topic: "finance", PublishedAt: 1700000500, LocalPath: "/x/fA.mp3", Archived: 1700000999})
	e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "fND", Topic: "finance", PublishedAt: 1700000400, LocalPath: ""}) // 没下完
	e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "s1", Topic: "science", PublishedAt: 1700000600, LocalPath: "/x/s1.mp3"})

	// 第 1 抽: topic=finance 应回 f2 (最新未归档已下完)
	status, body := e.get("/api/english/audios/candidate?topic=finance")
	e.mustOK("GET", "/candidate", status, body)
	var r1 candidateResp
	if err := json.Unmarshal(body, &r1); err != nil {
		t.Fatalf("unmarshal r1: %v", err)
	}
	if r1.Audio == nil || r1.Audio.SourceRef != "f2" {
		t.Fatalf("first candidate ref = %v, want f2", refOf(r1.Audio))
	}

	// 第 2 抽: 排除 f2 应回 f3 (次新)
	status, body = e.get("/api/english/audios/candidate?topic=finance&exclude=" + intStr(idF2))
	e.mustOK("GET", "/candidate exclude f2", status, body)
	var r2 candidateResp
	_ = json.Unmarshal(body, &r2)
	if r2.Audio == nil || r2.Audio.SourceRef != "f3" {
		t.Fatalf("second candidate ref = %v, want f3", refOf(r2.Audio))
	}

	// 第 3 抽: 排除 f2,f3 应回 f1
	status, body = e.get("/api/english/audios/candidate?topic=finance&exclude=" + intStr(idF2) + "," + intStr(r2.Audio.ID))
	e.mustOK("GET", "/candidate exclude f2 f3", status, body)
	var r3 candidateResp
	_ = json.Unmarshal(body, &r3)
	if r3.Audio == nil || r3.Audio.SourceRef != "f1" {
		t.Fatalf("third candidate ref = %v, want f1", refOf(r3.Audio))
	}

	// 第 4 抽: 全 finance 排完 应回 audio=nil (有 fND 但没下完, fA 已 archived 不进候选)
	status, body = e.get("/api/english/audios/candidate?topic=finance&exclude=" +
		intStr(idF2) + "," + intStr(r2.Audio.ID) + "," + intStr(r3.Audio.ID))
	e.mustOK("GET", "/candidate exclude all", status, body)
	var r4 candidateResp
	_ = json.Unmarshal(body, &r4)
	if r4.Audio != nil {
		t.Fatalf("fourth candidate audio = %v, want nil", r4.Audio.SourceRef)
	}

	// topic=science 切到 s1
	status, body = e.get("/api/english/audios/candidate?topic=science")
	e.mustOK("GET", "/candidate science", status, body)
	var r5 candidateResp
	_ = json.Unmarshal(body, &r5)
	if r5.Audio == nil || r5.Audio.SourceRef != "s1" {
		t.Fatalf("science candidate ref = %v, want s1", refOf(r5.Audio))
	}

	// current=<id> 也参与排除 (跟 exclude 等价)
	status, body = e.get("/api/english/audios/candidate?topic=finance&current=" + intStr(idF2))
	e.mustOK("GET", "/candidate current=f2", status, body)
	var r6 candidateResp
	_ = json.Unmarshal(body, &r6)
	if r6.Audio == nil || r6.Audio.SourceRef != "f3" {
		t.Fatalf("with current=f2 candidate = %v, want f3", refOf(r6.Audio))
	}
}

func TestEnglishCandidate_DescriptionPreviewTruncated(t *testing.T) {
	e := newTestEnv(t)

	longDesc := strings.Repeat("X", 250)
	e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "long-desc", Topic: "finance", PublishedAt: 1700000100,
		LocalPath: "/x/ld.mp3", Description: longDesc,
	})

	status, body := e.get("/api/english/audios/candidate?topic=finance")
	e.mustOK("GET", "/candidate longdesc", status, body)
	var r candidateResp
	_ = json.Unmarshal(body, &r)
	if r.Audio == nil {
		t.Fatalf("no candidate")
	}
	// 120 字 + 省略号 (1 个 rune)
	want := strings.Repeat("X", 120) + "…"
	if r.DescriptionPreview != want {
		t.Errorf("preview = %q (len %d), want truncated to 120 + …", r.DescriptionPreview, len([]rune(r.DescriptionPreview)))
	}
}

func TestEnglishSkip_ArchiveAndReturnNext(t *testing.T) {
	e := newTestEnv(t)

	idCur := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "cur", Topic: "finance", PublishedAt: 1700000100, LocalPath: "/x/cur.mp3"})
	idSel := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "sel", Topic: "science", PublishedAt: 1700000300, LocalPath: "/x/sel.mp3", Description: "selected desc"})

	body := map[string]any{
		"selected_audio_id": idSel,
		"topic":             "science",
	}
	status, raw := e.post("/api/english/audios/"+intStr(idCur)+"/skip", body)
	e.mustOK("POST", "/skip", status, raw)

	var r skipResp
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("unmarshal skip: %v", err)
	}
	if r.ArchivedID != idCur {
		t.Errorf("archived_id = %d, want %d", r.ArchivedID, idCur)
	}
	if r.NextAudio == nil || r.NextAudio.ID != idSel {
		t.Fatalf("next_audio = %v, want id=%d", r.NextAudio, idSel)
	}
	if r.DescriptionPreview != "selected desc" {
		t.Errorf("preview = %q, want short echo", r.DescriptionPreview)
	}

	// db 验: cur 已归档
	cur, err := e.store.GetAudio(idCur)
	if err != nil {
		t.Fatalf("get cur: %v", err)
	}
	if cur.ArchivedAt == nil {
		t.Errorf("cur not archived")
	}
	// sel 仍未归档
	sel, _ := e.store.GetAudio(idSel)
	if sel.ArchivedAt != nil {
		t.Errorf("sel got archived (should not)")
	}
}

func TestEnglishSkip_ValidationErrors(t *testing.T) {
	e := newTestEnv(t)

	idCur := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "cur", Topic: "finance", PublishedAt: 1700000100, LocalPath: "/x/cur.mp3"})

	// 1. selected_audio_id 缺失/0 → 400
	status, body := e.post("/api/english/audios/"+intStr(idCur)+"/skip", map[string]any{})
	if status != http.StatusBadRequest {
		t.Errorf("missing selected: status %d, want 400 (body=%s)", status, string(body))
	}

	// 2. selected_audio_id 不存在 → 404
	status, body = e.post("/api/english/audios/"+intStr(idCur)+"/skip",
		map[string]any{"selected_audio_id": int64(9999)})
	if status != http.StatusNotFound {
		t.Errorf("non-existent selected: status %d, want 404 (body=%s)", status, string(body))
	}

	// 3. selected 已归档 → 409 conflict
	idArc := e.seedEnglishAudio(seedEnglishAudioOpts{Ref: "arc", Topic: "finance", PublishedAt: 1700000200, LocalPath: "/x/arc.mp3", Archived: 1700000999})
	status, body = e.post("/api/english/audios/"+intStr(idCur)+"/skip",
		map[string]any{"selected_audio_id": idArc})
	if status != http.StatusConflict {
		t.Errorf("archived selected: status %d, want 409 (body=%s)", status, string(body))
	}

	// 4. cur 没归档 (因为 1/2/3 都失败), 验未漂移
	cur, _ := e.store.GetAudio(idCur)
	if cur.ArchivedAt != nil {
		t.Errorf("cur got archived after 3 failed /skip calls")
	}
}

// --- helpers ---

func intStr(n int64) string {
	return strconv.FormatInt(n, 10)
}

func refOf(a *store.EnglishAudio) string {
	if a == nil {
		return "<nil>"
	}
	return a.SourceRef
}
