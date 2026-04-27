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

	"panel/msgsource"
	"panel/server"
	"panel/store"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := repo.Watch(ctx); err != nil {
		log.Printf("watcher start: %v (continuing without watcher)", err)
	}

	// Phase 9 占位: 启动一个 mock 消息源,验证接口形状
	_ = msgsource.NewMock("wechat")

	srv := server.New(repo, st)
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
