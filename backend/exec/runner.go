// Package exec 在 panel 后端进程内执行 zsh 命令并捕获结果。
//
// 安全说明: 后端只监听 127.0.0.1:8089,无认证。本机不可信用户能进 8089
// 就能执行 任意 命令 -- 与 单用户单机 panel 的设计前提一致。
package exec

import (
	"bytes"
	"context"
	"errors"
	osexec "os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultTimeout 单条命令最长执行 60s。超时返回 exit_code=-1。
	DefaultTimeout = 60 * time.Second
	MaxOutputBytes = 1 << 20 // 1MB,各 stdout/stderr 截断
)

// Result 一次执行的全量结果。
type Result struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
}

// Run 用 `zsh -i -c <cmd>` 执行,捕获 stdout/stderr。
// -i 触发 source ~/.zshrc,继承 PATH/alias。
// 工作目录: home (避免被 panel 自身 cwd 污染)。
//
// 注: zsh 在 -i 下会向 stderr 写少量启动 banner / "no job control" 警告 -- 不过滤,原样返回。
func Run(ctx context.Context, cmd string, timeout time.Duration, workDir string) (Result, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Result{}, errors.New("empty cmd")
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := osexec.CommandContext(cctx, "zsh", "-i", "-c", cmd)
	c.Dir = workDir
	// SIGKILL 对子进程,以免它创建的子孙进程拖死
	c.Cancel = func() error {
		if c.Process != nil {
			return c.Process.Signal(syscall.SIGTERM)
		}
		return nil
	}

	var stdout, stderr cappedBuf
	c.Stdout = &stdout
	c.Stderr = &stderr

	start := time.Now()
	runErr := c.Run()
	dur := time.Since(start)

	res := Result{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: dur.Milliseconds(),
	}

	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.ExitCode = -1
		if !strings.Contains(res.Stderr, "[panel: timed out") {
			res.Stderr += "\n[panel: timed out after " + timeout.String() + "]"
		}
		return res, nil
	}

	if runErr != nil {
		var exitErr *osexec.ExitError
		if errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
			res.Stderr += "\n[panel: exec error: " + runErr.Error() + "]"
		}
	}
	return res, nil
}

// cappedBuf 防止单条命令输出爆内存:超过 MaxOutputBytes 后追加被丢弃 + 标记。
type cappedBuf struct {
	buf      bytes.Buffer
	overflow bool
}

func (c *cappedBuf) Write(p []byte) (int, error) {
	if c.buf.Len() >= MaxOutputBytes {
		c.overflow = true
		return len(p), nil
	}
	room := MaxOutputBytes - c.buf.Len()
	if len(p) <= room {
		return c.buf.Write(p)
	}
	_, _ = c.buf.Write(p[:room])
	c.overflow = true
	return len(p), nil
}

func (c *cappedBuf) String() string {
	s := c.buf.String()
	if c.overflow {
		s += "\n[panel: output truncated at 1MB]"
	}
	return s
}
