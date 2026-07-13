package curfew

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"panel/exec"
)

// cmdTimeout 单条关机命令的执行上限。命令生效时进程随系统一起终止, 超时只兜住卡死
// (如 TCC 授权弹窗挂起 osascript)。
const cmdTimeout = 15 * time.Second

// shutdownCmds 关机命令链, 按序尝试, 第一条 exit 0 即成功:
//
//  1. sudo shutdown — 真·强制关机, 无任何弹窗。需一次性 sudoers 授权
//     (/etc/sudoers.d/panel-shutdown: "kinesis ALL=(root) NOPASSWD: /sbin/shutdown");
//     -n = 不可交互, 未授权直接 exit 1 落到下一条。
//  2. loginwindow AppleEvent — 无确认框关机, app 逐个退出。daemon 首次触发会弹
//     一次 TCC「控制 loginwindow」授权, 点允许后不再弹。
//  3. pmset sleepnow — 兜底立即睡眠 (非关机, 但同样切断视频), 无需任何授权。
var shutdownCmds = []string{
	"sudo -n /sbin/shutdown -h now",
	`osascript -e 'tell app "loginwindow" to «event aevtrsdn»'`,
	"pmset sleepnow",
}

// runFn 执行一条 shell 命令; 生产用 exec.Run, 测试注入假实现。
type runFn func(ctx context.Context, cmd string, timeout time.Duration,
	workDir string) (exec.Result, error)

// Shutdown 依 shutdownCmds 链尝试关机, 全部失败时返回汇总错误。
func Shutdown(ctx context.Context) error {
	return shutdownWith(ctx, exec.Run)
}

func shutdownWith(ctx context.Context, run runFn) error {
	var fails []string
	for _, cmd := range shutdownCmds {
		res, err := run(ctx, cmd, cmdTimeout, "")
		if err != nil {
			fails = append(fails, fmt.Sprintf("%s: %v", cmd, err))
			continue
		}
		if res.ExitCode == 0 {
			log.Printf("curfew: 关机命令已生效: %s", cmd)
			return nil
		}
		fails = append(fails, fmt.Sprintf("%s: exit=%d stderr=%s",
			cmd, res.ExitCode, strings.TrimSpace(res.Stderr)))
	}
	return fmt.Errorf("关机命令链全部失败: %s", strings.Join(fails, " | "))
}
