package store

import (
	"database/sql"
	"errors"
	"fmt"

	"panel/dateutil"
	"panel/english/review"
)

// ReviewItem 是 english_review_queue 表的一行。
//
// 状态机:
//   - 入队 (EnsureReviewItem): stage=0, next_review_at = today + 1
//   - 推进 (AdvanceReviewItem): stage++; stage 达到 review.MasteredStage 时置 mastered_at
//   - 延后 (PostponeReviewItem): next_review_at = newDate; postponed_count++
//   - 跳过 (FinishReviewItem): finished_at = nowUnix; 不再展示
//
// 退出条件 (不再被 ListDueReviewItems 返回):
//   - finished_at IS NOT NULL (用户主动跳过)
//   - mastered_at IS NOT NULL (5 stage 自然毕业)
//
// 物理删除 (DeleteReviewItem) 用于: 取消勾选段落 / audio 永久删除 / segment 被替换后孤儿清理。
type ReviewItem struct {
	ID             int64  `json:"id"`
	AudioID        int64  `json:"audio_id"`
	SegIdx         int    `json:"seg_idx"`
	Stage          int    `json:"stage"`
	NextReviewAt   string `json:"next_review_at"` // YYYY-MM-DD; mastered 行可能为 ""
	FirstDoneAt    int64  `json:"first_done_at"`
	LastAdvancedAt int64  `json:"last_advanced_at"`
	PostponedCount int    `json:"postponed_count"`
	FinishedAt     *int64 `json:"finished_at,omitempty"`
	MasteredAt     *int64 `json:"mastered_at,omitempty"`
}

const reviewCols = `id, audio_id, seg_idx, stage, next_review_at, first_done_at,
		last_advanced_at, postponed_count, finished_at, mastered_at`

func scanReviewItem(r rowScanner) (ReviewItem, error) {
	var it ReviewItem
	var finishedAt, masteredAt sql.NullInt64
	err := r.Scan(&it.ID, &it.AudioID, &it.SegIdx, &it.Stage, &it.NextReviewAt,
		&it.FirstDoneAt, &it.LastAdvancedAt, &it.PostponedCount, &finishedAt, &masteredAt)
	if err != nil {
		return ReviewItem{}, err
	}
	if finishedAt.Valid {
		v := finishedAt.Int64
		it.FinishedAt = &v
	}
	if masteredAt.Valid {
		v := masteredAt.Int64
		it.MasteredAt = &v
	}
	return it, nil
}

// EnsureReviewItem upsert 语义:
//   - 不存在 → INSERT stage=0, next_review_at=nextReview, first_done_at=last_advanced_at=nowUnix
//   - 已存在 → no-op (重复勾选不重置 stage; 真要重置先 DeleteReviewItem 再 EnsureReviewItem)
//
// 返回 created=true 表示新插入, false 表示已存在。
func (s *Store) EnsureReviewItem(audioID int64, segIdx int, nextReview string, nowUnix int64) (bool, error) {
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO english_review_queue
			(audio_id, seg_idx, stage, next_review_at, first_done_at, last_advanced_at, postponed_count)
		VALUES (?, ?, 0, ?, ?, ?, 0)`,
		audioID, segIdx, nextReview, nowUnix, nowUnix,
	)
	if err != nil {
		return false, fmt.Errorf("ensure review item %d:%d: %w", audioID, segIdx, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteReviewItem 物理删除 (audio_id, seg_idx) 对应行。无匹配静默 nil。
//
// 用于:
//   - 用户取消勾选段落 (POST /segments/check checked=false)
//   - DeleteAudio 级联清理 (在事务内调 DeleteReviewItemsByAudio)
//   - ReplaceSegments 替换 segments 后清理孤儿 (在事务内调 DeleteReviewItemsByAudioExcept)
func (s *Store) DeleteReviewItem(audioID int64, segIdx int) error {
	_, err := s.db.Exec(
		`DELETE FROM english_review_queue WHERE audio_id=? AND seg_idx=?`,
		audioID, segIdx,
	)
	if err != nil {
		return fmt.Errorf("delete review item %d:%d: %w", audioID, segIdx, err)
	}
	return nil
}

// AdvanceReviewItem 推进 stage:
//   - newStage = oldStage + 1
//   - 若 newStage < MasteredStage: next_review_at = today + ReviewIntervalsDays[newStage]
//   - 若 newStage >= MasteredStage: 置 mastered_at, next_review_at 不动 (但已经退出 ListDueReviewItems)
//
// 已 finished_at 或已 mastered_at 的行不允许 advance, 报错。
func (s *Store) AdvanceReviewItem(id int64, today string, nowUnix int64) (ReviewItem, error) {
	cur, err := s.getReviewItemByID(id)
	if err != nil {
		return ReviewItem{}, err
	}
	if cur.FinishedAt != nil {
		return ReviewItem{}, fmt.Errorf("review item %d already finished", id)
	}
	if cur.MasteredAt != nil {
		return ReviewItem{}, fmt.Errorf("review item %d already mastered", id)
	}
	newStage := cur.Stage + 1
	if newStage >= review.MasteredStage {
		_, err := s.db.Exec(
			`UPDATE english_review_queue
			    SET stage=?, last_advanced_at=?, mastered_at=?
			  WHERE id=?`,
			review.MasteredStage, nowUnix, nowUnix, id,
		)
		if err != nil {
			return ReviewItem{}, fmt.Errorf("advance review %d to mastered: %w", id, err)
		}
		return s.getReviewItemByID(id)
	}
	nextDate, err := review.NextDate(newStage, today)
	if err != nil {
		return ReviewItem{}, fmt.Errorf("compute next date stage=%d today=%q: %w", newStage, today, err)
	}
	_, err = s.db.Exec(
		`UPDATE english_review_queue
		    SET stage=?, next_review_at=?, last_advanced_at=?
		  WHERE id=?`,
		newStage, nextDate, nowUnix, id,
	)
	if err != nil {
		return ReviewItem{}, fmt.Errorf("advance review %d: %w", id, err)
	}
	return s.getReviewItemByID(id)
}

// PostponeReviewItem 把 next_review_at 改为 newDate, postponed_count++。不动 stage。
//
// 只在用户主动点延后时调。配额溢出不再走这里 — 每日配额是纯展示上限, 不落库。
// 已 finished/mastered 的行报错。
func (s *Store) PostponeReviewItem(id int64, newDate string, nowUnix int64) (ReviewItem, error) {
	cur, err := s.getReviewItemByID(id)
	if err != nil {
		return ReviewItem{}, err
	}
	if cur.FinishedAt != nil {
		return ReviewItem{}, fmt.Errorf("review item %d already finished", id)
	}
	if cur.MasteredAt != nil {
		return ReviewItem{}, fmt.Errorf("review item %d already mastered", id)
	}
	_, err = s.db.Exec(
		`UPDATE english_review_queue
		    SET next_review_at=?, postponed_count=postponed_count+1
		  WHERE id=?`,
		newDate, id,
	)
	if err != nil {
		return ReviewItem{}, fmt.Errorf("postpone review %d: %w", id, err)
	}
	_ = nowUnix // 当前不写 last_advanced_at; postpone 不算 stage 推进
	return s.getReviewItemByID(id)
}

// FinishReviewItem 用户主动跳过, 写 finished_at。已 finished/mastered 的报错。
func (s *Store) FinishReviewItem(id int64, nowUnix int64) error {
	cur, err := s.getReviewItemByID(id)
	if err != nil {
		return err
	}
	if cur.FinishedAt != nil {
		return fmt.Errorf("review item %d already finished", id)
	}
	if cur.MasteredAt != nil {
		return fmt.Errorf("review item %d already mastered", id)
	}
	_, err = s.db.Exec(
		`UPDATE english_review_queue SET finished_at=? WHERE id=?`,
		nowUnix, id,
	)
	if err != nil {
		return fmt.Errorf("finish review %d: %w", id, err)
	}
	return nil
}

// ListDueReviewItems 返回到期 (next_review_at <= today) 且未 finished / 未 mastered 的项。
// ORDER BY next_review_at ASC (老的优先), audio_id ASC, seg_idx ASC。
func (s *Store) ListDueReviewItems(today string) ([]ReviewItem, error) {
	rows, err := s.db.Query(
		`SELECT `+reviewCols+`
		   FROM english_review_queue
		  WHERE next_review_at <= ?
		    AND finished_at IS NULL
		    AND mastered_at IS NULL
		  ORDER BY next_review_at ASC, audio_id ASC, seg_idx ASC`,
		today,
	)
	if err != nil {
		return nil, fmt.Errorf("list due review items today=%q: %w", today, err)
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		it, err := scanReviewItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan review item row: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetReviewItemBySeg 按 (audio_id, seg_idx) 查找。无行返回 (zero, false, nil)。
func (s *Store) GetReviewItemBySeg(audioID int64, segIdx int) (ReviewItem, bool, error) {
	row := s.db.QueryRow(
		`SELECT `+reviewCols+`
		   FROM english_review_queue WHERE audio_id=? AND seg_idx=?`,
		audioID, segIdx,
	)
	it, err := scanReviewItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ReviewItem{}, false, nil
	}
	if err != nil {
		return ReviewItem{}, false, fmt.Errorf("get review item by seg %d:%d: %w", audioID, segIdx, err)
	}
	return it, true, nil
}

// reviewDoneTodayCond 是"今日已完成复习"的统一判定 (占位符: last_advanced_at 区间的 start, end):
//   - stage > 0: 排掉只入队没复习过的行 (EnsureReviewItem 也会写 last_advanced_at)
//   - last_advanced_at 落在 today 的逻辑日区间 [06:00, 次日 06:00)
//   - finished_at IS NULL: 复习完又被用户永久跳过的不再露面
//
// 已 mastered 的算在内 — 今天复到毕业的那条同样该留在队列里让用户看见。
const reviewDoneTodayCond = `stage > 0
		    AND last_advanced_at >= ? AND last_advanced_at < ?
		    AND finished_at IS NULL`

// ListReviewItemsDoneToday 返回今日已完成复习的项, ORDER BY last_advanced_at ASC (先复的排前面)。
//
// 这批行的 next_review_at 已经被 AdvanceReviewItem 推到未来, 所以不会再出现在
// ListDueReviewItems 里 — 队列要把它们常驻在前缀展示, 必须单独查这一份。
func (s *Store) ListReviewItemsDoneToday(today string) ([]ReviewItem, error) {
	start, end, err := dateutil.LogicalDayRangeUnix(today)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT `+reviewCols+`
		   FROM english_review_queue
		  WHERE `+reviewDoneTodayCond+`
		  ORDER BY last_advanced_at ASC, audio_id ASC, seg_idx ASC`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("list review done today=%q: %w", today, err)
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		it, err := scanReviewItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan review done row: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CountReviewDoneToday 统计今日已完成 (advance) 的复习条数 = len(ListReviewItemsDoneToday)。
// 判定见 reviewDoneTodayCond。
func (s *Store) CountReviewDoneToday(today string) (int, error) {
	start, end, err := dateutil.LogicalDayRangeUnix(today)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM english_review_queue WHERE `+reviewDoneTodayCond,
		start, end,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count review done today=%q: %w", today, err)
	}
	return count, nil
}

// IsReviewItemDoneToday 判断单行今天是否已经推进过, 供 handler 挡重复 advance
// (队列现在常驻显示已完成行, 陈旧 UI 再点一次不该白跳一个 stage)。
func (s *Store) IsReviewItemDoneToday(id int64, today string) (bool, error) {
	start, end, err := dateutil.LogicalDayRangeUnix(today)
	if err != nil {
		return false, err
	}
	var n int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM english_review_queue
		  WHERE id=? AND `+reviewDoneTodayCond,
		id, start, end,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check review %d done today=%q: %w", id, today, err)
	}
	return n > 0, nil
}

func (s *Store) getReviewItemByID(id int64) (ReviewItem, error) {
	row := s.db.QueryRow(`SELECT `+reviewCols+` FROM english_review_queue WHERE id=?`, id)
	it, err := scanReviewItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ReviewItem{}, sql.ErrNoRows
	}
	if err != nil {
		return ReviewItem{}, fmt.Errorf("get review item %d: %w", id, err)
	}
	return it, nil
}
