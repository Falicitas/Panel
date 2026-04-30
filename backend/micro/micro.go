package micro

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"panel/dateutil"
	"panel/todolist"
)

// Repo 管理 ~/panel/todolists/micro/{YYYY-MM-DD}.md 文件。
// 当日文件不存在时 LoadToday 会自动创建空文件。
// "当日"按 dateutil.LogicalToday 算 (06:00 日界,凌晨写的还算前一天)。
type Repo struct {
	rootDir string
}

func NewRepo(rootDir string) *Repo {
	return &Repo{rootDir: rootDir}
}

func (r *Repo) RootDir() string { return r.rootDir }

func (r *Repo) todayName() string {
	return dateutil.LogicalToday(time.Now()) + ".md"
}

func (r *Repo) TodayPath() string {
	return filepath.Join(r.rootDir, r.todayName())
}

// LoadToday 读取当日 micro 文件,不存在则**自动新建空文件**。
func (r *Repo) LoadToday() (*todolist.Todolist, error) {
	if err := os.MkdirAll(r.rootDir, 0o755); err != nil {
		return nil, err
	}
	path := r.TodayPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rawStr := strings.ReplaceAll(string(raw), "\r\n", "\n")
	return &todolist.Todolist{
		Name:  r.todayName(),
		Path:  path,
		Done:  false,
		Items: todolist.Parse(rawStr),
		RawMD: rawStr,
	}, nil
}

// WriteToday 覆盖写入当日文件。
func (r *Repo) WriteToday(content string) error {
	if err := os.MkdirAll(r.rootDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.TodayPath(), []byte(content), 0o644)
}
