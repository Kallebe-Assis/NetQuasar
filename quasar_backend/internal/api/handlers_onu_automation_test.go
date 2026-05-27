package api

import (
	"testing"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
)

func TestOnuReportDue_MonthlyAfterScheduled(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 5, 18, 9, 0, 0, 0, loc)
	cfg := onuAutomationConfig{
		Enabled:    true,
		Mode:       "monthly",
		DayOfMonth: 18,
		TimeHHMM:   "08:00",
		Timezone:   "America/Sao_Paulo",
	}
	period, due := onuReportDue(cfg, now)
	if !due || period != "2026-05" {
		t.Fatalf("expected due 2026-05, got due=%v period=%q", due, period)
	}
}

func TestOnuReportDue_BeforeScheduledTime(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 5, 18, 7, 30, 0, 0, loc)
	cfg := onuAutomationConfig{
		Enabled: true, Mode: "monthly", DayOfMonth: 18, TimeHHMM: "08:00", Timezone: "America/Sao_Paulo",
	}
	_, due := onuReportDue(cfg, now)
	if due {
		t.Fatal("expected not due before scheduled time")
	}
}

func TestOnuReportDue_AlreadyRanThisMonth(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, loc)
	p := "2026-05"
	cfg := onuAutomationConfig{
		Enabled: true, Mode: "monthly", DayOfMonth: 18, TimeHHMM: "08:00", Timezone: "America/Sao_Paulo",
		LastRunPeriod: &p,
	}
	_, due := onuReportDue(cfg, now)
	if due {
		t.Fatal("expected not due when last_run_period matches month")
	}
}

func TestScheduleutilMonthlyDue_EndOfMonthDOM(t *testing.T) {
	t.Parallel()
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2026, 2, 28, 12, 0, 0, 0, loc)
	period, due := scheduleutil.MonthlyDue(true, "UTC", "08:00", 31, nil, nil, false, now)
	if !due || period != "2026-02" {
		t.Fatalf("feb 31 should schedule on 28th: due=%v period=%q", due, period)
	}
}
