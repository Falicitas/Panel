package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// 英文练习: 音频 / 句子级 segments / 窗口设置。

const (
	EnglishStatusPending = "pending"
	EnglishStatusRunning = "running"
	EnglishStatusReady   = "ready"
	EnglishStatusFailed  = "failed"
)

type EnglishAudio struct {
	ID                   int64  `json:"id"`
	Source               string `json:"source"`     // "voa" | "npr" | "sciam60" | ...
	SourceRef            string `json:"source_ref"` // 源端 GUID
	Title                string `json:"title"`
	Description          string `json:"description"`
	PublishedAt          int64  `json:"published_at"`
	DurationMs           int64  `json:"duration_ms"`
	AudioURL             string `json:"audio_url"`
	LocalPath            string `json:"local_path"`
	Transcript           string `json:"transcript"`
	SegmentsStatus       string `json:"segments_status"` // pending|running|ready|failed
	SegmentsFailedReason string `json:"segments_failed_reason"`
	FetchedAt            int64  `json:"fetched_at"`
	LastPlayedAt         *int64 `json:"last_played_at,omitempty"`
	ArchivedAt           *int64 `json:"archived_at,omitempty"`
	Topic                string `json:"topic"` // free-text: finance/tech/science/current_affairs/general/""
}

type EnglishSegment struct {
	ID      int64  `json:"id"`
	AudioID int64  `json:"audio_id"`
	Idx     int    `json:"idx"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

type EnglishWindow struct {
	Start string `json:"start"` // HH:MM
	End   string `json:"end"`
}

type EnglishSettings struct {
	Windows      []EnglishWindow `json:"windows"`
	ThresholdMin int             `json:"threshold_min"`
	Enabled      bool            `json:"enabled"`
}

const audioCols = `id, source, source_ref, title, description, published_at, duration_ms,
	audio_url, local_path, transcript, segments_status, segments_failed_reason,
	fetched_at, last_played_at, archived_at, topic`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAudio(r rowScanner) (EnglishAudio, error) {
	var a EnglishAudio
	var localPath sql.NullString
	var lastPlayed, archivedAt sql.NullInt64
	err := r.Scan(&a.ID, &a.Source, &a.SourceRef, &a.Title, &a.Description,
		&a.PublishedAt, &a.DurationMs, &a.AudioURL, &localPath, &a.Transcript,
		&a.SegmentsStatus, &a.SegmentsFailedReason, &a.FetchedAt, &lastPlayed, &archivedAt,
		&a.Topic)
	if err != nil {
		return EnglishAudio{}, err
	}
	if localPath.Valid {
		a.LocalPath = localPath.String
	}
	if lastPlayed.Valid {
		v := lastPlayed.Int64
		a.LastPlayedAt = &v
	}
	if archivedAt.Valid {
		v := archivedAt.Int64
		a.ArchivedAt = &v
	}
	return a, nil
}

// ListAudios archived=false 列出未归档,true 列出已归档。
//
// 排序规则:
//   - archived=true: ORDER BY archived_at DESC
//   - archived=false: ORDER BY COALESCE(last_played_at, 0) DESC, published_at DESC
//     (last_played_at 优先 = 用户主动 /skip 选过的 audio 顶到首位; 没动过的 fallback
//     按 published_at 最新优先。修复 issue-32 M5 bug: skip 后 /today 命中 NPR 最新一条
//     而不是 user 选的 sciam, 因为 NPR.published_at > sciam.published_at)
//
// topic="" 不过滤; 非空 case-insensitive 匹配 topic 列。
func (s *Store) ListAudios(archived bool, topic string) ([]EnglishAudio, error) {
	var (
		q    string
		args []any
	)
	base := `SELECT ` + audioCols + ` FROM english_audios WHERE `
	if archived {
		q = base + `archived_at IS NOT NULL`
	} else {
		q = base + `archived_at IS NULL`
	}
	if topic != "" {
		q += ` AND LOWER(topic) = LOWER(?)`
		args = append(args, topic)
	}
	if archived {
		q += ` ORDER BY archived_at DESC`
	} else {
		q += ` ORDER BY COALESCE(last_played_at, 0) DESC, published_at DESC`
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list english_audios archived=%v topic=%q: %w", archived, topic, err)
	}
	defer rows.Close()
	out := []EnglishAudio{}
	for rows.Next() {
		a, err := scanAudio(rows)
		if err != nil {
			return nil, fmt.Errorf("scan english_audio row: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAudio(id int64) (EnglishAudio, error) {
	row := s.db.QueryRow(`SELECT `+audioCols+` FROM english_audios WHERE id=?`, id)
	a, err := scanAudio(row)
	if errors.Is(err, sql.ErrNoRows) {
		return EnglishAudio{}, sql.ErrNoRows
	}
	if err != nil {
		return EnglishAudio{}, fmt.Errorf("get english_audio %d: %w", id, err)
	}
	return a, nil
}

// InsertAudioIfNew 按 (source, source_ref) 幂等插入。
// 返回 (id, isNew, err): isNew=false 表示已存在,id 是已有行的 id。
func (s *Store) InsertAudioIfNew(a EnglishAudio) (int64, bool, error) {
	var existingID int64
	err := s.db.QueryRow(
		`SELECT id FROM english_audios WHERE source=? AND source_ref=?`,
		a.Source, a.SourceRef,
	).Scan(&existingID)
	if err == nil {
		return existingID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("query existing english_audio: %w", err)
	}
	if a.FetchedAt == 0 {
		a.FetchedAt = time.Now().Unix()
	}
	if a.SegmentsStatus == "" {
		a.SegmentsStatus = EnglishStatusPending
	}
	var localPath any
	if a.LocalPath != "" {
		localPath = a.LocalPath
	}
	res, err := s.db.Exec(`
		INSERT INTO english_audios
			(source, source_ref, title, description, published_at, duration_ms,
			 audio_url, local_path, transcript, segments_status, segments_failed_reason,
			 fetched_at, topic)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Source, a.SourceRef, a.Title, a.Description, a.PublishedAt, a.DurationMs,
		a.AudioURL, localPath, a.Transcript, a.SegmentsStatus, a.SegmentsFailedReason,
		a.FetchedAt, a.Topic,
	)
	if err != nil {
		return 0, false, fmt.Errorf("insert english_audio: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, true, nil
}

// SetAudioLocalPath 更新本地缓存路径。
func (s *Store) SetAudioLocalPath(id int64, path string) error {
	var v any
	if path != "" {
		v = path
	}
	_, err := s.db.Exec(`UPDATE english_audios SET local_path=? WHERE id=?`, v, id)
	if err != nil {
		return fmt.Errorf("set local_path %d: %w", id, err)
	}
	return nil
}

// SetAudioSegmentsStatus 更新 segments_status (失败时同时写 reason,成功时清空)。
func (s *Store) SetAudioSegmentsStatus(id int64, status, failedReason string) error {
	_, err := s.db.Exec(
		`UPDATE english_audios SET segments_status=?, segments_failed_reason=? WHERE id=?`,
		status, failedReason, id,
	)
	if err != nil {
		return fmt.Errorf("set segments_status %d: %w", id, err)
	}
	return nil
}

// DismissStaleEnglishNotifications 把 source='english' 且 created_at < beforeUnix
// 且未 dismissed 的通知一并 dismissed。返回受影响行数。
// 用于 GenerateNotifications 入口, 避免昨天的英文精听通知堆到今天 (与 supplements 同模式)。
func (s *Store) DismissStaleEnglishNotifications(beforeUnix, atUnix int64) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE notifications
		    SET dismissed_at=?
		  WHERE source='english' AND dismissed_at IS NULL AND created_at < ?`,
		atUnix, beforeUnix,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SetAudioArchived atUnix=0 取消归档,>0 标记归档时间。
func (s *Store) SetAudioArchived(id int64, atUnix int64) error {
	var v any
	if atUnix > 0 {
		v = atUnix
	}
	_, err := s.db.Exec(`UPDATE english_audios SET archived_at=? WHERE id=?`, v, id)
	if err != nil {
		return fmt.Errorf("set archived_at %d: %w", id, err)
	}
	return nil
}

// SetAudioLastPlayedAt 更新最后播放时间 (unix 秒)。
func (s *Store) SetAudioLastPlayedAt(id int64, atUnix int64) error {
	_, err := s.db.Exec(`UPDATE english_audios SET last_played_at=? WHERE id=?`, atUnix, id)
	if err != nil {
		return fmt.Errorf("set last_played_at %d: %w", id, err)
	}
	return nil
}

// ListSegments 按 idx 升序列出 audio 的所有 segments。
func (s *Store) ListSegments(audioID int64) ([]EnglishSegment, error) {
	rows, err := s.db.Query(
		`SELECT id, audio_id, idx, start_ms, end_ms, text
		   FROM english_segments WHERE audio_id=? ORDER BY idx ASC`, audioID,
	)
	if err != nil {
		return nil, fmt.Errorf("list english_segments %d: %w", audioID, err)
	}
	defer rows.Close()
	out := []EnglishSegment{}
	for rows.Next() {
		var seg EnglishSegment
		if err := rows.Scan(&seg.ID, &seg.AudioID, &seg.Idx, &seg.StartMs, &seg.EndMs, &seg.Text); err != nil {
			return nil, fmt.Errorf("scan english_segment row: %w", err)
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

// ReplaceSegments 事务内: 删旧 + 插新 + 设 status=ready (清空 failed_reason)。
// 用于 mac ASR 完成后回写。
func (s *Store) ReplaceSegments(audioID int64, segs []EnglishSegment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM english_segments WHERE audio_id=?`, audioID); err != nil {
		return fmt.Errorf("delete old segments %d: %w", audioID, err)
	}
	stmt, err := tx.Prepare(
		`INSERT INTO english_segments (audio_id, idx, start_ms, end_ms, text)
		 VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert segment: %w", err)
	}
	defer stmt.Close()
	for _, seg := range segs {
		if _, err := stmt.Exec(audioID, seg.Idx, seg.StartMs, seg.EndMs, seg.Text); err != nil {
			return fmt.Errorf("insert segment idx=%d: %w", seg.Idx, err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE english_audios SET segments_status=?, segments_failed_reason='' WHERE id=?`,
		EnglishStatusReady, audioID,
	); err != nil {
		return fmt.Errorf("set status ready: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetEnglishSettings 读单行 settings (id=1)。无行返回硬编码默认。
func (s *Store) GetEnglishSettings() (EnglishSettings, error) {
	var raw string
	var threshold int
	var enabled int
	err := s.db.QueryRow(
		`SELECT windows_json, threshold_min, enabled FROM english_settings WHERE id=1`,
	).Scan(&raw, &threshold, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultEnglishSettings(), nil
	}
	if err != nil {
		return EnglishSettings{}, fmt.Errorf("get english_settings: %w", err)
	}
	windows := []EnglishWindow{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &windows); err != nil {
			return EnglishSettings{}, fmt.Errorf("parse windows_json: %w", err)
		}
	}
	if windows == nil {
		windows = []EnglishWindow{}
	}
	return EnglishSettings{
		Windows:      windows,
		ThresholdMin: threshold,
		Enabled:      enabled != 0,
	}, nil
}

// SetEnglishSettings 全量替换 id=1 行。
func (s *Store) SetEnglishSettings(es EnglishSettings) error {
	if es.Windows == nil {
		es.Windows = []EnglishWindow{}
	}
	raw, err := json.Marshal(es.Windows)
	if err != nil {
		return fmt.Errorf("marshal windows: %w", err)
	}
	enabled := 0
	if es.Enabled {
		enabled = 1
	}
	_, err = s.db.Exec(
		`UPDATE english_settings SET windows_json=?, threshold_min=?, enabled=? WHERE id=1`,
		string(raw), es.ThresholdMin, enabled,
	)
	if err != nil {
		return fmt.Errorf("update english_settings: %w", err)
	}
	return nil
}

func defaultEnglishSettings() EnglishSettings {
	return EnglishSettings{
		Windows: []EnglishWindow{
			{Start: "07:30", End: "09:00"},
			{Start: "21:00", End: "22:30"},
		},
		ThresholdMin: 10,
		Enabled:      true,
	}
}

// ComputeWindowMinutes 按 records.module='english_seg' 在 date 当日的勾选时长 (value_num=秒数),
// 按 record.created_at 落入 windows[i] 的 [startUnix, endUnix) 区间分桶,返回 map[idx] -> minutes。
// 落在所有窗口外的不计入任一桶。所有 idx (含为 0) 都会出现在返回 map 中。
func (s *Store) ComputeWindowMinutes(date string, windows []EnglishWindow) (map[int]float64, error) {
	out := make(map[int]float64, len(windows))
	for i := range windows {
		out[i] = 0
	}
	if len(windows) == 0 {
		return out, nil
	}
	ranges := make([][2]int64, 0, len(windows))
	for i, w := range windows {
		startT, err := parseHMOnDate(date, w.Start)
		if err != nil {
			return nil, fmt.Errorf("parse window %d start %q: %w", i, w.Start, err)
		}
		endT, err := parseHMOnDate(date, w.End)
		if err != nil {
			return nil, fmt.Errorf("parse window %d end %q: %w", i, w.End, err)
		}
		ranges = append(ranges, [2]int64{startT.Unix(), endT.Unix()})
	}
	rows, err := s.db.Query(
		`SELECT created_at, value_num FROM records
		  WHERE module='english_seg' AND date=? AND value_num IS NOT NULL`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("query english_seg records: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var createdAt int64
		var seconds float64
		if err := rows.Scan(&createdAt, &seconds); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		for i, r := range ranges {
			if createdAt >= r[0] && createdAt < r[1] {
				out[i] += seconds / 60.0
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate records: %w", err)
	}
	return out, nil
}

func parseHMOnDate(date, hhmm string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, time.Local)
}
