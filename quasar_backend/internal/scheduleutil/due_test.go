package scheduleutil

import (
	"testing"
	"time"
)

func TestDailyWeeklyDue_RescheduleLaterSameDay(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	scheduledOld := time.Date(2026, 5, 20, 8, 0, 0, 0, loc)
	lastAt := scheduledOld.Add(2 * time.Minute)
	runKey := "2026-05-20"
	key := runKey
	now := time.Date(2026, 5, 20, 14, 35, 0, 0, loc)
	_, due := DailyWeeklyDue(true, "daily", "America/Sao_Paulo", "14:30", nil, &key, &lastAt, false, now)
	if !due {
		t.Fatal("expected due after moving schedule later same day")
	}
}

func TestDailyWeeklyDue_AlreadyRanAfterScheduled(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("UTC")
	scheduled := time.Date(2026, 5, 20, 10, 0, 0, 0, loc)
	lastAt := scheduled.Add(time.Minute)
	runKey := "2026-05-20"
	key := runKey
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, loc)
	_, due := DailyWeeklyDue(true, "daily", "UTC", "10:00", nil, &key, &lastAt, false, now)
	if due {
		t.Fatal("expected not due when last run was after scheduled time")
	}
}
