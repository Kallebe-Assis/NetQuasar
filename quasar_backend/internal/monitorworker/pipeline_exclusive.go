package monitorworker

import (
	"errors"
	"sync"
)

// monitoringPipelineMu garante uma única sequência SNMP (telemetria → interfaces → PON)
// em voo por processo — evita corridas entre passos pesados e POST /monitoring/cycles.
var monitoringPipelineMu sync.Mutex

// latencyCycleMu permite ICMP/TCP em paralelo ao pipeline SNMP, para não atrasar ping/alertas.
var latencyCycleMu sync.Mutex

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

// TryLockLatencyCycle tenta adquirir o ciclo ICMP/TCP sem bloquear.
func TryLockLatencyCycle() bool {
	return latencyCycleMu.TryLock()
}

// UnlockLatencyCycle liberta o ciclo ICMP/TCP.
func UnlockLatencyCycle() {
	latencyCycleMu.Unlock()
}

// LockLatencyCycle bloqueia até o ciclo ICMP/TCP estar livre (bootstrap monolítico).
func LockLatencyCycle() {
	latencyCycleMu.Lock()
}

// ErrLatencyCycleBusy indica que já corre outro ciclo de latência.
var ErrLatencyCycleBusy = errors.New("monitor: ciclo de latência ocupado")
