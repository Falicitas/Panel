package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup writes an atomic snapshot of the live database to dstPath via
// `VACUUM INTO`. Safe to call while the store is in use.
func (s *Store) Backup(ctx context.Context, dstPath string) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dstPath); err != nil {
		return fmt.Errorf("vacuum into %s: %w", dstPath, err)
	}
	return nil
}

// RotateBackups keeps the newest `keep` files matching `panel-*.db` in dir
// and deletes the rest. Returns the number of files removed.
func RotateBackups(dir string, keep int) (int, error) {
	if keep < 0 {
		return 0, fmt.Errorf("keep must be >= 0, got %d", keep)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read backup dir: %w", err)
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}
	files := make([]fileInfo, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "panel-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: filepath.Join(dir, name), modTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })

	removed := 0
	for i, f := range files {
		if i < keep {
			continue
		}
		if err := os.Remove(f.path); err != nil {
			return removed, fmt.Errorf("remove %s: %w", f.path, err)
		}
		removed++
	}
	return removed, nil
}

// DailyBackupName returns the conventional backup filename for a given day.
func DailyBackupName(t time.Time) string {
	return fmt.Sprintf("panel-%s.db", t.Format("2006-01-02"))
}

// RunDailyBackups creates one snapshot per day and retains the newest seven.
// Each service must use its own directory so concurrent processes do not
// compete over the same temporary file. The caller must create dir first.
func (s *Store) RunDailyBackups(ctx context.Context, dir string) {
	const keep = 7
	ensureBackup := func(t time.Time) {
		dstPath := filepath.Join(dir, DailyBackupName(t))
		if _, err := os.Stat(dstPath); err == nil {
			return
		}
		tmpPath := dstPath + ".tmp"
		_ = os.Remove(tmpPath)
		if err := s.Backup(ctx, tmpPath); err != nil {
			log.Printf("backup: %v", err)
			return
		}
		if err := os.Rename(tmpPath, dstPath); err != nil {
			log.Printf("backup rename: %v", err)
			return
		}
		log.Printf("backup: %s", dstPath)
		if removed, err := RotateBackups(dir, keep); err != nil {
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
