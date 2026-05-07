package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mkAudio(source, ref string) EnglishAudio {
	return EnglishAudio{
		Source:      source,
		SourceRef:   ref,
		Title:       "Title " + ref,
		Description: "Desc " + ref,
		PublishedAt: 1700000000,
		DurationMs:  60000,
		AudioURL:    "http://example.test/" + ref + ".mp3",
		FetchedAt:   1700000010,
	}
}

func TestInsertAudioIfNew_NewThenIdempotent(t *testing.T) {
	s := newTestStore(t)
	id1, isNew, err := s.InsertAudioIfNew(mkAudio("voa", "abc-123"))
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !isNew {
		t.Errorf("first insert isNew=false, want true")
	}
	if id1 == 0 {
		t.Errorf("first insert returned id=0")
	}

	id2, isNew, err := s.InsertAudioIfNew(mkAudio("voa", "abc-123"))
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if isNew {
		t.Errorf("second insert isNew=true, want false")
	}
	if id2 != id1 {
		t.Errorf("second insert id=%d, want %d", id2, id1)
	}
}

func TestInsertAudioIfNew_DefaultsAndPersistedFields(t *testing.T) {
	s := newTestStore(t)
	a := mkAudio("voa", "x")
	a.SegmentsStatus = "" // 触发默认
	a.LocalPath = ""      // 触发 NULL
	id, _, err := s.InsertAudioIfNew(a)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.GetAudio(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SegmentsStatus != EnglishStatusPending {
		t.Errorf("status = %q, want %q", got.SegmentsStatus, EnglishStatusPending)
	}
	if got.LocalPath != "" {
		t.Errorf("local_path = %q, want empty", got.LocalPath)
	}
	if got.LastPlayedAt != nil {
		t.Errorf("last_played_at = %v, want nil", got.LastPlayedAt)
	}
	if got.ArchivedAt != nil {
		t.Errorf("archived_at = %v, want nil", got.ArchivedAt)
	}
	if got.AudioURL != a.AudioURL {
		t.Errorf("audio_url roundtrip mismatch: %q vs %q", got.AudioURL, a.AudioURL)
	}
}

func TestReplaceSegments_TxAndStatusReady(t *testing.T) {
	s := newTestStore(t)
	id, _, err := s.InsertAudioIfNew(mkAudio("voa", "x"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	segs1 := []EnglishSegment{
		{Idx: 0, StartMs: 0, EndMs: 1000, Text: "Hello."},
		{Idx: 1, StartMs: 1100, EndMs: 2500, Text: "World."},
	}
	if err := s.ReplaceSegments(id, segs1); err != nil {
		t.Fatalf("replace 1st: %v", err)
	}
	got, err := s.ListSegments(id)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("after 1st replace got %d segs, want 2", len(got))
	}
	if got[0].Text != "Hello." || got[1].Text != "World." {
		t.Errorf("seg text mismatch: %+v", got)
	}

	a, err := s.GetAudio(id)
	if err != nil {
		t.Fatalf("get audio: %v", err)
	}
	if a.SegmentsStatus != EnglishStatusReady {
		t.Errorf("status = %q, want %q", a.SegmentsStatus, EnglishStatusReady)
	}

	// 第二次替换 — 旧的应被全清
	segs2 := []EnglishSegment{
		{Idx: 0, StartMs: 0, EndMs: 500, Text: "Hi."},
	}
	if err := s.ReplaceSegments(id, segs2); err != nil {
		t.Fatalf("replace 2nd: %v", err)
	}
	got2, _ := s.ListSegments(id)
	if len(got2) != 1 || got2[0].Text != "Hi." {
		t.Errorf("after 2nd replace: %+v", got2)
	}
}

func TestSetAudioStatus(t *testing.T) {
	s := newTestStore(t)
	id, _, _ := s.InsertAudioIfNew(mkAudio("voa", "x"))

	if err := s.SetAudioSegmentsStatus(id, EnglishStatusFailed, "needs_on_device_model"); err != nil {
		t.Fatalf("set status failed: %v", err)
	}
	a, _ := s.GetAudio(id)
	if a.SegmentsStatus != EnglishStatusFailed || a.SegmentsFailedReason != "needs_on_device_model" {
		t.Errorf("status/reason = %q/%q, want failed/needs_on_device_model",
			a.SegmentsStatus, a.SegmentsFailedReason)
	}

	if err := s.SetAudioSegmentsStatus(id, EnglishStatusRunning, ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	a, _ = s.GetAudio(id)
	if a.SegmentsStatus != EnglishStatusRunning || a.SegmentsFailedReason != "" {
		t.Errorf("after running: status=%q reason=%q", a.SegmentsStatus, a.SegmentsFailedReason)
	}
}

func TestDeleteAudio_NotFoundReturnsErrNoRows(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteAudio(999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteAudio(unknown) = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteAudio_RemovesAudioAndSegments(t *testing.T) {
	s := newTestStore(t)
	id, _, err := s.InsertAudioIfNew(mkAudio("voa", "del-1"))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.ReplaceSegments(id, []EnglishSegment{
		{Idx: 0, StartMs: 0, EndMs: 1000, Text: "Hello."},
		{Idx: 1, StartMs: 1000, EndMs: 2000, Text: "World."},
	}); err != nil {
		t.Fatalf("replace segments: %v", err)
	}

	// 顺手插一条 records, 验证删除后 records 仍然保留。
	v := 30.0
	if _, err := s.UpsertRecord(Record{
		Module: "english_seg", Date: "2026-05-07",
		Key: fmt.Sprintf("%d:0", id), ValueNum: &v,
	}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	if err := s.DeleteAudio(id); err != nil {
		t.Fatalf("DeleteAudio: %v", err)
	}

	if _, err := s.GetAudio(id); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetAudio(deleted) = %v, want sql.ErrNoRows", err)
	}
	segs, err := s.ListSegments(id)
	if err != nil {
		t.Fatalf("list segments: %v", err)
	}
	if len(segs) != 0 {
		t.Errorf("ListSegments(deleted) = %d entries, want 0", len(segs))
	}

	recs, err := s.ListRecords("english_seg", "2026-05-07", "2026-05-07")
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(recs) != 1 {
		t.Errorf("records preserved across DeleteAudio: got %d, want 1", len(recs))
	}
}

func TestDeleteAudio_RemovesLocalFile(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "a.mp3")
	if err := os.WriteFile(mp3, []byte("fake mp3"), 0o644); err != nil {
		t.Fatalf("write mp3: %v", err)
	}
	a := mkAudio("voa", "del-2")
	a.LocalPath = mp3
	id, _, err := s.InsertAudioIfNew(a)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := s.DeleteAudio(id); err != nil {
		t.Fatalf("DeleteAudio: %v", err)
	}
	if _, err := os.Stat(mp3); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("local file should be removed; stat err = %v", err)
	}
}

func TestDeleteAudio_MissingLocalFileIsBestEffort(t *testing.T) {
	s := newTestStore(t)
	a := mkAudio("voa", "del-3")
	a.LocalPath = "/tmp/panel-test-nonexistent-dir/never.mp3"
	id, _, err := s.InsertAudioIfNew(a)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.DeleteAudio(id); err != nil {
		t.Errorf("DeleteAudio with missing local file should not error, got %v", err)
	}
}

func TestSetAudioArchivedAndListing(t *testing.T) {
	s := newTestStore(t)
	id1, _, _ := s.InsertAudioIfNew(mkAudio("voa", "ref-1"))
	id2, _, _ := s.InsertAudioIfNew(mkAudio("voa", "ref-2"))

	active, err := s.ListAudios(false, "")
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active, got %d", len(active))
	}

	if err := s.SetAudioArchived(id1, 1700000099); err != nil {
		t.Fatalf("archive: %v", err)
	}
	active, _ = s.ListAudios(false, "")
	if len(active) != 1 || active[0].ID != id2 {
		t.Errorf("after archive id1, active=%v want id2 only", active)
	}
	archived, _ := s.ListAudios(true, "")
	if len(archived) != 1 || archived[0].ID != id1 {
		t.Errorf("archived list = %v, want [id1]", archived)
	}
	if archived[0].ArchivedAt == nil || *archived[0].ArchivedAt != 1700000099 {
		t.Errorf("archived_at not preserved")
	}

	// 取消归档
	if err := s.SetAudioArchived(id1, 0); err != nil {
		t.Fatalf("un-archive: %v", err)
	}
	a, _ := s.GetAudio(id1)
	if a.ArchivedAt != nil {
		t.Errorf("after un-archive: archived_at=%v want nil", a.ArchivedAt)
	}
}

func TestSetAudioLocalPath(t *testing.T) {
	s := newTestStore(t)
	id, _, _ := s.InsertAudioIfNew(mkAudio("voa", "x"))

	if err := s.SetAudioLocalPath(id, "/tmp/x.mp3"); err != nil {
		t.Fatalf("set: %v", err)
	}
	a, _ := s.GetAudio(id)
	if a.LocalPath != "/tmp/x.mp3" {
		t.Errorf("local_path=%q, want /tmp/x.mp3", a.LocalPath)
	}

	if err := s.SetAudioLocalPath(id, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	a, _ = s.GetAudio(id)
	if a.LocalPath != "" {
		t.Errorf("after clear: local_path=%q, want empty", a.LocalPath)
	}
}

func TestEnglishSettings_DefaultAndOverride(t *testing.T) {
	s := newTestStore(t)

	es, err := s.GetEnglishSettings()
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if !es.Enabled {
		t.Errorf("default enabled=false, want true")
	}
	if es.ThresholdMin != 10 {
		t.Errorf("default threshold=%d, want 10", es.ThresholdMin)
	}
	if len(es.Windows) != 2 {
		t.Errorf("default windows count=%d, want 2 (%v)", len(es.Windows), es.Windows)
	}

	override := EnglishSettings{
		Windows:      []EnglishWindow{{Start: "10:00", End: "11:00"}},
		ThresholdMin: 15,
		Enabled:      false,
	}
	if err := s.SetEnglishSettings(override); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.GetEnglishSettings()
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if got.Enabled || got.ThresholdMin != 15 || len(got.Windows) != 1 ||
		got.Windows[0].Start != "10:00" || got.Windows[0].End != "11:00" {
		t.Errorf("settings after override: %+v", got)
	}
}

func TestComputeWindowMinutes(t *testing.T) {
	s := newTestStore(t)
	const date = "2026-05-03"
	mkUnix := func(hhmm string) int64 {
		tm, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, time.Local)
		if err != nil {
			t.Fatalf("parseHM %q: %v", hhmm, err)
		}
		return tm.Unix()
	}
	upsertSeg := func(audioID int64, segIdx int, seconds float64, hhmm string) {
		v := seconds
		if _, err := s.UpsertRecord(Record{
			Module:    "english_seg",
			Date:      date,
			Key:       fmt.Sprintf("%d:%d", audioID, segIdx),
			ValueNum:  &v,
			CreatedAt: mkUnix(hhmm),
		}); err != nil {
			t.Fatalf("upsert seg: %v", err)
		}
	}

	upsertSeg(1, 0, 30, "08:00")  // window 0
	upsertSeg(1, 1, 90, "21:30")  // window 1
	upsertSeg(1, 2, 60, "12:00")  // 窗口外
	upsertSeg(1, 3, 120, "08:45") // window 0 (累加)

	windows := []EnglishWindow{
		{Start: "07:30", End: "09:00"},
		{Start: "21:00", End: "22:30"},
	}
	got, err := s.ComputeWindowMinutes(date, windows)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	wantW0 := (30.0 + 120.0) / 60.0 // 2.5 min
	wantW1 := 90.0 / 60.0           // 1.5 min
	if got[0] != wantW0 {
		t.Errorf("window 0: got %f, want %f", got[0], wantW0)
	}
	if got[1] != wantW1 {
		t.Errorf("window 1: got %f, want %f", got[1], wantW1)
	}
	if _, ok := got[2]; ok {
		t.Errorf("got unexpected key 2 in result")
	}
}

func TestListAudios_TopicFilter(t *testing.T) {
	s := newTestStore(t)

	// 4 个 audio 覆盖 3 种 topic + 1 个空 topic;published_at 各异看排序。
	mk := func(source, ref, topic string, pub int64) EnglishAudio {
		a := mkAudio(source, ref)
		a.Topic = topic
		a.PublishedAt = pub
		return a
	}
	type want struct {
		topic    string
		wantRefs []string // 按 published_at DESC 排
	}
	type seedAudio struct {
		audio    EnglishAudio
		archived int64 // 0 = 未归档, > 0 = archived_at
	}
	cases := []struct {
		name  string
		seeds []seedAudio
		query want
	}{
		{
			name: "topic=finance 只回 npr",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 0},
				{mk("voa", "voa-1", "general", 1700000200), 0},
				{mk("sciam60", "sci-1", "science", 1700000400), 0},
			},
			query: want{topic: "finance", wantRefs: []string{"npr-1"}},
		},
		{
			name: "topic=science 只回 sciam",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 0},
				{mk("sciam60", "sci-1", "science", 1700000400), 0},
				{mk("sciam60", "sci-2", "science", 1700000500), 0},
			},
			query: want{topic: "science", wantRefs: []string{"sci-2", "sci-1"}},
		},
		{
			name: "topic 空串 不过滤 (按 pub DESC)",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 0},
				{mk("voa", "voa-1", "general", 1700000400), 0},
			},
			query: want{topic: "", wantRefs: []string{"voa-1", "npr-1"}},
		},
		{
			name: "topic 大小写不敏感",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 0},
			},
			query: want{topic: "FINANCE", wantRefs: []string{"npr-1"}},
		},
		{
			name: "已归档不返",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 1700000999},
				{mk("npr", "npr-2", "finance", 1700000400), 0},
			},
			query: want{topic: "finance", wantRefs: []string{"npr-2"}},
		},
		{
			name: "topic=tech 没有匹配回空",
			seeds: []seedAudio{
				{mk("npr", "npr-1", "finance", 1700000300), 0},
			},
			query: want{topic: "tech", wantRefs: []string{}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s2 := newTestStore(t)
			for _, sa := range tc.seeds {
				id, _, err := s2.InsertAudioIfNew(sa.audio)
				if err != nil {
					t.Fatalf("seed %s: %v", sa.audio.SourceRef, err)
				}
				if sa.archived > 0 {
					if err := s2.SetAudioArchived(id, sa.archived); err != nil {
						t.Fatalf("archive %s: %v", sa.audio.SourceRef, err)
					}
				}
			}
			got, err := s2.ListAudios(false, tc.query.topic)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			gotRefs := make([]string, len(got))
			for i, a := range got {
				gotRefs[i] = a.SourceRef
			}
			if len(gotRefs) != len(tc.query.wantRefs) {
				t.Fatalf("len = %d, want %d (got=%v)", len(gotRefs), len(tc.query.wantRefs), gotRefs)
			}
			for i, want := range tc.query.wantRefs {
				if gotRefs[i] != want {
					t.Errorf("idx %d: got %q, want %q (full=%v)", i, gotRefs[i], want, gotRefs)
				}
			}
			_ = s
		})
	}
}

func TestComputeWindowMinutes_EmptyAndEdges(t *testing.T) {
	s := newTestStore(t)
	const date = "2026-05-03"

	// 空 windows
	out, err := s.ComputeWindowMinutes(date, nil)
	if err != nil {
		t.Fatalf("empty windows: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty windows result not empty: %v", out)
	}

	// 无 records 时所有桶为 0
	windows := []EnglishWindow{
		{Start: "07:30", End: "09:00"},
		{Start: "21:00", End: "22:30"},
	}
	out, err = s.ComputeWindowMinutes(date, windows)
	if err != nil {
		t.Fatalf("no records: %v", err)
	}
	if len(out) != 2 || out[0] != 0 || out[1] != 0 {
		t.Errorf("no records but got: %+v", out)
	}

	// 边界: created_at == startUnix 应算入,== endUnix 应不算
	mkUnix := func(hhmm string) int64 {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, time.Local)
		return tm.Unix()
	}
	v := 60.0
	if _, err := s.UpsertRecord(Record{
		Module: "english_seg", Date: date, Key: "edge:start",
		ValueNum: &v, CreatedAt: mkUnix("07:30"),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := s.UpsertRecord(Record{
		Module: "english_seg", Date: date, Key: "edge:end",
		ValueNum: &v, CreatedAt: mkUnix("09:00"),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	out, _ = s.ComputeWindowMinutes(date, windows)
	if out[0] != 1.0 { // 仅 07:30 那条 (60s = 1.0min);09:00 在端点外
		t.Errorf("boundary: window 0 = %f, want 1.0", out[0])
	}
}
