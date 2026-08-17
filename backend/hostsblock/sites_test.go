package hostsblock

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeSites(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "blocked-sites.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSites(t *testing.T) {
	t.Run("文件不存在时写默认模板并返回默认名单", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "blocked-sites.txt")
		sites, skipped, err := LoadSites(path)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("skipped = %v, want 空", skipped)
		}
		want := []string{
			"bilibili.com", "www.bilibili.com",
			"youtube.com", "www.youtube.com",
		}
		if !slices.Equal(sites, want) {
			t.Errorf("sites = %v, want %v", sites, want)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("模板文件未写入: %v", err)
		}
	})
	t.Run("注释与空行跳过, 大小写归一, 自动补 www 变体", func(t *testing.T) {
		path := writeSites(t, "# 名单\n\n  Weibo.com  \nwww.zhihu.com\n")
		sites, skipped, err := LoadSites(path)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(skipped) != 0 {
			t.Errorf("skipped = %v, want 空", skipped)
		}
		want := []string{"weibo.com", "www.weibo.com", "www.zhihu.com"}
		if !slices.Equal(sites, want) {
			t.Errorf("sites = %v, want %v", sites, want)
		}
	})
	t.Run("重复域名去重且保序", func(t *testing.T) {
		path := writeSites(t, "a.com\nwww.a.com\na.com\n")
		sites, _, err := LoadSites(path)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if want := []string{"a.com", "www.a.com"}; !slices.Equal(sites, want) {
			t.Errorf("sites = %v, want %v", sites, want)
		}
	})
	t.Run("非法行跳过并报告 (shell 注入字符/无点)", func(t *testing.T) {
		path := writeSites(t, "a.com\nbad;domain.com\nb.com$(x)\nlocalhost\nc d.com\n")
		sites, skipped, err := LoadSites(path)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if want := []string{"a.com", "www.a.com"}; !slices.Equal(sites, want) {
			t.Errorf("sites = %v, want %v", sites, want)
		}
		if len(skipped) != 4 {
			t.Errorf("skipped = %v, want 4 条", skipped)
		}
	})
}
