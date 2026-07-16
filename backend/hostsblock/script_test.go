package hostsblock

// panel-hostsblock 脚本行为测试: 用 PANEL_HOSTS 指向替身文件, 无需 root。
// 沉淀自需求验证路径: block 幂等替换 / unblock 还原 / 非法域名拒绝。

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const scriptRepoPath = "../scripts/panel-hostsblock"

// execScript 以替身 hosts 跑脚本, 返回 exit code 与合并输出。
func execScript(t *testing.T, hostsFile string, args ...string) (int, string) {
	t.Helper()
	cmd := osexec.Command("/bin/bash", append([]string{scriptRepoPath}, args...)...)
	cmd.Env = append(os.Environ(), "PANEL_HOSTS="+hostsFile)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *osexec.ExitError
	if ok := errorsAs(err, &exitErr); ok {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("跑脚本失败: %v (%s)", err, out)
	return -1, ""
}

func errorsAs(err error, target **osexec.ExitError) bool {
	e, ok := err.(*osexec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func newHosts(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(plainHosts), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestScriptBlockUnblock(t *testing.T) {
	hosts := newHosts(t)

	if code, out := execScript(t, hosts, "block", "a.com", "www.a.com"); code != 0 {
		t.Fatalf("block exit=%d out=%s", code, out)
	}
	got := readFile(t, hosts)
	if !strings.Contains(got, plainHosts) {
		t.Error("原有内容未保留")
	}
	for _, line := range []string{"0.0.0.0 a.com", ":: a.com", "0.0.0.0 www.a.com"} {
		if !strings.Contains(got, line) {
			t.Errorf("缺少行 %q, got:\n%s", line, got)
		}
	}
	if !UpToDate(got, []string{"a.com", "www.a.com"}) {
		t.Errorf("Go 侧 UpToDate 应认可脚本输出:\n%s", got)
	}

	// 换名单重复 block = 幂等替换, 不叠加
	if code, out := execScript(t, hosts, "block", "b.com"); code != 0 {
		t.Fatalf("re-block exit=%d out=%s", code, out)
	}
	got = readFile(t, hosts)
	if strings.Contains(got, "a.com") {
		t.Errorf("旧名单未被替换:\n%s", got)
	}
	if strings.Count(got, beginMark) != 1 {
		t.Errorf("标记块叠加了:\n%s", got)
	}

	// unblock 还原为原始内容
	if code, out := execScript(t, hosts, "unblock"); code != 0 {
		t.Fatalf("unblock exit=%d out=%s", code, out)
	}
	if got := readFile(t, hosts); got != plainHosts {
		t.Errorf("unblock 后未还原:\n%q\nwant:\n%q", got, plainHosts)
	}

	// 无块时 unblock 幂等
	if code, _ := execScript(t, hosts, "unblock"); code != 0 {
		t.Errorf("空 unblock exit=%d, want 0", code)
	}
	if got := readFile(t, hosts); got != plainHosts {
		t.Errorf("幂等 unblock 改动了文件:\n%q", got)
	}
}

func TestScriptSetBypass(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	stub := filepath.Join(dir, "networksetup")
	stubSrc := "#!/bin/bash\nprintf '%s\\n' \"$@\" >> " + logPath + "\n"
	if err := os.WriteFile(stub, []byte(stubSrc), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) int {
		cmd := osexec.Command("/bin/bash", append([]string{scriptRepoPath}, args...)...)
		cmd.Env = append(os.Environ(), "PANEL_NETWORKSETUP="+stub)
		if err := cmd.Run(); err != nil {
			var exitErr *osexec.ExitError
			if errorsAs(err, &exitErr) {
				return exitErr.ExitCode()
			}
			t.Fatalf("跑脚本失败: %v", err)
		}
		return 0
	}

	t.Run("整表替换原样转发 (含带空格斜杠的服务名)", func(t *testing.T) {
		if code := run("setbypass", "USB 10/100/1000 LAN",
			"localhost", "10.*", "a.com"); code != 0 {
			t.Fatalf("exit=%d, want 0", code)
		}
		want := "-setproxybypassdomains\nUSB 10/100/1000 LAN\n" +
			"localhost\n10.*\na.com\n"
		if got := readFile(t, logPath); got != want {
			t.Errorf("networksetup 收到:\n%q\nwant:\n%q", got, want)
		}
	})
	t.Run("Empty 清表字面量放行", func(t *testing.T) {
		if code := run("setbypass", "Wi-Fi", "Empty"); code != 0 {
			t.Errorf("exit=%d, want 0", code)
		}
	})
	t.Run("拒绝非法输入", func(t *testing.T) {
		for _, args := range [][]string{
			{"setbypass"},
			{"setbypass", "Wi-Fi"},
			{"setbypass", "-evil", "a.com"},
			{"setbypass", "Wi-Fi", "a;b"},
			{"setbypass", "Wi-Fi", "$(x)"},
		} {
			if code := run(args...); code != 2 {
				t.Errorf("args=%v exit=%d, want 2", args, code)
			}
		}
	})
}

func TestScriptRejects(t *testing.T) {
	hosts := newHosts(t)
	cases := [][]string{
		{"block", "bad;domain.com"},
		{"block", "$(rm -rf /).com"},
		{"block", "a b.com"},
		{"block"},
		{"nonsense"},
		{},
	}
	for _, args := range cases {
		if code, _ := execScript(t, hosts, args...); code != 2 {
			t.Errorf("args=%v exit=%d, want 2", args, code)
		}
	}
	if got := readFile(t, hosts); got != plainHosts {
		t.Errorf("被拒调用不应改文件:\n%q", got)
	}
}
