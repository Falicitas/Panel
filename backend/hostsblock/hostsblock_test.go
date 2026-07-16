package hostsblock

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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

// world 假环境: hosts/bypass/命令记录的所有变更都发生在 run goroutine 的
// read/exec 调用内, 主 goroutine 只在 <-done 之后读, 无数据竞争。
// 模拟单个启用服务 Wi-Fi (带空格服务名的参数正确性由脚本测试覆盖)。
type world struct {
	hosts        string
	bypass       []string // 服务 Wi-Fi 的代理例外表
	cmds         []string
	failUnblocks int          // 前 N 次 hosts unblock 返回 exit 1
	wipeAt       map[int]bool // 第 N 次 read (1-based) 前重置 hosts (模拟手滑删块)
	reads        int
}

func (w *world) read() (string, error) {
	w.reads++
	if w.wipeAt[w.reads] {
		w.hosts = plainHosts
	}
	return w.hosts, nil
}

// unq 剥掉 runScript 给参数加的单引号。
func unq(fields []string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = strings.Trim(f, "'")
	}
	return out
}

func (w *world) exec(_ context.Context, cmd string, _ time.Duration, _ string) (exec.Result, error) {
	w.cmds = append(w.cmds, cmd)
	switch {
	case strings.HasPrefix(cmd, "/usr/sbin/networksetup -listallnetworkservices"):
		out := "An asterisk (*) denotes that a network service is disabled.\n" +
			"Wi-Fi\n*Thunderbolt Bridge\n"
		return exec.Result{Stdout: out}, nil
	case strings.HasPrefix(cmd, "/usr/sbin/networksetup -getproxybypassdomains"):
		if len(w.bypass) == 0 {
			return exec.Result{Stdout: "There aren't any bypass domains set on Wi-Fi.\n"}, nil
		}
		return exec.Result{Stdout: strings.Join(w.bypass, "\n") + "\n"}, nil
	}
	args := unq(strings.Fields(cmd)) // sudo -n <script> <subcmd> <args...>
	switch args[3] {
	case "block":
		w.hosts = blocked(args[4:]...)
	case "unblock":
		if w.failUnblocks > 0 {
			w.failUnblocks--
			return exec.Result{ExitCode: 1, Stderr: "sudo: a password is required"}, nil
		}
		w.hosts = plainHosts
	case "setbypass":
		if len(args) == 6 && args[5] == "Empty" {
			w.bypass = nil
		} else {
			w.bypass = args[5:] // args[4] = 服务名 Wi-Fi
		}
	}
	return exec.Result{ExitCode: 0}, nil
}

// sudoCalls 过滤出经 sudo 脚本的写操作 (跳过 networksetup 只读命令)。
func sudoCalls(cmds []string) []string {
	var out []string
	for _, c := range cmds {
		if strings.HasPrefix(c, "sudo -n ") {
			out = append(out, c)
		}
	}
	return out
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

// userBypass 模拟用户已有的代理例外表 (Clash 常见默认 + macOS <local>
// 记号, 实测 Thunderbolt Bridge 自带), 全程必须原样保留。
func userBypass() []string {
	return []string{"localhost", "127.*", "192.168.1.6", "10.*", "<local>"}
}

func TestRunLoop(t *testing.T) {
	t0 := time.Date(2026, 7, 16, 8, 0, 0, 0, time.Local)
	t.Run("启动即拦 hosts+bypass, 窗口内幂等, 到点双双还原", func(t *testing.T) {
		w := &world{hosts: plainHosts, bypass: userBypass()}
		driveRun(t, w, 2*time.Minute, t0,
			[]time.Time{t0.Add(1 * time.Minute), t0.Add(3 * time.Minute)})
		sudo := sudoCalls(w.cmds)
		if len(sudo) != 4 {
			t.Fatalf("sudo 调用 = %v, want block/setbypass/unblock/setbypass 4 条", sudo)
		}
		if !strings.Contains(sudo[0], " 'block' 'a.com' 'www.a.com'") ||
			!strings.HasPrefix(sudo[0], "sudo -n /usr/local/bin/panel-hostsblock") {
			t.Errorf("sudo[0] = %q", sudo[0])
		}
		if !strings.Contains(sudo[1], " 'setbypass' 'Wi-Fi'") ||
			!strings.Contains(sudo[1], "'a.com'") ||
			!strings.Contains(sudo[1], "'localhost'") {
			t.Errorf("sudo[1] = %q, want 追加名单且保留原表", sudo[1])
		}
		if !strings.Contains(sudo[2], " 'unblock'") {
			t.Errorf("sudo[2] = %q", sudo[2])
		}
		if !strings.Contains(sudo[3], " 'setbypass' 'Wi-Fi'") ||
			strings.Contains(sudo[3], "a.com") {
			t.Errorf("sudo[3] = %q, want 移除名单", sudo[3])
		}
		if w.hosts != plainHosts {
			t.Errorf("最终 hosts 未恢复: %q", w.hosts)
		}
		if !slices.Equal(w.bypass, userBypass()) {
			t.Errorf("最终 bypass = %v, want 还原 %v", w.bypass, userBypass())
		}
	})
	t.Run("用户例外表为空时, 解除用 Empty 清表", func(t *testing.T) {
		w := &world{hosts: plainHosts}
		driveRun(t, w, 2*time.Minute, t0, []time.Time{t0.Add(3 * time.Minute)})
		sudo := sudoCalls(w.cmds)
		if len(sudo) != 4 || !strings.Contains(sudo[3], "'Empty'") {
			t.Fatalf("sudo = %v, want 末条 setbypass Empty", sudo)
		}
		if len(w.bypass) != 0 {
			t.Errorf("最终 bypass = %v, want 空", w.bypass)
		}
	})
	t.Run("窗口内块被手删, 下个 tick 自动修复", func(t *testing.T) {
		w := &world{hosts: plainHosts, bypass: userBypass(),
			wipeAt: map[int]bool{2: true}}
		driveRun(t, w, 5*time.Minute, t0, []time.Time{
			t0.Add(1 * time.Minute), // read#2 前被 wipe → 重新 block
			t0.Add(2 * time.Minute), // 稳定, 无动作
			t0.Add(6 * time.Minute), // 到点解除
		})
		sudo := sudoCalls(w.cmds)
		if len(sudo) != 5 {
			t.Fatalf("sudo = %v, want 5 条 (含修复性 block)", sudo)
		}
		if !strings.Contains(sudo[2], " 'block' ") {
			t.Errorf("sudo[2] = %q, want 修复性 block", sudo[2])
		}
	})
	t.Run("解除失败下个 tick 重试直到成功", func(t *testing.T) {
		w := &world{hosts: plainHosts, bypass: userBypass(), failUnblocks: 1}
		driveRun(t, w, 1*time.Minute, t0, []time.Time{
			t0.Add(2 * time.Minute), // unblock 失败; bypass 移除成功
			t0.Add(3 * time.Minute), // unblock 重试成功 → 退出
		})
		sudo := sudoCalls(w.cmds)
		if len(sudo) != 5 {
			t.Fatalf("sudo = %v, want 5 条 (unblock 失败重试)", sudo)
		}
		if w.hosts != plainHosts || !slices.Equal(w.bypass, userBypass()) {
			t.Errorf("最终未双双还原: hosts=%q bypass=%v", w.hosts, w.bypass)
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
