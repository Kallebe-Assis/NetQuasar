package monitorworker

import (
	"errors"
	"sync"
)

// monitoringPipelineMu garante uma única sequência SNMP (telemetria → interfaces → PON)
// em voo por processo — evita corridas entre passos pesados e POST /monitoring/cycles.
var monitoringPipelineMu sync.Mutex

// telemetryCycleMu permite telemetria SNMP em paralelo ao pipeline de interfaces (walks pesados).
var telemetryCycleMu sync.Mutex

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

// oltPonCycleMu permite coleta ONU/PON em paralelo ao pipeline SNMP pesado (telemetria/interfaces).
var oltPonCycleMu sync.Mutex

// TryLockOltPonCycle tenta adquirir o ciclo ONU/PON sem bloquear.
func TryLockOltPonCycle() bool {
	return oltPonCycleMu.TryLock()
}

// UnlockOltPonCycle liberta o ciclo ONU/PON.
func UnlockOltPonCycle() {
	oltPonCycleMu.Unlock()
}

// ErrOltPonCycleBusy indica que já corre outro ciclo ONU/PON.
var ErrOltPonCycleBusy = errors.New("monitor: ciclo ONU/PON ocupado")

// TryLockTelemetryCycle tenta adquirir o ciclo de telemetria SNMP sem bloquear.
func TryLockTelemetryCycle() bool {
	return telemetryCycleMu.TryLock()
}

// UnlockTelemetryCycle liberta o ciclo de telemetria SNMP.
func UnlockTelemetryCycle() {
	telemetryCycleMu.Unlock()
}

// ErrTelemetryCycleBusy indica que já corre outro ciclo de telemetria SNMP.
var ErrTelemetryCycleBusy = errors.New("monitor: ciclo de telemetria ocupado")

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
