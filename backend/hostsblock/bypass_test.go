package hostsblock

import (
	"slices"
	"testing"
)

func TestParseServices(t *testing.T) {
	out := "An asterisk (*) denotes that a network service is disabled.\n" +
		"USB 10/100/1000 LAN\n" +
		"Wi-Fi\n" +
		"*Thunderbolt Bridge\n\n"
	got := parseServices(out)
	want := []string{"USB 10/100/1000 LAN", "Wi-Fi"}
	if !slices.Equal(got, want) {
		t.Errorf("parseServices = %v, want %v", got, want)
	}
}

func TestParseBypass(t *testing.T) {
	t.Run("常规表逐行", func(t *testing.T) {
		got := parseBypass("localhost\n127.*\n10.*\n")
		want := []string{"localhost", "127.*", "10.*"}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("空表提示语返回空", func(t *testing.T) {
		out := "There aren't any bypass domains set on Wi-Fi.\n"
		if got := parseBypass(out); len(got) != 0 {
			t.Errorf("got %v, want 空", got)
		}
	})
}

func TestTargetBypass(t *testing.T) {
	cur := []string{"localhost", "127.*"}
	sites := []string{"a.com", "www.a.com"}
	t.Run("拦截时追加名单且保序", func(t *testing.T) {
		got := targetBypass(cur, sites, true)
		want := []string{"localhost", "127.*", "a.com", "www.a.com"}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("拦截幂等: 已含名单则不变", func(t *testing.T) {
		full := []string{"localhost", "a.com", "127.*", "www.a.com"}
		if got := targetBypass(full, sites, true); !slices.Equal(got, full) {
			t.Errorf("got %v, want %v", got, full)
		}
	})
	t.Run("解除时移除名单", func(t *testing.T) {
		full := []string{"localhost", "a.com", "127.*", "www.a.com"}
		if got := targetBypass(full, sites, false); !slices.Equal(got, cur) {
			t.Errorf("got %v, want %v", got, cur)
		}
	})
	t.Run("解除幂等: 不含名单则不变", func(t *testing.T) {
		if got := targetBypass(cur, sites, false); !slices.Equal(got, cur) {
			t.Errorf("got %v, want %v", got, cur)
		}
	})
}
