package hostsblock

import (
	"slices"
	"strings"
	"testing"
)

const plainHosts = "127.0.0.1 localhost\n::1 localhost\n"

// blocked 拼一份带标记块的 hosts 内容 (与脚本输出格式一致)。
func blocked(sites ...string) string {
	var b strings.Builder
	b.WriteString(plainHosts)
	b.WriteString(beginMark + "\n")
	for _, s := range sites {
		b.WriteString("0.0.0.0 " + s + "\n:: " + s + "\n")
	}
	b.WriteString(endMark + "\n")
	return b.String()
}

func TestBlockLines(t *testing.T) {
	got := BlockLines([]string{"a.com", "www.a.com"})
	want := []string{"0.0.0.0 a.com", ":: a.com", "0.0.0.0 www.a.com", ":: www.a.com"}
	if !slices.Equal(got, want) {
		t.Errorf("BlockLines = %v, want %v", got, want)
	}
}

func TestCurrentBlock(t *testing.T) {
	t.Run("无标记块返回 nil", func(t *testing.T) {
		if got := CurrentBlock(plainHosts); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("有标记块返回块内行", func(t *testing.T) {
		got := CurrentBlock(blocked("a.com"))
		want := []string{"0.0.0.0 a.com", ":: a.com"}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("空标记块返回非 nil 空 slice", func(t *testing.T) {
		got := CurrentBlock(plainHosts + beginMark + "\n" + endMark + "\n")
		if got == nil || len(got) != 0 {
			t.Errorf("got %v, want 非 nil 空", got)
		}
	})
}

func TestUpToDate(t *testing.T) {
	sites := []string{"a.com", "www.a.com"}
	t.Run("块与名单一致", func(t *testing.T) {
		if !UpToDate(blocked(sites...), sites) {
			t.Error("want true")
		}
	})
	t.Run("无块", func(t *testing.T) {
		if UpToDate(plainHosts, sites) {
			t.Error("want false")
		}
	})
	t.Run("名单变化 (块里是旧名单)", func(t *testing.T) {
		if UpToDate(blocked("b.com"), sites) {
			t.Error("want false")
		}
	})
}
