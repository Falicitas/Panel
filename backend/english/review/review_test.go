package review

import "testing"

func TestNextDate(t *testing.T) {
	cases := []struct {
		name    string
		stage   int
		today   string
		want    string
		wantErr bool
	}{
		{"stage 0 → +1 day", 0, "2026-05-10", "2026-05-11", false},
		{"stage 1 → +3 day", 1, "2026-05-10", "2026-05-13", false},
		{"stage 2 → +7 day", 2, "2026-05-10", "2026-05-17", false},
		{"stage 3 → +14 day", 3, "2026-05-10", "2026-05-24", false},
		{"stage 4 → +30 day", 4, "2026-05-10", "2026-06-09", false},
		{"stage 5 = mastered", 5, "2026-05-10", "", false},
		{"stage 6 同 mastered", 6, "2026-05-10", "", false},
		{"stage -1 报错", -1, "2026-05-10", "", true},
		{"today 跨月", 0, "2026-05-31", "2026-06-01", false},
		{"today 跨年", 0, "2026-12-31", "2027-01-01", false},
		{"today 闰年 2 月末", 0, "2024-02-28", "2024-02-29", false},
		{"today 非闰年 2 月末", 0, "2026-02-28", "2026-03-01", false},
		{"today 格式错误", 0, "2026/05/10", "", true},
		{"today 空", 0, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NextDate(c.stage, c.today)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestIntervalsDays(t *testing.T) {
	if len(IntervalsDays) != MasteredStage {
		t.Errorf("len(IntervalsDays) = %d, MasteredStage = %d, must equal",
			len(IntervalsDays), MasteredStage)
	}
	want := []int{1, 3, 7, 14, 30}
	for i, v := range want {
		if IntervalsDays[i] != v {
			t.Errorf("IntervalsDays[%d] = %d, want %d", i, IntervalsDays[i], v)
		}
	}
}
