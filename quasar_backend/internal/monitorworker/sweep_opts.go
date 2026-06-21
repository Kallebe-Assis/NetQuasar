package monitorworker

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Fases opcionais para snapshots de interface (pipeline ordenado MikroTik antes de OLT).
const (
	InterfacePhaseAll      = ""
	InterfacePhaseMikrotik = "mikrotik"
	InterfacePhaseOLT      = "olt"
)

// SweepOpts parametriza uma execução de ciclo partilhada entre o worker em background e as rotas HTTP.
type SweepOpts struct {
	// DeviceID, se não-nil, limita ao equipamento (útil para OLT Mikrotik / ping avulso).
	DeviceID *uuid.UUID
	// Force ignora apenas o período desde o último ciclo global daquele tipo; mantém filtros por equipamento (ex.: última telemetria).
	Force bool
	// Source identifica origem nos logs/detail (worker, api, api_ping, bootstrap, pipeline).
	Source string
	// InterfacePhase restringe RunInterfaceSnapshotSweep: InterfacePhaseMikrotik, InterfacePhaseOLT ou vazio (todos).
	InterfacePhase string
	// PipelineStep passo do pipeline configurado (scope/options).
	PipelineStep *PipelineStep
	// ScopedDevices lista pré-filtrada (pipeline); quando definida, os sweeps usam-na em vez de loadPingableDevices.
	ScopedDevices []pingableDeviceRow
	// SkipPingInPipeline quando true, passos «ping» do pipeline são ignorados (correm em paralelo via ping_seconds).
	SkipPingInPipeline bool
}

// ErrSweepBusy indica que já existe uma execução do mesmo tipo em curso (evita SNMP/ICMP sobrepostos).
var ErrSweepBusy = errors.New("monitor: ciclo deste tipo já em execução")

// sweepShouldCollectDevice true quando este equipamento deve ser incluído neste ciclo.
func sweepShouldCollectDevice(opts SweepOpts, lastAt time.Time, interval time.Duration) bool {
	if opts.Force {
		return true
	}
	if strings.TrimSpace(opts.Source) == "bootstrap" {
		return true
	}
	return lastAt.IsZero() || time.Since(lastAt) >= interval
}
