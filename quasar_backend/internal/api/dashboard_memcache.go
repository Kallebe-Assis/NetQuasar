package api

import (
	"sync"
	"time"
)

type dashMemEntry struct {
	body      []byte
	expiresAt time.Time
}

var (
	dashMemMu    sync.RWMutex
	dashMemStore = map[string]dashMemEntry{}
)

const dashboardMemTTL = 5 * time.Minute

func dashboardMemGet(key string) []byte {
	dashMemMu.RLock()
	defer dashMemMu.RUnlock()
	e, ok := dashMemStore[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil
	}
	out := make([]byte, len(e.body))
	copy(out, e.body)
	return out
}

func dashboardMemSet(key string, body []byte) {
	if len(body) == 0 {
		return
	}
	cp := make([]byte, len(body))
	copy(cp, body)
	dashMemMu.Lock()
	dashMemStore[key] = dashMemEntry{body: cp, expiresAt: time.Now().Add(dashboardMemTTL)}
	// Evitar crescimento indefinido se a chave variar muito.
	if len(dashMemStore) > 32 {
		now := time.Now()
		for k, v := range dashMemStore {
			if now.After(v.expiresAt) {
				delete(dashMemStore, k)
			}
		}
	}
	dashMemMu.Unlock()
}

func dashboardMemDeletePrefix(prefix string) {
	dashMemMu.Lock()
	for k := range dashMemStore {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(dashMemStore, k)
		}
	}
	dashMemMu.Unlock()
}
