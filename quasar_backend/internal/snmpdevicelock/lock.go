package snmpdevicelock

import (
	"sync"

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
