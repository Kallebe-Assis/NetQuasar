package monitorworker

import (
	"errors"

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
