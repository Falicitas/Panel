package english

import (
	"fmt"

	"panel/store"
)

// PickCandidate 在 topic 内抽下一个 audio。
//
// 规则:
//   - 仅未归档 (archived_at IS NULL)
//   - 排除 excludeIDs (含 currentID, 调用方加进去)
//   - 跳过 LocalPath == "" 的 (没下完不能听)
//   - 跳过 segments_status == "failed" 的 (size cap 拒 / silence / ASR 失败 — 切过去
//     也听不了, 不该进抽奖)
//   - 排序由 store.ListAudios 决定 (last_played_at DESC, published_at DESC)
//   - topic == "" 不过滤
//
// 返回 nil, nil 表示没候选了 (UI 显示空状态)。
func PickCandidate(s *store.Store, topic string, excludeIDs []int64) (*store.EnglishAudio, error) {
	audios, err := s.ListAudios(false, topic)
	if err != nil {
		return nil, fmt.Errorf("list audios topic=%q: %w", topic, err)
	}
	skip := make(map[int64]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		skip[id] = struct{}{}
	}
	for i := range audios {
		a := audios[i]
		if _, drop := skip[a.ID]; drop {
			continue
		}
		if a.LocalPath == "" {
			continue
		}
		if a.SegmentsStatus == store.EnglishStatusFailed {
			continue
		}
		return &a, nil
	}
	return nil, nil
}
