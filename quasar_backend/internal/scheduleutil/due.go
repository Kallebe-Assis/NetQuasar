package scheduleutil

import (
	"strconv"
	"strings"
	"time"
)

func ParseHHMM(hhmm string) (hour, min int) {
	parts := strings.Split(strings.TrimSpace(hhmm), ":")
	if len(parts) >= 1 {
		hour, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	if len(parts) >= 2 {
		min, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	if hour < 0 || hour > 23 {
		hour = 8
	}
	if min < 0 || min > 59 {
		min = 0
	}
	return hour, min
}

func EffectiveDOM(dom, year, month int) int {
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
	if dom > last {
		return last
	}
	if dom < 1 {
		return 1
	}
	return dom
}

func LoadTZ(tz string) *time.Location {
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

// MonthlyDue indica se já passou o agendamento mensal (YYYY-MM) e ainda não houve execução
// bem-sucedida neste período após o horário agendado do mês.
func MonthlyDue(enabled bool, tz, hhmm string, dom int, lastRunKey *string, lastRunAt *time.Time, running bool, now time.Time) (period string, due bool) {
	if !enabled || running {
		return "", false
	}
	loc := LoadTZ(tz)
	now = now.In(loc)
	period = now.Format("2006-01")
	day := EffectiveDOM(dom, now.Year(), int(now.Month()))
	hour, min := ParseHHMM(hhmm)
	scheduled := time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, loc)
	if now.Before(scheduled) {
		return period, false
	}
	if lastRunKey != nil && strings.TrimSpace(*lastRunKey) == period {
		if lastRunAt != nil && !lastRunAt.In(loc).Before(scheduled) {
			return period, false
		}
		if lastRunAt == nil {
			return period, false
		}
	}
	return period, true
}

// DailyWeeklyDue runKey = YYYY-MM-DD. frequency: daily | weekly (dayOfWeek 0=Sunday).
func DailyWeeklyDue(enabled bool, frequency, tz, hhmm string, dayOfWeek *int, lastRunKey *string, lastRunAt *time.Time, running bool, now time.Time) (runKey string, due bool) {
	if !enabled || running {
		return "", false
	}
	loc := LoadTZ(tz)
	now = now.In(loc)
	runKey = now.Format("2006-01-02")
	if strings.EqualFold(strings.TrimSpace(frequency), "weekly") {
		dow := 1
		if dayOfWeek != nil {
			dow = *dayOfWeek
		}
		if int(now.Weekday()) != dow {
			return runKey, false
		}
	}
	hour, min := ParseHHMM(hhmm)
	scheduled := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
	if now.Before(scheduled) {
		return runKey, false
	}
	if lastRunKey != nil && strings.TrimSpace(*lastRunKey) == runKey {
		if lastRunAt != nil && !lastRunAt.In(loc).Before(scheduled) {
			return runKey, false
		}
		if lastRunAt == nil {
			return runKey, false
		}
	}
	return runKey, true
}
