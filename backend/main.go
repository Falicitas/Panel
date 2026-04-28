package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"panel/micro"
	"panel/msgsource"
	"panel/server"
	"panel/store"
	"panel/supplements"
	"panel/todolist"
)

const listenAddr = "127.0.0.1:8089"

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("get home: %v", err)
	}
	panelDir := filepath.Join(home, "panel")
	todolistsDir := filepath.Join(panelDir, "todolists")
	dataDir := filepath.Join(panelDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	st, err := store.Open(filepath.Join(dataDir, "panel.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	log.Printf("store: %s", filepath.Join(dataDir, "panel.db"))

	repo := todolist.NewRepo(todolistsDir)
	log.Printf("todolists: %s", todolistsDir)

	microDir := filepath.Join(todolistsDir, "micro")
	microRepo := micro.NewRepo(microDir)
	log.Printf("micro: %s", microDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := repo.Watch(ctx); err != nil {
		log.Printf("watcher start: %v (continuing without watcher)", err)
	}

	// Phase 9 占位: 启动一个 mock 消息源,验证接口形状
	_ = msgsource.NewMock("wechat")

	// 食物保健品通知生成器: 每 60s 扫一次,active window 写 notification (幂等)
	go func() {
		_, _ = supplements.GenerateNotifications(st, time.Now())
		tick := time.NewTicker(60 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-tick.C:
				if n, err := supplements.GenerateNotifications(st, t); err != nil {
					log.Printf("supplement notif gen: %v", err)
				} else if n > 0 {
					log.Printf("supplement notif gen: %d new", n)
				}
			}
		}
	}()

	srv := server.New(repo, st, microRepo)
	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("listening on http://%s", listenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	sig := <-sigCh
	log.Printf("got %v, shutting down", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}
