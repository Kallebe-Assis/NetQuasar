package monitorworker

import (
	"sync"

	"github.com/google/uuid"
)

// Serializa escritas em device_probe_cache por equipamento (evita corrida entre
// ciclo de latência, ping avulso e actualização SNMP da telemetria no mesmo processo).
var probeRowMu sync.Map // string(uuid) -> *sync.Mutex

// WithDeviceProbeRowLock executa fn com exclusão mútua por device_id em toda a instância do backend.
func WithDeviceProbeRowLock(deviceID uuid.UUID, fn func()) {
	v, _ := probeRowMu.LoadOrStore(deviceID.String(), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	fn()
}
