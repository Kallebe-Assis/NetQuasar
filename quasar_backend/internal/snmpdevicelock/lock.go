package snmpdevicelock

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// locks serializa operações SNMP pesadas por equipamento (API HTTP, jobs em
// background, ciclo do monitor_worker) para o mesmo device_id.
var locks sync.Map // uuid string -> *sync.Mutex

// Acquire bloqueia até obter exclusividade para o equipamento; chame a função
// retornada para libertar (normalmente com defer imediatamente após Acquire).
func Acquire(deviceID uuid.UUID) (unlock func()) {
	key := deviceID.String()
	v, _ := locks.LoadOrStore(key, new(sync.Mutex))
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// TryAcquire tenta obter o lock sem esperar para sempre.
// Preferir no ciclo paralelo de telemetria para não prender o ciclo inteiro.
func TryAcquire(ctx context.Context, deviceID uuid.UUID, wait time.Duration) (unlock func(), ok bool) {
	if wait <= 0 {
		wait = 3 * time.Second
	}
	key := deviceID.String()
	v, _ := locks.LoadOrStore(key, new(sync.Mutex))
	mu := v.(*sync.Mutex)

	deadline := time.Now().Add(wait)
	for {
		if mu.TryLock() {
			return mu.Unlock, true
		}
		if ctx != nil && ctx.Err() != nil {
			return nil, false
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(40 * time.Millisecond):
		}
	}
}
