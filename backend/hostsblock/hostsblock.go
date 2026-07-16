// Package hostsblock 早间防刷: 后端启动后的一段窗口 (默认 2h) 内, 把名单里的
// 娱乐网站在 DNS 层截断 (/etc/hosts 标记块指到 0.0.0.0/::), 到点自动恢复。
//
// 触发只看「进程启动」不看钟点: 早上无论几点开机, daemon 起来即拦 2 小时。
// rebuild 重启后端也会重新计 2 小时 (副作用, 拦的是娱乐站, 不影响干活)。
//
// 设计:
//   - 名单: dataDir/blocked-sites.txt, 一行一个域名, 用户直接编辑, 重启生效;
//     不存在时自动写默认模板。
//   - 写 /etc/hosts 需要 root → 经 sudoers 授权的专用脚本
//     /usr/local/bin/panel-hostsblock (见 backend/scripts/), Go 侧只读 hosts
//     判断幂等, 需要变更才 sudo 调脚本 (避免每分钟无谓 flush DNS)。
//   - 每 60s 收敛一次: 窗口内块被手动删掉会自动修回; 窗口结束 unblock,
//     失败则每 tick 重试直到成功才退出 (sudoers 没配好时错误去重打日志)。
//   - 窗口计时用 now-start (monotonic, darwin 含睡眠时间), 改系统时钟不影响。
package hostsblock

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"panel/exec"
)

const (
	// DefaultWindow 启动后的拦截时长; 可用环境变量 PANEL_HOSTSBLOCK_WINDOW
	// 覆盖 (time.ParseDuration 格式, 如 "3m", 便于验证)。
	DefaultWindow = 2 * time.Hour
	WindowEnv     = "PANEL_HOSTSBLOCK_WINDOW"
	// Poll 收敛间隔。
	Poll = 60 * time.Second
	// cmdTimeout 单次脚本调用上限。
	cmdTimeout = 15 * time.Second

	hostsPath  = "/etc/hosts"
	scriptPath = "/usr/local/bin/panel-hostsblock"
	sitesFile  = "blocked-sites.txt"
)

// runFn 执行一条 shell 命令; 生产用 exec.Run, 测试注入假实现。
type runFn func(ctx context.Context, cmd string, timeout time.Duration,
	workDir string) (exec.Result, error)

// deps run 循环的可注入依赖。
type deps struct {
	sites  []string
	window time.Duration
	read   func() (string, error) // 读 hosts 全文
	exec   runFn
	now    func() time.Time
}

// Wanted 报告启动于 start 的进程在 now 时刻是否应处于拦截状态。
func Wanted(start, now time.Time, window time.Duration) bool {
	return now.Sub(start) < window
}

// EffectiveWindow 解析环境变量里的窗口时长, 未设/非法时用默认值。
func EffectiveWindow(raw string) time.Duration {
	if raw == "" {
		return DefaultWindow
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("hostsblock: %s=%q 无效, 用默认 %s", WindowEnv, raw, DefaultWindow)
		return DefaultWindow
	}
	return d
}

// Unblock 立即解除拦截 (幂等, 无块时不调 sudo), 供进程退出时清理残留块。
// 清理失败无妨: 下次启动会重新接管标记块并在窗口结束时解除。
func Unblock(ctx context.Context) error {
	d := deps{
		read: func() (string, error) {
			b, err := os.ReadFile(hostsPath)
			return string(b), err
		},
		exec: exec.Run,
	}
	return ensure(ctx, d, false)
}

// Run 阻塞: 加载名单, 启动即拦截, 窗口结束解除成功后返回。
func Run(ctx context.Context, dataDir string) {
	sites, skipped, err := LoadSites(filepath.Join(dataDir, sitesFile))
	if err != nil {
		log.Printf("hostsblock: 名单加载失败, 本次不拦截: %v", err)
		return
	}
	if len(skipped) > 0 {
		log.Printf("hostsblock: 跳过非法名单行: %q", skipped)
	}
	if len(sites) == 0 {
		log.Printf("hostsblock: 名单为空, 本次不拦截")
		return
	}
	d := deps{
		sites:  sites,
		window: EffectiveWindow(os.Getenv(WindowEnv)),
		read: func() (string, error) {
			b, err := os.ReadFile(hostsPath)
			return string(b), err
		},
		exec: exec.Run,
		now:  time.Now,
	}
	tick := time.NewTicker(Poll)
	defer tick.Stop()
	run(ctx, d, tick.C)
}

// run 每 tick 把 hosts 收敛到期望状态; 窗口结束且解除成功后返回。
func run(ctx context.Context, d deps, ticks <-chan time.Time) {
	start := d.now()
	log.Printf("hostsblock: 拦截 %d 个域名至 %s: %s",
		len(d.sites), start.Add(d.window).Format("15:04"), strings.Join(d.sites, " "))
	lastErr := ""
	step := func(now time.Time) (done bool) {
		wanted := Wanted(start, now, d.window)
		err := ensure(ctx, d, wanted)
		switch {
		case err == nil:
			lastErr = ""
		case err.Error() != lastErr: // 同样的错误只打一次, 不刷日志
			lastErr = err.Error()
			log.Printf("hostsblock: %v", err)
		}
		return !wanted && err == nil
	}
	if step(start) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticks:
			if step(t) {
				log.Printf("hostsblock: 窗口结束, 拦截已解除")
				return
			}
		}
	}
}

// ensure 幂等收敛: 已达标不动, 需要变更才 sudo 调脚本。
func ensure(ctx context.Context, d deps, wantBlocked bool) error {
	content, err := d.read()
	if err != nil {
		return fmt.Errorf("读 %s: %w", hostsPath, err)
	}
	if wantBlocked {
		if UpToDate(content, d.sites) {
			return nil
		}
		return runScript(ctx, d.exec, append([]string{"block"}, d.sites...))
	}
	if CurrentBlock(content) == nil {
		return nil
	}
	return runScript(ctx, d.exec, []string{"unblock"})
}

func runScript(ctx context.Context, run runFn, args []string) error {
	cmd := "sudo -n " + scriptPath + " " + strings.Join(args, " ")
	res, err := run(ctx, cmd, cmdTimeout, "")
	if err != nil {
		return fmt.Errorf("%s: %w", cmd, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: exit=%d stderr=%s",
			cmd, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	log.Printf("hostsblock: %s 完成", args[0])
	return nil
}
