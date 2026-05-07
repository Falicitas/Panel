package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"panel/english"
	"panel/store"
)

// TestEnglish_SilenceMarksFailed 沉淀 VOA Akamai CDN -90dB 占位坑 (issue-32 M0 引入静音检测后):
// 模拟 fetcher 抓回的 mp3 是静音占位 — ValidateAudioEnergy 报错 →
// SetAudioSegmentsStatus(failed, "silent ...") → 后续 mac 不会对该 audio 触发 ASR。
//
// 用 backend/english/testdata/silent.mp3 作为 fixture (已用 ffmpeg anullsrc 合成)。
func TestEnglish_SilenceMarksFailed(t *testing.T) {
	e := newTestEnv(t)

	// fixture 路径相对 testdata/integration_silent.mp3 → 用 backend/english/testdata 共享
	silentMP3 := filepath.Join("..", "..", "english", "testdata", "silent.mp3")

	// 先 seed audio (LocalPath 指向 silent.mp3, 模拟 fetcher 已下载完)
	id := e.seedEnglishAudio(seedEnglishAudioOpts{
		Ref: "silent-fixture", Topic: "general", PublishedAt: 1700000100,
		LocalPath: silentMP3,
	})

	// 直接调 ValidateAudioEnergy 模拟 fetcher.RunFetchAll 内部那一步
	err := english.ValidateAudioEnergy(silentMP3)
	if err == nil {
		t.Fatalf("silent.mp3 should be detected as silent, ValidateAudioEnergy returned nil")
	}
	if !strings.Contains(err.Error(), "silent") {
		t.Errorf("err message %q does not contain 'silent'", err.Error())
	}

	// 模拟 fetcher 把这个 audio 标 failed
	if e2 := e.store.SetAudioSegmentsStatus(id, store.EnglishStatusFailed, err.Error()); e2 != nil {
		t.Fatalf("SetAudioSegmentsStatus: %v", e2)
	}

	// 验 db 状态: status='failed' + reason 含 'silent'
	a, err := e.store.GetAudio(id)
	if err != nil {
		t.Fatalf("get audio: %v", err)
	}
	if a.SegmentsStatus != store.EnglishStatusFailed {
		t.Errorf("status = %q, want %q", a.SegmentsStatus, store.EnglishStatusFailed)
	}
	if !strings.Contains(a.SegmentsFailedReason, "silent") {
		t.Errorf("reason = %q, want contains 'silent'", a.SegmentsFailedReason)
	}
}

// TestEnglish_VoicedPasses 反向校验: 正常人声 mp3 ValidateAudioEnergy 不报错。
// 用 voiced.mp3 (sine 440Hz, 32KB) 作为 fixture。
func TestEnglish_VoicedPasses(t *testing.T) {
	voicedMP3 := filepath.Join("..", "..", "english", "testdata", "voiced.mp3")
	if err := english.ValidateAudioEnergy(voicedMP3); err != nil {
		t.Errorf("voiced.mp3 should pass, got error: %v", err)
	}
}
