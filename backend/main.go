package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"panel/english"
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

	logPath := filepath.Join(dataDir, "panel.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("open panel.log: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("log: %s", logPath)

	st, err := store.Open(filepath.Join(dataDir, "panel.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	log.Printf("store: %s", filepath.Join(dataDir, "panel.db"))

	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		log.Fatalf("mkdir backups: %v", err)
	}

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

	// 数据库每日备份: 启动 + 每 6h 扫一次,缺当日备份则 VACUUM INTO,保留 7 份
	go runBackupLoop(ctx, st, backupsDir)

	// 食物保健品 + 英文练习通知生成器: 每 60s 扫一次, active window 写 notification (幂等)
	go func() {
		_, _ = supplements.GenerateNotifications(st, time.Now())
		_, _ = english.GenerateNotifications(st, time.Now())
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
				if n, err := english.GenerateNotifications(st, t); err != nil {
					log.Printf("english notif gen: %v", err)
				} else if n > 0 {
					log.Printf("english notif gen: %d new", n)
				}
			}
		}
	}()

	// 英文练习音频源抓取: 启动 + 每 6h 扫一次,新条目下载到 ~/panel/data/english/{id}.mp3
	go runEnglishFetchLoop(ctx, st, filepath.Join(dataDir, "english"))

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

func runEnglishFetchLoop(ctx context.Context, st *store.Store, englishDir string) {
	if err := os.MkdirAll(englishDir, 0o755); err != nil {
		log.Printf("english mkdir %s: %v", englishDir, err)
		return
	}
	// MVP 主源 = NPR Indicator (经济新闻 9-11min, mp3 真有声, ≤25MB 适配 Groq Whisper)。
	// VOA Akamai CDN mp3 全静音 (issue-11 调研发现 -90dB 占位), 暂不注册;
	// Phase 1 加 audio energy 探测后再考虑回挂 VOA + Sciam 作多源轮播。
	fetchers := []english.Fetcher{english.NewNPRFetcher()}
	runOnce := func() {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := english.RunFetchAll(runCtx, st, fetchers, englishDir, time.Second, 5); err != nil {
			log.Printf("english fetch run: %v", err)
		}
	}
	runOnce()
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			runOnce()
		}
	}
}

func runBackupLoop(ctx context.Context, st *store.Store, dir string) {
	const keep = 7
	ensureBackup := func(t time.Time) {
		dstPath := filepath.Join(dir, store.DailyBackupName(t))
		if _, err := os.Stat(dstPath); err == nil {
			return
		}
		tmpPath := dstPath + ".tmp"
		_ = os.Remove(tmpPath)
		if err := st.Backup(ctx, tmpPath); err != nil {
			log.Printf("backup: %v", err)
			return
		}
		if err := os.Rename(tmpPath, dstPath); err != nil {
			log.Printf("backup rename: %v", err)
			return
		}
		log.Printf("backup: %s", dstPath)
		if removed, err := store.RotateBackups(dir, keep); err != nil {
			log.Printf("backup rotate: %v", err)
		} else if removed > 0 {
			log.Printf("backup rotate: removed %d", removed)
		}
	}
	ensureBackup(time.Now())
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-tick.C:
			ensureBackup(t)
		}
	}
}
