package monitorworker

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// OltManualRefresher executa a mesma coleta OLT que POST /devices/{id}/refresh (refresh manual).
type OltManualRefresher interface {
	RefreshOLT(ctx context.Context, deviceID uuid.UUID, source string) OltCollectOutcome
}

var (
	oltManualRefresher   OltManualRefresher
	oltManualRefresherMu sync.RWMutex
)

// SetOltManualRefresher regista o refresher da API (chamado na inicialização do servidor).
func SetOltManualRefresher(r OltManualRefresher) {
	oltManualRefresherMu.Lock()
	oltManualRefresher = r
	oltManualRefresherMu.Unlock()
}

func tryOltManualRefresh(ctx context.Context, deviceID uuid.UUID, source string) (OltCollectOutcome, bool) {
	oltManualRefresherMu.RLock()
	r := oltManualRefresher
	oltManualRefresherMu.RUnlock()
	if r == nil {
		return OltCollectOutcome{}, false
	}
	return r.RefreshOLT(ctx, deviceID, source), true
}
