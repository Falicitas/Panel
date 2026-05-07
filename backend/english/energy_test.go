package english

import (
	"os"
	"path/filepath"
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

func TestValidateAudioSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("≤25MB 通过", func(t *testing.T) {
		small := filepath.Join(dir, "small.mp3")
		if err := writeSparseFile(small, 24*1024*1024); err != nil {
			t.Fatalf("write small: %v", err)
		}
		if err := ValidateAudioSize(small); err != nil {
			t.Errorf("24MB should pass, got: %v", err)
		}
	})
	t.Run("=25MB 边界通过", func(t *testing.T) {
		boundary := filepath.Join(dir, "boundary.mp3")
		if err := writeSparseFile(boundary, 25*1024*1024); err != nil {
			t.Fatalf("write boundary: %v", err)
		}
		if err := ValidateAudioSize(boundary); err != nil {
			t.Errorf("exactly 25MB should pass (>25 cap), got: %v", err)
		}
	})
	t.Run(">25MB 拒", func(t *testing.T) {
		big := filepath.Join(dir, "big.mp3")
		if err := writeSparseFile(big, 26*1024*1024); err != nil {
			t.Fatalf("write big: %v", err)
		}
		err := ValidateAudioSize(big)
		if err == nil {
			t.Fatalf("26MB should fail")
		}
		if !strings.Contains(err.Error(), "too large") {
			t.Errorf("err %q lacks 'too large'", err.Error())
		}
	})
	t.Run("33MB Sciam 实际场景拒 (regression)", func(t *testing.T) {
		// Sciam Science Quickly 24min @ 192kbps = 33MB, 必须被这个 cap 拒。
		sciam := filepath.Join(dir, "sciam.mp3")
		if err := writeSparseFile(sciam, 33*1024*1024); err != nil {
			t.Fatalf("write sciam: %v", err)
		}
		err := ValidateAudioSize(sciam)
		if err == nil {
			t.Fatal("33MB should fail (Sciam regression)")
		}
		if !strings.Contains(err.Error(), "33MB") {
			t.Errorf("err %q lacks size '33MB'", err.Error())
		}
	})
	t.Run("不存在文件 → stat error", func(t *testing.T) {
		err := ValidateAudioSize(filepath.Join(dir, "nope.mp3"))
		if err == nil {
			t.Fatal("missing file should error")
		}
	})
}

// writeSparseFile 用 Truncate 创建 sparse 文件 (内容全 0, 占用磁盘空间最小,
// 但 os.Stat 看到的 size 是 truncate 值, 满足 size cap 测试)。
func writeSparseFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Truncate(size)
}
