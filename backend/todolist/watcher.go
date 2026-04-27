package todolist

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Watch 在 rootDir 上启动 fsnotify 监听,文件变更后失效对应缓存。
// 调用方负责 cancel ctx 来停止监听。
func (r *Repo) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(r.rootDir); err != nil {
		w.Close()
		return err
	}
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				name := filepath.Base(ev.Name)
				if !strings.EqualFold(filepath.Ext(name), ".md") {
					continue
				}
				r.Invalidate(name)
				log.Printf("watcher: invalidate %q (%s)", name, ev.Op)
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()
	return nil
}
