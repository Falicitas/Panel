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
func TestMealPausedUntil_IsActive(t *testing.T) {
	now := int64(1700000000)
	future := now + 3600
	past := now - 3600
	tests := []struct {
		name string
		mp   MealPausedUntil
		want bool
	}{
		{"nil paused", MealPausedUntil{}, true},
		{"future paused", MealPausedUntil{PausedUntil: &future}, false},
		{"past paused (expired)", MealPausedUntil{PausedUntil: &past}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mp.IsActive(now); got != tt.want {
				t.Errorf("IsActive = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetDailyState_PausedFieldsJoined GetDailyState 应自动 join meal_settings 的 paused_until。
// 这是 mac UI 单点取数据的关键约定 — DailyState 既混 daily_state 表又混 meal_settings 表。
func TestGetDailyState_PausedFieldsJoined(t *testing.T) {
	s := openTestStore(t)
	until := int64(1700000000)
	if err := s.SetMealPausedUntil("breakfast", &until); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ds, err := s.GetDailyState("2026-05-14")
	if err != nil {
		t.Fatalf("GetDailyState: %v", err)
	}
	if ds.BreakfastPausedUntil == nil || *ds.BreakfastPausedUntil != until {
		t.Errorf("breakfast_paused_until = %v, want %d", ds.BreakfastPausedUntil, until)
	}
	if ds.LunchPausedUntil != nil {
		t.Errorf("lunch should be nil, got %v", *ds.LunchPausedUntil)
	}
	if ds.DinnerPausedUntil != nil {
		t.Errorf("dinner should be nil, got %v", *ds.DinnerPausedUntil)
	}
}
