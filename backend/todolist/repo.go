package todolist

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const checkmarkPrefix = "✅"

// Repo 管理 todolists/ 下 .md 文件的读取与缓存。
// micro/ 子目录由本 repo 不予列出 (Phase 3 单独管)。
type Repo struct {
	rootDir string

	mu    sync.RWMutex
	cache map[string]*Todolist
}

func NewRepo(rootDir string) *Repo {
	return &Repo{rootDir: rootDir, cache: make(map[string]*Todolist)}
}

func (r *Repo) RootDir() string { return r.rootDir }

// List 列出 rootDir 下所有 .md (不递归)。
func (r *Repo) List() ([]*Todolist, error) {
	entries, err := os.ReadDir(r.rootDir)
	if err != nil {
		return nil, err
	}
	var lists []*Todolist
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		tl, err := r.Get(name)
		if err != nil {
			continue
		}
		lists = append(lists, tl)
	}
	return lists, nil
}

// Get 读取(或返回缓存的)指定 todolist。
func (r *Repo) Get(name string) (*Todolist, error) {
	if !validateName(name) {
		return nil, errors.New("invalid todolist name")
	}
	r.mu.RLock()
	if cached, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()
	return r.load(name)
}

func (r *Repo) load(name string) (*Todolist, error) {
	path := filepath.Join(r.rootDir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rawStr := strings.ReplaceAll(string(raw), "\r\n", "\n")
	items := Parse(rawStr)
	tl := &Todolist{
		Name:  name,
		Path:  path,
		Done:  strings.HasPrefix(name, checkmarkPrefix),
		Items: items,
		RawMD: rawStr,
	}
	r.mu.Lock()
	r.cache[name] = tl
	r.mu.Unlock()
	return tl, nil
}

// Write 写入 raw_md (覆盖整个文件) 并失效缓存。
func (r *Repo) Write(name string, content string) error {
	if !validateName(name) {
		return errors.New("invalid todolist name")
	}
	path := filepath.Join(r.rootDir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	r.Invalidate(name)
	return nil
}

// Invalidate 失效指定 name 的缓存 (watcher 在文件变更时调用)。
func (r *Repo) Invalidate(name string) {
	r.mu.Lock()
	delete(r.cache, name)
	r.mu.Unlock()
}

func validateName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		return false
	}
	return true
}
