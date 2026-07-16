package hostsblock

// 系统代理 bypass 联动。
//
// Clash 等代理软件开启系统代理时 (如 127.0.0.1:7890), 浏览器把整个请求交给
// 代理进程, 域名解析发生在代理内部或远端 — /etc/hosts 拦不到 (issue-57 实测)。
// 对策: 拦截窗口内把名单域名并入系统代理例外表 (ExceptionsList), 逼这些域名
// 直连 → 直连解析命中 hosts 黑洞; 窗口结束移除, 恢复走代理 (如 youtube 需代理)。
//
// 列服务/读例外表不需要 root, 直接跑 networksetup; 写例外表经 sudo 脚本
// setbypass 子命令整表替换。目标表在 Go 侧计算, 用户已有条目原样保留。

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const networksetupBin = "/usr/sbin/networksetup"

// serviceRe 合法网络服务名 (含 "USB 10/100/1000 LAN" 这类带空格/斜杠的)。
// 兼做安全过滤: 服务名会拼进 shell 命令行, 不匹配的直接跳过。
var serviceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ./_-]*$`)

// parseServices 解析 networksetup -listallnetworkservices 输出:
// 跳过首行说明与 * 前缀的禁用服务。
func parseServices(out string) []string {
	var svcs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "asterisk") ||
			strings.HasPrefix(line, "*") || !serviceRe.MatchString(line) {
			continue
		}
		svcs = append(svcs, line)
	}
	return svcs
}

// parseBypass 解析 -getproxybypassdomains 输出; 空表提示语返回空。
func parseBypass(out string) []string {
	var entries []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "There aren't any") {
			continue
		}
		entries = append(entries, line)
	}
	return entries
}

// targetBypass 计算期望例外表: 拦截时并入 sites (保序追加), 解除时移除
// sites, 其余条目一律原样保留。对已达标输入返回等值表 (幂等)。
func targetBypass(cur, sites []string, blocked bool) []string {
	if blocked {
		out := slices.Clone(cur)
		for _, s := range sites {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return out
	}
	var out []string
	for _, e := range cur {
		if !slices.Contains(sites, e) {
			out = append(out, e)
		}
	}
	return out
}

// ensureBypass 对每个启用的网络服务把代理例外表收敛到期望状态。
func ensureBypass(ctx context.Context, d deps, wantBlocked bool) error {
	res, err := d.exec(ctx, networksetupBin+" -listallnetworkservices", cmdTimeout, "")
	if err != nil {
		return fmt.Errorf("列网络服务: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("列网络服务: exit=%d stderr=%s",
			res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	var errs []string
	for _, svc := range parseServices(res.Stdout) {
		if err := ensureServiceBypass(ctx, d, svc, wantBlocked); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("bypass: %s", strings.Join(errs, "; "))
	}
	return nil
}

func ensureServiceBypass(ctx context.Context, d deps, svc string, wantBlocked bool) error {
	cmd := fmt.Sprintf("%s -getproxybypassdomains '%s'", networksetupBin, svc)
	res, err := d.exec(ctx, cmd, cmdTimeout, "")
	if err != nil {
		return fmt.Errorf("%s: %w", svc, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: exit=%d stderr=%s",
			svc, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	cur := parseBypass(res.Stdout)
	want := targetBypass(cur, d.sites, wantBlocked)
	if slices.Equal(cur, want) {
		return nil
	}
	if len(want) == 0 {
		want = []string{"Empty"} // networksetup 清表关键字
	}
	return runScript(ctx, d.exec, append([]string{"setbypass", svc}, want...))
}
