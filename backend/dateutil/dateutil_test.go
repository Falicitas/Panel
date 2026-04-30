package dateutil

import (
	"testing"
	"time"
)

func TestLogicalToday(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	t.Run("06:00 整点 = 当日开始", func(t *testing.T) {
		now := time.Date(2026, 4, 30, 6, 0, 0, 0, loc)
		if got := LogicalToday(now); got != "2026-04-30" {
			t.Errorf("got %s, want 2026-04-30", got)
		}
	})
	t.Run("23:59 仍是当日", func(t *testing.T) {
		now := time.Date(2026, 4, 30, 23, 59, 59, 0, loc)
		if got := LogicalToday(now); got != "2026-04-30" {
			t.Errorf("got %s, want 2026-04-30", got)
		}
	})
	t.Run("次日 03:00 仍算前一天", func(t *testing.T) {
		now := time.Date(2026, 5, 1, 3, 0, 0, 0, loc)
		if got := LogicalToday(now); got != "2026-04-30" {
			t.Errorf("got %s, want 2026-04-30 (cutoff at 06:00)", got)
		}
	})
	t.Run("次日 05:59:59 临界 = 仍是前一天", func(t *testing.T) {
		now := time.Date(2026, 5, 1, 5, 59, 59, 0, loc)
		if got := LogicalToday(now); got != "2026-04-30" {
			t.Errorf("got %s, want 2026-04-30", got)
		}
	})
	t.Run("次日 06:00:00 = 切到新一天", func(t *testing.T) {
		now := time.Date(2026, 5, 1, 6, 0, 0, 0, loc)
		if got := LogicalToday(now); got != "2026-05-01" {
			t.Errorf("got %s, want 2026-05-01", got)
		}
	})
	t.Run("月初凌晨 02:00 跨月回退", func(t *testing.T) {
		now := time.Date(2026, 5, 1, 2, 0, 0, 0, loc)
		if got := LogicalToday(now); got != "2026-04-30" {
			t.Errorf("got %s, want 2026-04-30 (跨月回退)", got)
		}
	})
	t.Run("年初凌晨 04:00 跨年回退", func(t *testing.T) {
		now := time.Date(2027, 1, 1, 4, 0, 0, 0, loc)
		if got := LogicalToday(now); got != "2026-12-31" {
			t.Errorf("got %s, want 2026-12-31", got)
		}
	})
}
