package bngcollect

import (
	"sort"
	"time"
)

const authSessionWalkInterval = 12 * time.Second

type authPollState struct {
	ring               []AuthAttemptLog
	seen               map[string]struct{}
	prevOnline         map[string]int // idx SNMP -> segundos online
	lastMaxFailIndex   int64
	lastPPPoECount     int
	lastPPPoECountOK   bool
	lastSessionWalkAt  time.Time
	bootstrapped       bool
}

func newAuthPollState() *authPollState {
	return &authPollState{
		seen:       make(map[string]struct{}),
		prevOnline: make(map[string]int),
	}
}

func (st *authPollState) shouldWalkSessions(pppoeCount int, pppoeOK bool, now time.Time) bool {
	if !st.bootstrapped {
		return true
	}
	if st.lastSessionWalkAt.IsZero() || now.Sub(st.lastSessionWalkAt) >= authSessionWalkInterval {
		return true
	}
	if pppoeOK && st.lastPPPoECountOK && pppoeCount > st.lastPPPoECount {
		return true
	}
	return false
}

func (st *authPollState) notePPPoECount(count int, ok bool) {
	if ok {
		st.lastPPPoECount = count
		st.lastPPPoECountOK = true
	}
}

func (st *authPollState) noteSessionWalk(at time.Time) {
	st.lastSessionWalkAt = at
}

func (st *authPollState) failIndexThreshold() int64 {
	if st.lastMaxFailIndex <= authFailIndexOverlap {
		return 0
	}
	return st.lastMaxFailIndex - authFailIndexOverlap
}

func (st *authPollState) updateMaxFailIndex(indices []string) {
	for _, idx := range indices {
		if n := authIndexNum(idx); n > st.lastMaxFailIndex {
			st.lastMaxFailIndex = n
		}
	}
}

func (st *authPollState) markSeen(rec AuthAttemptLog) bool {
	key := authEventDedupeKey(rec)
	if _, ok := st.seen[key]; ok {
		return false
	}
	st.seen[key] = struct{}{}
	return true
}

func (st *authPollState) mergeFresh(fresh []AuthAttemptLog, limit int) {
	if limit <= 0 {
		limit = maxAuthRecordsGeneral
	}
	for _, rec := range fresh {
		st.ring = append([]AuthAttemptLog{rec}, st.ring...)
	}
	st.ring = sortLimitAuthRecords(st.ring, limit)
	st.pruneSeen()
}

func (st *authPollState) seed(initial []AuthAttemptLog, limit int) {
	if limit <= 0 {
		limit = maxAuthRecordsGeneral
	}
	sort.Slice(initial, func(i, j int) bool {
		ti, oki := parseAAARecordTime(initial[i].Time)
		tj, okj := parseAAARecordTime(initial[j].Time)
		if oki && okj {
			return ti.After(tj)
		}
		return authIndexNum(initial[i].Seq) > authIndexNum(initial[j].Seq)
	})
	if len(initial) > limit {
		initial = initial[:limit]
	}
	st.ring = initial
	for _, rec := range st.ring {
		st.seen[authEventDedupeKey(rec)] = struct{}{}
		if rec.Kind == "failure" && rec.Seq != "" {
			if n := authIndexNum(rec.Seq); n > st.lastMaxFailIndex {
				st.lastMaxFailIndex = n
			}
		}
	}
	st.bootstrapped = true
}

func (st *authPollState) records() []AuthAttemptLog {
	if len(st.ring) == 0 {
		return nil
	}
	out := make([]AuthAttemptLog, len(st.ring))
	copy(out, st.ring)
	return out
}

func (st *authPollState) pruneSeen() {
	if len(st.seen) <= maxAuthRecordsGeneral*4 {
		return
	}
	next := make(map[string]struct{}, len(st.ring)+8)
	for _, rec := range st.ring {
		next[authEventDedupeKey(rec)] = struct{}{}
	}
	st.seen = next
}
