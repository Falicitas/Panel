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
//   - 系统代理 (Clash 等 127.0.0.1:7890) 会让浏览器绕过 hosts → 窗口内同步把
//     名单域名加进代理例外表逼其直连, 见 bypass.go。
//   - 每 60s 收敛一次: 窗口内块被手动删掉会自动修回; 窗口结束 unblock,
//     失败则每 tick 重试直到成功才退出 (sudoers 没配好时错误去重打日志)。
//   - 窗口计时用 now-start (monotonic, darwin 含睡眠时间), 改系统时钟不影响。
package hostsblock

import (
	"context"
	"errors"
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
	DefaultWindow = 3 * time.Hour
	WindowEnv     = "PANEL_HOSTSBLOCK_WINDOW"
	// Poll 收敛间隔。
	Poll = 60 * time.Second
	// cmdTimeout 单次命令上限。networksetup -set* 实测单次可达十秒级
	// (19:05:47→19:06:12 三个服务), 15s 偏紧, 放宽到 30s。
	cmdTimeout = 30 * time.Second

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
	svcs   *svcCache // 网络服务列表缓存, 见 bypass.go
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

// productionDeps 组装生产依赖 (真 hosts / 真 exec / 真时钟)。
func productionDeps(sites []string, window time.Duration) deps {
	return deps{
		sites:  sites,
		window: window,
		read: func() (string, error) {
			b, err := os.ReadFile(hostsPath)
			return string(b), err
		},
		exec: exec.Run,
		now:  time.Now,
		svcs: &svcCache{},
	}
}

// Unblock 立即解除拦截 (幂等), 供进程退出时清理残留 (hosts 块 + 代理例外)。
// 名单读不到时仍清 hosts 块 (例外表差集无从算, 残留交给下次启动窗口自愈)。
func Unblock(ctx context.Context, dataDir string) error {
	sites, _, err := LoadSites(filepath.Join(dataDir, sitesFile))
	if err != nil {
		sites = nil
	}
	return ensure(ctx, productionDeps(sites, 0), false)
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
	d := productionDeps(sites, EffectiveWindow(os.Getenv(WindowEnv)))
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

// ensure 幂等收敛 hosts 标记块与系统代理例外表, 已达标不动。
// 两条腿互不阻塞, 错误聚合返回 (代理例外失败不该拖住 hosts 收敛, 反之亦然)。
func ensure(ctx context.Context, d deps, wantBlocked bool) error {
	return errors.Join(
		ensureHosts(ctx, d, wantBlocked),
		ensureBypass(ctx, d, wantBlocked),
	)
}

func ensureHosts(ctx context.Context, d deps, wantBlocked bool) error {
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

// runScript 经 sudo 调专用 root 脚本。每个参数单引号包裹 -- 例外表条目常含
// "10.*" 这类 glob 字符, zsh 对无匹配 glob 会直接报错。
func runScript(ctx context.Context, run runFn, args []string) error {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + a + "'"
	}
	cmd := "sudo -n " + scriptPath + " " + strings.Join(quoted, " ")
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
