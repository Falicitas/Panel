package english

import (
	"strings"
	"testing"
)

func TestValidateAudioEnergy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		path     string
		wantErr  bool
		wantHint string // 错误消息子串 (用于验失败原因)
	}{
		{
			name:     "silent mp3 (anullsrc 占位) → 静音 error",
			path:     "testdata/silent.mp3",
			wantErr:  true,
			wantHint: "silent",
		},
		{
			name:    "voiced mp3 (440Hz sine) → 通过",
			path:    "testdata/voiced.mp3",
			wantErr: false,
		},
		{
			name:    "不存在的文件 → open error",
			path:    "testdata/does-not-exist.mp3",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAudioEnergy(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if tc.wantHint != "" && !strings.Contains(err.Error(), tc.wantHint) {
					t.Errorf("err %q does not contain %q", err.Error(), tc.wantHint)
				}
			} else if err != nil {
				t.Fatalf("want nil error, got: %v", err)
			}
		})
	}
}
