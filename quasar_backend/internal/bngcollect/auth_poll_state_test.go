package bngcollect

import (
	"testing"
	"time"
)

func TestAuthPollState_shouldWalkSessions(t *testing.T) {
	st := newAuthPollState()
	now := time.Now()

	if !st.shouldWalkSessions(100, true, now) {
		t.Fatal("expected walk before bootstrap")
	}
	st.bootstrapped = true
	st.lastSessionWalkAt = now
	st.notePPPoECount(100, true)

	if st.shouldWalkSessions(100, true, now) {
		t.Fatal("expected no walk within interval with stable count")
	}
	if !st.shouldWalkSessions(105, true, now) {
		t.Fatal("expected walk when PPPoE count increases")
	}
	st.lastPPPoECount = 105
	st.lastPPPoECountOK = true
	st.lastSessionWalkAt = now.Add(-authSessionWalkInterval - time.Second)
	if !st.shouldWalkSessions(105, true, now) {
		t.Fatal("expected periodic walk after interval")
	}
}

func TestAuthPollState_failIndexThreshold(t *testing.T) {
	st := newAuthPollState()
	if st.failIndexThreshold() != 0 {
		t.Fatalf("expected 0, got %d", st.failIndexThreshold())
	}
	st.lastMaxFailIndex = 8758600
	if got := st.failIndexThreshold(); got != 8758600-int64(authFailIndexOverlap) {
		t.Fatalf("unexpected threshold: %d", got)
	}
}

func TestAuthPollState_updateMaxFailIndex(t *testing.T) {
	st := newAuthPollState()
	st.updateMaxFailIndex([]string{"8758600", "8758598", "100"})
	if st.lastMaxFailIndex != 8758600 {
		t.Fatalf("expected 8758600, got %d", st.lastMaxFailIndex)
	}
}
