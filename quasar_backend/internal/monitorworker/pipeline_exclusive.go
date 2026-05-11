package monitorworker

import (
	"errors"
	"sync"
)

// monitoringPipelineMu garante uma única sequência de coleta (ping → telemetria → interfaces → PON)
// em voo por processo — evita corridas entre passos e entre worker e POST /monitoring/cycles.
var monitoringPipelineMu sync.Mutex

// TryLockMonitoringPipeline tenta adquirir o pipeline sem bloquear.
func TryLockMonitoringPipeline() bool {
	return monitoringPipelineMu.TryLock()
}

// UnlockMonitoringPipeline liberta o pipeline após TryLockMonitoringPipeline ou LockMonitoringPipeline.
func UnlockMonitoringPipeline() {
	monitoringPipelineMu.Unlock()
}

// LockMonitoringPipeline bloqueia até o pipeline estar livre (bootstrap ao iniciar modo full).
func LockMonitoringPipeline() {
	monitoringPipelineMu.Lock()
}

// ErrPipelineBusy indica que já corre outro pipeline (worker ou bootstrap ou API monolítica).
var ErrPipelineBusy = errors.New("monitor: pipeline de monitorização ocupado")
