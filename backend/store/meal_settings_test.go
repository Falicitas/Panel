package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestIsValidMeal(t *testing.T) {
	tests := []struct {
		meal string
		want bool
	}{
		{"breakfast", true},
		{"lunch", true},
		{"dinner", true},
		{"snack", false},
		{"", false},
		{"Breakfast", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := IsValidMeal(tt.meal); got != tt.want {
			t.Errorf("IsValidMeal(%q) = %v, want %v", tt.meal, got, tt.want)
		}
	}
}

// TestMealSettings_DefaultAllNil 初始化后 GetMealPausedAll 返三餐 nil — seed 行存在但 paused_until=NULL。
func TestMealSettings_DefaultAllNil(t *testing.T) {
	s := openTestStore(t)
	got, err := s.GetMealPausedAll()
	if err != nil {
		t.Fatalf("GetMealPausedAll: %v", err)
	}
	for _, meal := range []string{"breakfast", "lunch", "dinner"} {
		if got[meal] != nil {
			t.Errorf("%s: paused_until = %v, want nil", meal, *got[meal])
		}
	}
}

// TestMealSettings_DefaultEnabled 初始化后 GetMealSettingsAll 返三餐 enabled=true。
// 行为跟 supplement.enabled 默认 true 对齐, seed migration DEFAULT 1。
func TestMealSettings_DefaultEnabled(t *testing.T) {
	s := openTestStore(t)
	got, err := s.GetMealSettingsAll()
	if err != nil {
		t.Fatalf("GetMealSettingsAll: %v", err)
	}
	for _, meal := range []string{"breakfast", "lunch", "dinner"} {
		if !got[meal].Enabled {
			t.Errorf("%s: enabled = false, want true (default)", meal)
		}
	}
}

// TestMealSettings_SetEnabled SetMealEnabled 写入后 GetMealSettingsAll 立即反映。
// 不动 paused_until — 两轴独立 (跟 supplement.enabled / paused_until 行为对齐)。
func TestMealSettings_SetEnabled(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)
	if err := s.SetMealPausedUntil("lunch", &until); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	if err := s.SetMealEnabled("lunch", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	got, _ := s.GetMealSettingsAll()
	if got["lunch"].Enabled {
		t.Errorf("lunch enabled = true, want false")
	}
	// paused_until 不被覆盖
	if got["lunch"].PausedUntil == nil || *got["lunch"].PausedUntil != until {
		t.Errorf("lunch paused_until 被 SetEnabled 误覆盖: %v", got["lunch"].PausedUntil)
	}
	// 其它餐不受影响
	if !got["breakfast"].Enabled {
		t.Errorf("breakfast enabled 不该被影响")
	}
}

// TestMealSetting_IsActive enabled=false 或 paused 时 IsActive=false。
func TestMealSetting_IsActive(t *testing.T) {
	now := int64(1700000000)
	future := now + 3600
	tests := []struct {
		name string
		ms   MealSetting
		want bool
	}{
		{"enabled + no pause", MealSetting{Enabled: true}, true},
		{"disabled", MealSetting{Enabled: false}, false},
		{"enabled + paused future", MealSetting{Enabled: true, PausedUntil: &future}, false},
		{"disabled + paused", MealSetting{Enabled: false, PausedUntil: &future}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ms.IsActive(now); got != tt.want {
				t.Errorf("IsActive = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMealSettings_SetAndGet(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)

	if err := s.SetMealPausedUntil("lunch", &until); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.GetMealPausedUntil("lunch")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || *got != until {
		t.Errorf("lunch paused_until = %v, want %d", got, until)
	}
	// 其它 meal 仍 nil
	if v, _ := s.GetMealPausedUntil("breakfast"); v != nil {
		t.Errorf("breakfast should remain nil, got %v", *v)
	}
}

// TestMealSettings_ClearWithNil 设 nil 取消暂停。
func TestMealSettings_ClearWithNil(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)
	if err := s.SetMealPausedUntil("dinner", &until); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.SetMealPausedUntil("dinner", nil); err != nil {
		t.Fatalf("Set nil: %v", err)
	}
	got, _ := s.GetMealPausedUntil("dinner")
	if got != nil {
		t.Errorf("dinner should be nil after clear, got %d", *got)
	}
}

// TestMealSettings_InvalidMeal 拒绝非 breakfast/lunch/dinner 名字, 防写脏 meal_settings 表。
func TestMealSettings_InvalidMeal(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)
	if err := s.SetMealPausedUntil("brunch", &until); err == nil {
		t.Error("SetMealPausedUntil should reject invalid meal")
	}
	if _, err := s.GetMealPausedUntil("brunch"); err == nil {
		t.Error("GetMealPausedUntil should reject invalid meal")
	}
}

// TestMealPausedUntil_IsActive paused_until > now → 不 active (不发通知)。
// 注: 老 TestMealSetting_IsActive 覆盖了 enabled / paused 矩阵; 这里是历史 case 保留。
func TestMealPausedUntil_IsActive(t *testing.T) {
	now := int64(1700000000)
	future := now + 3600
	past := now - 3600
	tests := []struct {
		name string
		mp   MealSetting
		want bool
	}{
		{"nil paused", MealSetting{Enabled: true}, true},
		{"future paused", MealSetting{Enabled: true, PausedUntil: &future}, false},
		{"past paused (expired)", MealSetting{Enabled: true, PausedUntil: &past}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mp.IsActive(now); got != tt.want {
				t.Errorf("IsActive = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetDailyState_PausedFieldsJoined GetDailyState 应自动 join meal_settings 的 paused_until + enabled。
// 这是 mac UI 单点取数据的关键约定 — DailyState 既混 daily_state 表又混 meal_settings 表。
func TestGetDailyState_PausedFieldsJoined(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)
	if err := s.SetMealPausedUntil("breakfast", &until); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	if err := s.SetMealEnabled("dinner", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	ds, err := s.GetDailyState("2026-05-14")
	if err != nil {
		t.Fatalf("GetDailyState: %v", err)
	}
	if ds.BreakfastPausedUntil == nil || *ds.BreakfastPausedUntil != until {
		t.Errorf("breakfast_paused_until = %v, want %d", ds.BreakfastPausedUntil, until)
	}
	if !ds.BreakfastEnabled || !ds.LunchEnabled {
		t.Errorf("breakfast/lunch enabled 默认 true, 不该被 dinner 禁用影响")
	}
	if ds.DinnerEnabled {
		t.Errorf("dinner_enabled = true, want false (已禁)")
	}
	if ds.LunchPausedUntil != nil {
		t.Errorf("lunch paused_until should be nil")
	}
}
