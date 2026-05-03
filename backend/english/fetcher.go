package english

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"panel/store"
)

// Fetcher 是单一音频源的抽象。
// Fetch 列出当前 feed 内所有候选;DownloadAudio 把单条流式存盘。
type Fetcher interface {
	Name() string
	Fetch(ctx context.Context) ([]AudioMeta, error)
	DownloadAudio(ctx context.Context, audioURL, destPath string) (int64, error)
}

// RunFetchAll 串行调每个 fetcher:
//   - Fetch 列出所有候选;每条都做 InsertAudioIfNew (元数据全入库,UI 看得到)
//   - 但每轮最多只下载 maxDownloadsPerRun 条 mp3 (本地缺失的优先)
//   - 已有 local_path 的跳过;限额满了的 audio 留到下一轮 cron 补
//   - 每次成功下载后 sleep politeDelay (礼貌限速)
//
// VOA 单 RSS ~20 条 × ~30MB,首轮下完会 ~600MB,所以 maxDownloadsPerRun 必须有上限。
func RunFetchAll(ctx context.Context, st *store.Store, fetchers []Fetcher, dataDir string, politeDelay time.Duration, maxDownloadsPerRun int) error {
	for _, fetcher := range fetchers {
		items, err := fetcher.Fetch(ctx)
		if err != nil {
			log.Printf("english fetch %s: %v", fetcher.Name(), err)
			continue
		}
		log.Printf("english fetch %s: got %d items", fetcher.Name(), len(items))
		downloaded := 0
		for _, m := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			id, _, err := st.InsertAudioIfNew(store.EnglishAudio{
				Source:      m.Source,
				SourceRef:   m.SourceRef,
				Title:       m.Title,
				Description: m.Description,
				PublishedAt: m.PublishedAt,
				DurationMs:  m.DurationMs,
				AudioURL:    m.AudioURL,
				FetchedAt:   time.Now().Unix(),
			})
			if err != nil {
				log.Printf("english fetch %s: insert ref=%q: %v", fetcher.Name(), m.SourceRef, err)
				continue
			}
			a, err := st.GetAudio(id)
			if err != nil {
				log.Printf("english fetch %s: get id=%d: %v", fetcher.Name(), id, err)
				continue
			}
			if a.LocalPath != "" {
				continue // 已下载
			}
			if downloaded >= maxDownloadsPerRun {
				continue // 本轮限额满, 留给下一轮
			}
			destPath := filepath.Join(dataDir, fmt.Sprintf("%d.mp3", id))
			n, err := fetcher.DownloadAudio(ctx, m.AudioURL, destPath)
			if err != nil {
				log.Printf("english fetch %s: download id=%d: %v", fetcher.Name(), id, err)
				continue
			}
			if err := st.SetAudioLocalPath(id, destPath); err != nil {
				log.Printf("english fetch %s: set local_path id=%d: %v", fetcher.Name(), id, err)
				continue
			}
			downloaded++
			log.Printf("english fetch %s: downloaded id=%d %d bytes -> %s", fetcher.Name(), id, n, destPath)

			if politeDelay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(politeDelay):
				}
			}
		}
		if downloaded > 0 {
			log.Printf("english fetch %s: %d new audios downloaded this run", fetcher.Name(), downloaded)
		}
	}
	return nil
}
