package curfew

import (
	"context"
	"testing"
	"time"
)

var loc = time.FixedZone("Asia/Shanghai", 8*3600)

// at 2026-07-13 当日的 hour:min (本地固定时区, 不依赖跑测试机器的 TZ)。
func at(hour, min int) time.Time {
	return time.Date(2026, 7, 13, hour, min, 0, 0, loc)
}

func TestInWindow(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"21:59 窗口前", at(21, 59), false},
		{"22:00 整点进窗口", at(22, 0), true},
		{"23:30 夜间", at(23, 30), true},
		{"00:00 跨日", at(0, 0), true},
		{"03:00 凌晨", at(3, 0), true},
		{"05:59 临界仍在窗口", at(5, 59), true},
		{"06:00 整点出窗口", at(6, 0), false},
		{"12:00 白天", at(12, 0), false},
		{"18:00 傍晚", at(18, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InWindow(c.t); got != c.want {
				t.Errorf("InWindow(%s) = %v, want %v", c.t.Format("15:04"), got, c.want)
			}
		})
	}
}

func TestNext(t *testing.T) {
	t.Run("窗口外未上膛 = 无事", func(t *testing.T) {
		st, act := Next(State{}, at(12, 0))
		if act != ActionNone || st.armed() {
			t.Errorf("got (%+v, %d), want 零状态 + ActionNone", st, act)
		}
	})
	t.Run("进窗口上膛, deadline = now+Grace", func(t *testing.T) {
		st, act := Next(State{}, at(22, 0))
		if act != ActionArm {
			t.Fatalf("act = %d, want ActionArm", act)
		}
		if want := at(22, 5); !st.Deadline.Equal(want) {
			t.Errorf("Deadline = %s, want %s", st.Deadline.Format("15:04"), want.Format("15:04"))
		}
	})
	t.Run("倒计时未到点 = 保持", func(t *testing.T) {
		armed := State{Deadline: at(22, 5)}
		st, act := Next(armed, at(22, 3))
		if act != ActionNone || !st.Deadline.Equal(armed.Deadline) {
			t.Errorf("got (%+v, %d), want 状态不变 + ActionNone", st, act)
		}
	})
	t.Run("到点关机, 状态清零", func(t *testing.T) {
		st, act := Next(State{Deadline: at(22, 5)}, at(22, 5))
		if act != ActionShutdown || st.armed() {
			t.Errorf("got (%+v, %d), want 零状态 + ActionShutdown", st, act)
		}
	})
	t.Run("超点 (如睡眠错过 tick) 也关机", func(t *testing.T) {
		_, act := Next(State{Deadline: at(22, 5)}, at(23, 30))
		if act != ActionShutdown {
			t.Errorf("act = %d, want ActionShutdown", act)
		}
	})
	t.Run("倒计时中越过 06:00 = 解除不关机", func(t *testing.T) {
		armed := State{Deadline: at(6, 3)} // 05:58 上膛
		st, act := Next(armed, at(6, 0))
		if act != ActionDisarm || st.armed() {
			t.Errorf("got (%+v, %d), want 零状态 + ActionDisarm", st, act)
		}
	})
	t.Run("关机失败后下个 tick 重新上膛", func(t *testing.T) {
		st, act := Next(State{}, at(22, 6)) // 22:05 关机失败清零后
		if act != ActionArm || !st.Deadline.Equal(at(22, 11)) {
			t.Errorf("got (%+v, %d), want 重新上膛 deadline 22:11", st, act)
		}
	})
}

// TestMinuteTickSequence 模拟 21:58 起逐分钟 tick 的完整推进。
func TestMinuteTickSequence(t *testing.T) {
	var st State
	var got []Action
	for min := 58; min <= 67; min++ { // 21:58 .. 22:07
		var act Action
		st, act = Next(st, at(21, min))
		got = append(got, act)
	}
	want := []Action{
		ActionNone, ActionNone, // 21:58 21:59
		ActionArm,                                      // 22:00
		ActionNone, ActionNone, ActionNone, ActionNone, // 22:01-04
		ActionShutdown, // 22:05
		ActionArm,      // 22:06 (关机失败场景的重试上膛)
		ActionNone,     // 22:07
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tick %d: act = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestCrossMidnightSequence 23:57 上膛, deadline 落到次日 00:02。
func TestCrossMidnightSequence(t *testing.T) {
	st, act := Next(State{}, at(23, 57))
	if act != ActionArm {
		t.Fatalf("23:57 act = %d, want ActionArm", act)
	}
	for _, tt := range []time.Time{
		at(23, 58), at(23, 59),
		time.Date(2026, 7, 14, 0, 0, 0, 0, loc),
		time.Date(2026, 7, 14, 0, 1, 0, 0, loc),
	} {
		if st, act = Next(st, tt); act != ActionNone {
			t.Fatalf("%s act = %d, want ActionNone", tt.Format("15:04"), act)
		}
	}
	if _, act = Next(st, time.Date(2026, 7, 14, 0, 2, 0, 0, loc)); act != ActionShutdown {
		t.Errorf("00:02 act = %d, want ActionShutdown", act)
	}
}

// drive 启动 run 循环, 逐个送 tick 后取消, 返回 shutdown 被调用次数。
func drive(t *testing.T, start time.Time, ticks []time.Time) int {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tickCh := make(chan time.Time)
	calls := 0
	done := make(chan struct{})
	go func() {
		run(ctx, func() time.Time { return start },
			tickCh, func(context.Context) error { calls++; return nil })
		close(done)
	}()
	for _, tt := range ticks {
		tickCh <- tt
	}
	cancel()
	<-done
	return calls
}

func TestRunLoop(t *testing.T) {
	t.Run("白天启动, 22:05 到点关机一次", func(t *testing.T) {
		got := drive(t, at(21, 58), []time.Time{at(22, 0), at(22, 3), at(22, 5)})
		if got != 1 {
			t.Errorf("shutdown 调用 %d 次, want 1", got)
		}
	})
	t.Run("窗口内启动 (关机后重新开机), 首查即上膛", func(t *testing.T) {
		got := drive(t, at(23, 0), []time.Time{at(23, 4), at(23, 5)})
		if got != 1 {
			t.Errorf("shutdown 调用 %d 次, want 1", got)
		}
	})
	t.Run("上膛后越过 06:00 不关机", func(t *testing.T) {
		got := drive(t, at(5, 58), []time.Time{at(5, 59), at(6, 0), at(6, 3)})
		if got != 0 {
			t.Errorf("shutdown 调用 %d 次, want 0", got)
		}
	})
}
