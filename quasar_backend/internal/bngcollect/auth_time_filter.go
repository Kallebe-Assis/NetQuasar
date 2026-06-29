package bngcollect

import (
	"sort"
	"time"
)

const (
	authMaxAge        = 45 * time.Minute
	authFutureSkew    = 2 * time.Minute
	authOfflineMaxAge = 6 * time.Hour
)

func authTimeAcceptable(t time.Time, maxAge time.Duration) bool {
	if t.IsZero() {
		return false
	}
	now := time.Now()
	if t.After(now.Add(authFutureSkew)) {
		return false
	}
	if maxAge <= 0 {
		maxAge = authMaxAge
	}
	if t.Before(now.Add(-maxAge)) {
		return false
	}
	// DateAndTime só com data (00:00:00) — rejeita dia futuro (ex.: 7.234.6.29 → 29/Jun).
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if t.After(today) {
			return false
		}
	}
	return true
}

func authRecordSortTime(rec AuthAttemptLog) (time.Time, bool) {
	if t, ok := parseAAARecordTime(rec.Time); ok {
		return t, true
	}
	if rec.Kind == "failure" && rec.Seq != "" {
		// Falhas sem hora SNMP válida: ordenar pelo índice/seq (monotónico).
		return time.Unix(authIndexNum(rec.Seq), 0), true
	}
	return time.Time{}, false
}

// sortLimitAuthRecords ordena e limita sem descartar por filtro temporal (buffer em tempo real).
func sortLimitAuthRecords(records []AuthAttemptLog, limit int) []AuthAttemptLog {
	if len(records) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = maxAuthRecordsGeneral
	}
	now := time.Now()
	type scored struct {
		rec AuthAttemptLog
		at  time.Time
		ok  bool
	}
	scoredRows := make([]scored, 0, len(records))
	for _, r := range records {
		at, ok := authRecordSortTime(r)
		if ok && at.After(now.Add(authFutureSkew)) {
			continue
		}
		scoredRows = append(scoredRows, scored{rec: r, at: at, ok: ok})
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		a, b := scoredRows[i], scoredRows[j]
		if a.ok && b.ok {
			return a.at.After(b.at)
		}
		if a.ok != b.ok {
			return a.ok
		}
		return authIndexNum(a.rec.Seq) > authIndexNum(b.rec.Seq)
	})
	if len(scoredRows) > limit {
		scoredRows = scoredRows[:limit]
	}
	out := make([]AuthAttemptLog, len(scoredRows))
	for i, s := range scoredRows {
		out[i] = s.rec
	}
	return out
}

func filterSortLimitAuthRecords(records []AuthAttemptLog, limit int) []AuthAttemptLog {
	type scored struct {
		rec AuthAttemptLog
		at  time.Time
	}
	valid := make([]scored, 0, len(records))
	for _, r := range records {
		maxAge := authMaxAge
		if r.Kind == "success" {
			maxAge = authOfflineMaxAge
		}
		t, ok := authRecordSortTime(r)
		if !ok {
			continue
		}
		if !authTimeAcceptable(t, maxAge) {
			continue
		}
		valid = append(valid, scored{rec: r, at: t})
	}
	if len(valid) == 0 {
		return nil
	}
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].at.After(valid[j].at)
	})
	if limit <= 0 {
		limit = maxAuthRecordsGeneral
	}
	if len(valid) > limit {
		valid = valid[:limit]
	}
	out := make([]AuthAttemptLog, len(valid))
	for i, s := range valid {
		out[i] = s.rec
	}
	return out
}
