// panel-dual-notes runs independently of panel-backend and its background jobs.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"panel/dualnotes"
	"panel/store"
)

const listenAddr = "127.0.0.1:8090"

func main() {
	addr := flag.String("listen", listenAddr, "HTTP listen address")
	dataDir := flag.String("data-dir", "", "data directory (default: ~/panel/data)")
	flag.Parse()
	if *dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("get home: %v", err)
		}
		*dataDir = filepath.Join(home, "panel", "data")
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}
	logFile, err := os.OpenFile(filepath.Join(*dataDir, "dual-notes.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("open dual-notes.log: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *dataDir, listener); err != nil {
		log.Fatalf("dual notes: %v", err)
	}
}

// run owns listener and opens the existing database directly. It has no
// dependency on the main service, its HTTP port, or its background workers.
func run(ctx context.Context, dataDir string, listener net.Listener) error {
	defer listener.Close()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir data: %w", err)
	}
	st, err := store.OpenDualNotes(filepath.Join(dataDir, "panel.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	backupsDir := filepath.Join(dataDir, "dual-notes-backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir backups: %w", err)
	}

	backupCtx, cancelBackups := context.WithCancel(ctx)
	backupsDone := make(chan struct{})
	go func() {
		defer close(backupsDone)
		st.RunDailyBackups(backupCtx, backupsDir)
	}()
	defer func() {
		cancelBackups()
		<-backupsDone
	}()

	srv := &http.Server{
		Handler:           dualnotes.New(st).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveDone := make(chan error, 1)
	go func() {
		log.Printf("listening on http://%s", listener.Addr())
		serveDone <- srv.Serve(listener)
	}()
	select {
	case err := <-serveDone:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
