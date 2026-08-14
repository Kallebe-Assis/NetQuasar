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

func TestDailyWeeklyDueOnDays_OnlySelectedWeekdays(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, loc)
	today := int(now.Weekday())
	other := (today + 1) % 7
	_, due := DailyWeeklyDueOnDays(true, "custom", "UTC", "10:00", nil, []int{other}, nil, nil, false, now)
	if due {
		t.Fatal("should not run on a day that is not selected")
	}
	_, due = DailyWeeklyDueOnDays(true, "custom", "UTC", "10:00", nil, []int{today}, nil, nil, false, now)
	if !due {
		t.Fatal("should run when today is selected")
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
