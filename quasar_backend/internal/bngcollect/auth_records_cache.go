package bngcollect

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

const authCacheMinInterval = 1800 * time.Millisecond

type authCacheEntry struct {
	mu         sync.Mutex
	state      *authPollState
	updatedAt  time.Time
	refreshing bool
}

var bngAuthRecordsCache sync.Map // deviceID string -> *authCacheEntry

// FetchRecentBngAuthRecordsCached mantém buffer circular de eventos AAA em tempo real.
func FetchRecentBngAuthRecordsCached(ctx context.Context, deviceID uuid.UUID, host, community string, limit int, stripSuffix string) []AuthAttemptLog {
	if limit <= 0 || limit > maxAuthRecordsGeneral {
		limit = maxAuthRecordsGeneral
	}
	key := deviceID.String()
	raw, _ := bngAuthRecordsCache.LoadOrStore(key, &authCacheEntry{state: newAuthPollState()})
	entry := raw.(*authCacheEntry)

	entry.mu.Lock()
	if time.Since(entry.updatedAt) < authCacheMinInterval && len(entry.state.records()) > 0 {
		out := entry.state.records()
		entry.mu.Unlock()
		return out
	}
	if entry.refreshing && len(entry.state.records()) > 0 {
		out := entry.state.records()
		entry.mu.Unlock()
		return out
	}
	entry.refreshing = true
	st := entry.state
	entry.mu.Unlock()

	if len(st.records()) == 0 {
		st.lastMaxFailIndex = 0
		seed := fetchRecentBngAuthRecordsFast(ctx, host, community, limit, stripSuffix)
		st.seed(seed, limit)
	}
	fresh := pollAuthEvents(ctx, host, community, st, stripSuffix)
	st.mergeFresh(fresh, limit)

	entry.mu.Lock()
	entry.updatedAt = time.Now()
	entry.refreshing = false
	out := entry.state.records()
	entry.mu.Unlock()
	return out
}
