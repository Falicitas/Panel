// Package review 是精听复习队列的纯算法工具 (无 store 依赖)。
//
// 拆出独立子包是为了避开 store 与 english 之间的 import cycle:
// english 子包 (fetcher / notifications) 需要 store, 而 store/english_review.go 又需要复习算法。
package review

import (
	"fmt"
	"time"
)

// IntervalsDays 艾宾浩斯遗忘曲线间隔: 进入 stage N 时 next_review_at = today + IntervalsDays[N] 天。
//
// stage 含义:
//   - 0: 段落首次完成 → 1 天后首复
//   - 1: 已完成第 1 次复习 → 3 天后第 2 复
//   - 2: 已完成第 2 次复习 → 7 天后第 3 复
//   - 3: 已完成第 3 次复习 → 14 天后第 4 复
//   - 4: 已完成第 4 次复习 → 30 天后第 5 复
//   - 5: mastered (毕业, 不再排日期, 不再展示)
//
// 调高/调低节奏只需改这个数组; 数组长度即 mastered 阈值。
var IntervalsDays = []int{1, 3, 7, 14, 30}

// MasteredStage 是毕业 stage 值 (== len(IntervalsDays))。
// store 层 AdvanceReviewItem 把 stage 推进到这个值时把 mastered_at 置位且不再设 next_review_at。
const MasteredStage = 5

// NextDate 计算 stage 对应的下一次复习日期 (today + IntervalsDays[stage] 天, YYYY-MM-DD)。
//
// stage 越界 (< 0) 报错; stage >= MasteredStage 返回 "" (mastered, 调用方应另存 mastered_at)。
func NextDate(stage int, today string) (string, error) {
	if stage < 0 {
		return "", fmt.Errorf("invalid stage %d (must be >= 0)", stage)
	}
	if stage >= MasteredStage {
		return "", nil
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return "", fmt.Errorf("parse today %q: %w", today, err)
	}
	return t.AddDate(0, 0, IntervalsDays[stage]).Format("2006-01-02"), nil
}
