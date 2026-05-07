package english

import (
	"testing"

	"panel/store"
)

func TestPickCandidate(t *testing.T) {
	t.Parallel()
	type seed struct {
		ref       string
		topic     string
		pub       int64
		localPath string // 空 = 没下完
		archived  int64
		failed    bool // status=failed (size cap / silence / ASR 失败)
	}
	mk := func(s seed) store.EnglishAudio {
		return store.EnglishAudio{
			Source:      "npr",
			SourceRef:   s.ref,
			Title:       "T " + s.ref,
			Description: "D " + s.ref,
			PublishedAt: s.pub,
			DurationMs:  60000,
			AudioURL:    "http://example.test/" + s.ref + ".mp3",
			LocalPath:   s.localPath,
			FetchedAt:   1700000010,
			Topic:       s.topic,
		}
	}

	cases := []struct {
		name      string
		seeds     []seed
		topic     string
		excluding []string // 排除哪些 ref (会被映射成 ID)
		wantRef   string   // 期望抽到的 ref, "" = 期望 nil
	}{
		{
			name: "topic=finance 抽到最新一条",
			seeds: []seed{
				{ref: "f1", topic: "finance", pub: 1700000100, localPath: "/x/1.mp3"},
				{ref: "f2", topic: "finance", pub: 1700000300, localPath: "/x/2.mp3"},
				{ref: "f3", topic: "finance", pub: 1700000200, localPath: "/x/3.mp3"},
			},
			topic:   "finance",
			wantRef: "f2", // 最新 pub
		},
		{
			name: "排除最新, 取次新",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000100, localPath: "/x/a.mp3"},
				{ref: "b", topic: "finance", pub: 1700000300, localPath: "/x/b.mp3"},
				{ref: "c", topic: "finance", pub: 1700000200, localPath: "/x/c.mp3"},
			},
			topic:     "finance",
			excluding: []string{"b"},
			wantRef:   "c",
		},
		{
			name: "链式排除直到没了",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000100, localPath: "/x/a.mp3"},
				{ref: "b", topic: "finance", pub: 1700000300, localPath: "/x/b.mp3"},
			},
			topic:     "finance",
			excluding: []string{"a", "b"},
			wantRef:   "", // 全排完
		},
		{
			name: "已归档不进候选",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000300, localPath: "/x/a.mp3", archived: 1700001000},
				{ref: "b", topic: "finance", pub: 1700000200, localPath: "/x/b.mp3"},
			},
			topic:   "finance",
			wantRef: "b",
		},
		{
			name: "LocalPath 空 跳过 (没下完不能听)",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000300, localPath: ""},
				{ref: "b", topic: "finance", pub: 1700000200, localPath: "/x/b.mp3"},
			},
			topic:   "finance",
			wantRef: "b",
		},
		{
			name: "status=failed 跳过 (size cap 拒 / silence / ASR 失败)",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000300, localPath: "/x/a.mp3", failed: true},
				{ref: "b", topic: "finance", pub: 1700000200, localPath: "/x/b.mp3"},
			},
			topic:   "finance",
			wantRef: "b",
		},
		{
			name: "topic 不匹配返 nil",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000300, localPath: "/x/a.mp3"},
			},
			topic:   "tech",
			wantRef: "",
		},
		{
			name: "topic 空串 不过滤 跨 topic 取最新",
			seeds: []seed{
				{ref: "f", topic: "finance", pub: 1700000100, localPath: "/x/f.mp3"},
				{ref: "s", topic: "science", pub: 1700000300, localPath: "/x/s.mp3"},
			},
			topic:   "",
			wantRef: "s",
		},
		{
			name: "case-insensitive (FINANCE 也能匹配 finance)",
			seeds: []seed{
				{ref: "a", topic: "finance", pub: 1700000300, localPath: "/x/a.mp3"},
			},
			topic:   "FINANCE",
			wantRef: "a",
		},
		{
			name:    "无 audio 返 nil",
			seeds:   nil,
			topic:   "finance",
			wantRef: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := store.Open(":memory:")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer s.Close()

			refToID := map[string]int64{}
			for _, sd := range tc.seeds {
				id, _, err := s.InsertAudioIfNew(mk(sd))
				if err != nil {
					t.Fatalf("seed %s: %v", sd.ref, err)
				}
				refToID[sd.ref] = id
				if sd.archived > 0 {
					if err := s.SetAudioArchived(id, sd.archived); err != nil {
						t.Fatalf("archive %s: %v", sd.ref, err)
					}
				}
				if sd.failed {
					if err := s.SetAudioSegmentsStatus(id, store.EnglishStatusFailed, "test failure"); err != nil {
						t.Fatalf("set failed %s: %v", sd.ref, err)
					}
				}
			}
			var excludeIDs []int64
			for _, ref := range tc.excluding {
				excludeIDs = append(excludeIDs, refToID[ref])
			}

			got, err := PickCandidate(s, tc.topic, excludeIDs)
			if err != nil {
				t.Fatalf("PickCandidate: %v", err)
			}
			if tc.wantRef == "" {
				if got != nil {
					t.Errorf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want ref=%q, got nil", tc.wantRef)
			}
			if got.SourceRef != tc.wantRef {
				t.Errorf("got ref=%q, want %q", got.SourceRef, tc.wantRef)
			}
		})
	}
}
