// Package curfew 夜间宵禁: 22:00–06:00 窗口内启动 5 分钟倒计时, 到点静默强制关机。
//
// 目标: 晚十点后阻断刷视频。后端 daemon 常驻 (launchd respawn), 关机后若重新
// 开机, daemon 起来首查又上膛 → 5 分钟后再关 → 窗口内机器每轮只有约 5 分钟可用。
//
// 设计:
//   - 单 goroutine 每 60s 轮询本地时间, 无锁; 状态迁移 Next 是纯函数。
//   - 窗口内首个 tick 上膛 (deadline = now + Grace); 倒计时中越过 06:00
//     (或时钟被改出窗口) 则解除, 不关机。
//   - 进程睡眠错过 tick: 醒来首个 tick 已过 deadline 且仍在窗口 → 立即关机。
//   - 关机失败 (命令链全挂, 见 shutdown.go): 状态清零, 下个 tick 重新上膛,
//     即约每 6 分钟重试一次。
package curfew

import (
	"context"
	"log"
	"time"
)

const (
	// StartHour / EndHour 宵禁窗口 [22:00, 06:00), 本地时间。
	// 06:00 与 dateutil.LogicalToday 的逻辑日切点一致。
	StartHour = 22
	EndHour   = 6
	// Grace 上膛到关机的倒计时。
	Grace = 5 * time.Minute
	// Poll 轮询间隔。
	Poll = 60 * time.Second
)

// InWindow 报告 t (本地时间) 是否在宵禁窗口内。
func InWindow(t time.Time) bool {
	h := t.Hour()
	return h >= StartHour || h < EndHour
}

// State 状态机的不可变状态, 零值 = 未上膛。Next 返回新副本, 不原地修改。
type State struct {
	// Deadline 非零表示已上膛, 到点执行关机。
	Deadline time.Time
}

func (s State) armed() bool { return !s.Deadline.IsZero() }

// Action 一次状态迁移要求调用方执行的副作用。
type Action int

const (
	ActionNone     Action = iota // 无事发生
	ActionArm                    // 进入窗口, 开始倒计时
	ActionDisarm                 // 倒计时中离开窗口, 解除
	ActionShutdown               // 倒计时到点, 执行关机
)

// Next 纯函数状态机: 给定当前状态与当前时间, 返回新状态与动作。
func Next(s State, t time.Time) (State, Action) {
	if !InWindow(t) {
		if s.armed() {
			return State{}, ActionDisarm
		}
		return State{}, ActionNone
	}
	if !s.armed() {
		return State{Deadline: t.Add(Grace)}, ActionArm
	}
	if t.Before(s.Deadline) {
		return s, ActionNone
	}
	return State{}, ActionShutdown
}

// Run 阻塞轮询直到 ctx 取消; shutdown 在倒计时到点时执行。
func Run(ctx context.Context, shutdown func(context.Context) error) {
	tick := time.NewTicker(Poll)
	defer tick.Stop()
	run(ctx, time.Now, tick.C, shutdown)
}

// run 时钟与 tick 源可注入, 供测试驱动。启动时立即做一次检查 (不等首个 tick),
// 保证夜间重新开机后 daemon 一起来就上膛。
func run(ctx context.Context, now func() time.Time, ticks <-chan time.Time,
	shutdown func(context.Context) error) {
	log.Printf("curfew: 宵禁生效 %02d:00–%02d:00, 倒计时 %s", StartHour, EndHour, Grace)
	var st State
	step := func(t time.Time) {
		var act Action
		st, act = Next(st, t)
		switch act {
		case ActionArm:
			log.Printf("curfew: 进入宵禁窗口, %s 后关机 (%s)",
				Grace, st.Deadline.Format("15:04"))
		case ActionDisarm:
			log.Printf("curfew: 已离开宵禁窗口, 取消关机")
		case ActionShutdown:
			log.Printf("curfew: 倒计时到点, 执行关机")
			if err := shutdown(ctx); err != nil {
				log.Printf("curfew: 关机失败, 窗口内下一轮重试: %v", err)
			}
		}
	}
	step(now())
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticks:
			step(t)
		}
	}
}
