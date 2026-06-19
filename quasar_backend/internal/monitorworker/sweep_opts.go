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
	// Source identifica origem nos logs/detail (worker, api, api_ping, bootstrap).
	Source string
	// InterfacePhase restringe RunInterfaceSnapshotSweep: InterfacePhaseMikrotik, InterfacePhaseOLT ou vazio (todos).
	InterfacePhase string
}

// ErrSweepBusy indica que já existe uma execução do mesmo tipo em curso (evita SNMP/ICMP sobrepostos).
var ErrSweepBusy = errors.New("monitor: ciclo deste tipo já em execução")

// sweepShouldCollectDevice true quando o worker/bootstrap/API force deve tentar coleta neste ciclo.
func sweepShouldCollectDevice(opts SweepOpts, lastAt time.Time, interval time.Duration) bool {
	if opts.Force {
		return true
	}
	switch strings.TrimSpace(opts.Source) {
	case "worker", "bootstrap":
		return true
	}
	return lastAt.IsZero() || time.Since(lastAt) >= interval
}
