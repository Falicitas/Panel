package hostsblock

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"panel/exec"
)

func TestWanted(t *testing.T) {
	start := time.Date(2026, 7, 16, 8, 0, 0, 0, time.Local)
	w := 2 * time.Hour
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"启动瞬间", start, true},
		{"窗口内", start.Add(119 * time.Minute), true},
		{"整点到期", start.Add(2 * time.Hour), false},
		{"窗口后", start.Add(3 * time.Hour), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Wanted(start, c.now, w); got != c.want {
				t.Errorf("Wanted = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEffectiveWindow(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", DefaultWindow},
		{"3m", 3 * time.Minute},
		{"90m", 90 * time.Minute},
		{"abc", DefaultWindow},
		{"-5m", DefaultWindow},
		{"0", DefaultWindow},
	}
	for _, c := range cases {
		if got := EffectiveWindow(c.raw); got != c.want {
			t.Errorf("EffectiveWindow(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// world 假环境: hosts 内容/命令记录的所有变更都发生在 run goroutine 的
// read/exec 调用内, 主 goroutine 只在 <-done 之后读, 无数据竞争。
type world struct {
	hosts  string
	cmds   []string
	fail   map[int]bool // 第 N 次 exec (1-based) 返回 exit 1
	wipeAt map[int]bool // 第 N 次 read (1-based) 前重置 hosts (模拟手滑删块)
	reads  int
}

func (w *world) read() (string, error) {
	w.reads++
	if w.wipeAt[w.reads] {
		w.hosts = plainHosts
	}
	return w.hosts, nil
}

func (w *world) exec(_ context.Context, cmd string, _ time.Duration, _ string) (exec.Result, error) {
	w.cmds = append(w.cmds, cmd)
	if w.fail[len(w.cmds)] {
		return exec.Result{ExitCode: 1, Stderr: "sudo: a password is required"}, nil
	}
	fields := strings.Fields(cmd)
	switch fields[3] {
	case "block":
		w.hosts = blocked(fields[4:]...)
	case "unblock":
		w.hosts = plainHosts
	}
	return exec.Result{ExitCode: 0}, nil
}

// driveRun 启动 run 循环, 依次送 tick, 等 run 自行结束 (窗口结束且解除成功)。
func driveRun(t *testing.T, w *world, window time.Duration, start time.Time,
	ticks []time.Time) {
	t.Helper()
	d := deps{
		sites:  []string{"a.com", "www.a.com"},
		window: window,
		read:   w.read,
		exec:   w.exec,
		now:    func() time.Time { return start },
	}
	tickCh := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		run(context.Background(), d, tickCh)
		close(done)
	}()
	for _, tt := range ticks {
		tickCh <- tt
	}
	<-done
}

func TestRunLoop(t *testing.T) {
	t0 := time.Date(2026, 7, 16, 8, 0, 0, 0, time.Local)
	t.Run("启动即拦, 窗口内幂等, 到点解除后退出", func(t *testing.T) {
		w := &world{hosts: plainHosts}
		driveRun(t, w, 2*time.Minute, t0,
			[]time.Time{t0.Add(1 * time.Minute), t0.Add(3 * time.Minute)})
		if len(w.cmds) != 2 {
			t.Fatalf("cmds = %v, want block + unblock 共 2 条", w.cmds)
		}
		if !strings.Contains(w.cmds[0], " block a.com www.a.com") ||
			!strings.HasPrefix(w.cmds[0], "sudo -n /usr/local/bin/panel-hostsblock") {
			t.Errorf("cmds[0] = %q", w.cmds[0])
		}
		if !strings.HasSuffix(w.cmds[1], " unblock") {
			t.Errorf("cmds[1] = %q", w.cmds[1])
		}
		if w.hosts != plainHosts {
			t.Errorf("最终 hosts 未恢复: %q", w.hosts)
		}
	})
	t.Run("窗口内块被手删, 下个 tick 自动修复", func(t *testing.T) {
		w := &world{hosts: plainHosts, wipeAt: map[int]bool{2: true}}
		driveRun(t, w, 5*time.Minute, t0, []time.Time{
			t0.Add(1 * time.Minute), // read#2 前被 wipe → 重新 block
			t0.Add(2 * time.Minute), // 稳定, 无动作
			t0.Add(6 * time.Minute), // 到点 unblock
		})
		if len(w.cmds) != 3 {
			t.Fatalf("cmds = %v, want block/block/unblock 共 3 条", w.cmds)
		}
		if !strings.Contains(w.cmds[1], " block ") {
			t.Errorf("cmds[1] = %q, want 修复性 block", w.cmds[1])
		}
	})
	t.Run("解除失败下个 tick 重试直到成功", func(t *testing.T) {
		w := &world{hosts: plainHosts, fail: map[int]bool{2: true}}
		driveRun(t, w, 1*time.Minute, t0, []time.Time{
			t0.Add(2 * time.Minute), // unblock 失败 (exit 1)
			t0.Add(3 * time.Minute), // 重试成功 → 退出
		})
		if len(w.cmds) != 3 {
			t.Fatalf("cmds = %v, want block/unblock失败/unblock 共 3 条", w.cmds)
		}
		if w.hosts != plainHosts {
			t.Errorf("最终 hosts 未恢复: %q", w.hosts)
		}
	})
	t.Run("名单异常时 Run 直接返回不进轮询", func(t *testing.T) {
		// 这些路径都在进 loop 之前 return, 不会碰真 exec/sudo
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "blocked-sites.txt"),
			[]byte("# 全是注释\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		Run(context.Background(), dir) // 空名单
		if err := os.WriteFile(filepath.Join(dir, "blocked-sites.txt"),
			[]byte("bad;line\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		Run(context.Background(), dir) // 全非法行 → skipped + 空名单
		Run(context.Background(), filepath.Join(dir, "不存在的子目录"))
		// 不挂起、不 panic 即通过
	})
	t.Run("ctx 取消时直接退出", func(t *testing.T) {
		w := &world{hosts: plainHosts}
		ctx, cancel := context.WithCancel(context.Background())
		d := deps{
			sites:  []string{"a.com"},
			window: time.Hour,
			read:   w.read,
			exec:   w.exec,
			now:    func() time.Time { return t0 },
		}
		done := make(chan struct{})
		go func() {
			run(ctx, d, make(chan time.Time))
			close(done)
		}()
		cancel()
		<-done // 不挂起即通过
	})
}
