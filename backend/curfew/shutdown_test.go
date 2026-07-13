package curfew

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"panel/exec"
)

// fakeRun 构造按序返回预设结果的 runFn, 并记录实际执行的命令。
func fakeRun(got *[]string, results []exec.Result, errs []error) runFn {
	i := 0
	return func(_ context.Context, cmd string, _ time.Duration, _ string) (exec.Result, error) {
		*got = append(*got, cmd)
		res, err := results[i], errs[i]
		i++
		return res, err
	}
}

func TestShutdownChain(t *testing.T) {
	t.Run("首条 sudo shutdown 成功即停", func(t *testing.T) {
		var got []string
		run := fakeRun(&got, []exec.Result{{ExitCode: 0}}, []error{nil})
		if err := shutdownWith(context.Background(), run); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(got) != 1 || !strings.Contains(got[0], "/sbin/shutdown -h now") {
			t.Errorf("执行了 %v, want 仅 sudo shutdown", got)
		}
	})
	t.Run("sudo 未授权回退 loginwindow", func(t *testing.T) {
		var got []string
		run := fakeRun(&got,
			[]exec.Result{{ExitCode: 1, Stderr: "sudo: a password is required"}, {ExitCode: 0}},
			[]error{nil, nil})
		if err := shutdownWith(context.Background(), run); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(got) != 2 || !strings.Contains(got[1], "loginwindow") {
			t.Errorf("执行了 %v, want 回退到 loginwindow", got)
		}
	})
	t.Run("runFn 返回 error 同样回退", func(t *testing.T) {
		var got []string
		run := fakeRun(&got,
			[]exec.Result{{}, {ExitCode: 0}},
			[]error{errors.New("boom"), nil})
		if err := shutdownWith(context.Background(), run); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(got) != 2 {
			t.Errorf("执行了 %d 条, want 2", len(got))
		}
	})
	t.Run("超时算失败, 走到 pmset 兜底", func(t *testing.T) {
		var got []string
		run := fakeRun(&got,
			[]exec.Result{{ExitCode: -1, TimedOut: true}, {ExitCode: 1}, {ExitCode: 0}},
			[]error{nil, nil, nil})
		if err := shutdownWith(context.Background(), run); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(got) != 3 || !strings.Contains(got[2], "pmset sleepnow") {
			t.Errorf("执行了 %v, want 兜底 pmset sleepnow", got)
		}
	})
	t.Run("全链失败返回汇总错误", func(t *testing.T) {
		var got []string
		run := fakeRun(&got,
			[]exec.Result{{ExitCode: 1, Stderr: "no sudo"}, {ExitCode: 1}, {ExitCode: 1}},
			[]error{nil, nil, nil})
		err := shutdownWith(context.Background(), run)
		if err == nil {
			t.Fatal("err = nil, want 汇总错误")
		}
		if len(got) != 3 {
			t.Errorf("执行了 %d 条, want 3", len(got))
		}
		if !strings.Contains(err.Error(), "全部失败") || !strings.Contains(err.Error(), "no sudo") {
			t.Errorf("错误信息缺少细节: %v", err)
		}
	})
}
