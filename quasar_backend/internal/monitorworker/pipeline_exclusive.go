package monitorworker

import (
	"errors"
	"sync"
)

// monitoringPipelineMu garante uma única sequência SNMP pesada (interfaces → OLT)
// em voo por processo — ping/telemetria/BNG correm em ciclos paralelos dedicados.
var monitoringPipelineMu sync.Mutex

// telemetryCycleMu permite telemetria SNMP em paralelo ao pipeline de interfaces (walks pesados).
var telemetryCycleMu sync.Mutex

// latencyCycleMu permite ICMP/TCP em paralelo ao pipeline SNMP, para não atrasar ping/alertas.
var latencyCycleMu sync.Mutex

// bngCycleMu permite totais BNG (PPPoE online, etc.) em paralelo ao pipeline pesado.
var bngCycleMu sync.Mutex

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

// TryLockBngCycle tenta adquirir o ciclo BNG sem bloquear.
func TryLockBngCycle() bool {
	return bngCycleMu.TryLock()
}

// UnlockBngCycle liberta o ciclo BNG.
func UnlockBngCycle() {
	bngCycleMu.Unlock()
}

// ErrBngCycleBusy indica que já corre outro ciclo BNG.
var ErrBngCycleBusy = errors.New("monitor: ciclo BNG ocupado")

// bngSessionsCycleMu — ciclo dedicado de sessões PPPoE detalhadas (walk completo de logins +
// GET por índice), separado do ciclo de totais BNG por ser bem mais pesado/lento.
var bngSessionsCycleMu sync.Mutex

// TryLockBngSessionsCycle tenta adquirir o ciclo de sessões PPPoE sem bloquear.
func TryLockBngSessionsCycle() bool {
	return bngSessionsCycleMu.TryLock()
}

// UnlockBngSessionsCycle liberta o ciclo de sessões PPPoE.
func UnlockBngSessionsCycle() {
	bngSessionsCycleMu.Unlock()
}

// bngInterfacesCycleMu — ciclo dedicado de snapshot IF-MIB para equipamentos BNG (ver
// TryStartParallelBngInterfaceCycle), separado do ciclo de totais e do de sessões PPPoE.
var bngInterfacesCycleMu sync.Mutex

// TryLockBngInterfacesCycle tenta adquirir o ciclo de interfaces BNG sem bloquear.
func TryLockBngInterfacesCycle() bool {
	return bngInterfacesCycleMu.TryLock()
}

// UnlockBngInterfacesCycle liberta o ciclo de interfaces BNG.
func UnlockBngInterfacesCycle() {
	bngInterfacesCycleMu.Unlock()
}

// retentionCycleMu evita duas purgas de histórico simultâneas (TryRunHistoryRetention).
var retentionCycleMu sync.Mutex

// TryLockRetentionCycle tenta adquirir a purga de histórico sem bloquear.
func TryLockRetentionCycle() bool {
	return retentionCycleMu.TryLock()
}

// UnlockRetentionCycle liberta a purga de histórico.
func UnlockRetentionCycle() {
	retentionCycleMu.Unlock()
}
