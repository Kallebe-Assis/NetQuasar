package monitorworker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StepKindPing               = "ping"
	StepKindTelemetry          = "telemetry"
	StepKindBng                = "bng"
	StepKindOltOnu             = "olt_onu"
	StepKindMikrotik           = "mikrotik"
	StepKindInterfacesOLT      = "interfaces_olt"
	StepKindInterfacesMikrotik = "interfaces_mikrotik"
)

// StepScope filtro de equipamentos para um passo do pipeline.
type StepScope struct {
	Target    string   `json:"target"`
	Category  string   `json:"category,omitempty"`
	DeviceIDs []string `json:"device_ids,omitempty"`
}

// StepOptions opções por tipo de passo (telemetria parcial, ONU simplificada, MikroTik, etc.).
type StepOptions struct {
	TelemetryFields []string `json:"telemetry_fields,omitempty"`
	OltOnuMode      string   `json:"olt_onu_mode,omitempty"`
	MikrotikMode    string   `json:"mikrotik_mode,omitempty"`
	BngMode         string   `json:"bng_mode,omitempty"`
}

// PipelineStep um passo configurável na ordem de monitoramento.
type PipelineStep struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	Enabled bool        `json:"enabled"`
	Scope   StepScope   `json:"scope"`
	Options StepOptions `json:"options"`
}

// DefaultPipelineSteps ordem inicial equivalente ao pipeline legado.
func DefaultPipelineSteps() []PipelineStep {
	return []PipelineStep{
		{ID: "ping-all", Kind: StepKindPing, Enabled: true, Scope: StepScope{Target: "all"}},
		{ID: "telemetry-all", Kind: StepKindTelemetry, Enabled: true, Scope: StepScope{Target: "all"}},
		{ID: "bng-subscribers", Kind: StepKindBng, Enabled: true, Scope: StepScope{Target: "category", Category: "bng"}, Options: StepOptions{BngMode: "totals"}},
		{ID: "mikrotik-if", Kind: StepKindMikrotik, Enabled: true, Scope: StepScope{Target: "category", Category: "mikrotik"}, Options: StepOptions{MikrotikMode: "full"}},
		{ID: "olt-if", Kind: StepKindInterfacesOLT, Enabled: true, Scope: StepScope{Target: "category", Category: "olt"}},
		{ID: "olt-onu", Kind: StepKindOltOnu, Enabled: true, Scope: StepScope{Target: "category", Category: "olt"}, Options: StepOptions{OltOnuMode: "full"}},
	}
}

func ParsePipelineSteps(raw []byte) []PipelineStep {
	if len(raw) == 0 || string(raw) == "[]" || string(raw) == "null" {
		return nil
	}
	var steps []PipelineStep
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil
	}
	return NormalizePipelineSteps(steps)
}

func NormalizePipelineSteps(steps []PipelineStep) []PipelineStep {
	out := make([]PipelineStep, 0, len(steps))
	seen := map[string]struct{}{}
	for i, s := range steps {
		kind := strings.ToLower(strings.TrimSpace(s.Kind))
		if kind == "" {
			continue
		}
		s.Kind = kind
		if strings.TrimSpace(s.ID) == "" {
			s.ID = kind + "-" + strconv.Itoa(i+1)
		}
		if _, ok := seen[s.ID]; ok {
			s.ID = s.ID + "-" + strconv.Itoa(len(out)+1)
		}
		seen[s.ID] = struct{}{}
		if strings.TrimSpace(s.Scope.Target) == "" {
			s.Scope.Target = "all"
		}
		s.Scope.Target = strings.ToLower(strings.TrimSpace(s.Scope.Target))
		s.Scope.Category = strings.TrimSpace(s.Scope.Category)
		out = append(out, s)
	}
	return out
}

func EnabledPipelineSteps(steps []PipelineStep) []PipelineStep {
	out := make([]PipelineStep, 0, len(steps))
	for _, s := range steps {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out
}

// FirstEnabledPingStep devolve o primeiro passo ping activo (scope para ping paralelo).
func FirstEnabledPingStep(steps []PipelineStep) *PipelineStep {
	for i := range steps {
		if steps[i].Enabled && steps[i].Kind == StepKindPing {
			return &steps[i]
		}
	}
	return nil
}

func LoadPipelineSteps(ctx context.Context, pool *pgxpool.Pool) ([]PipelineStep, error) {
	if pool == nil {
		return DefaultPipelineSteps(), nil
	}
	var raw []byte
	err := pool.QueryRow(ctx, `SELECT coalesce(pipeline_steps::text, '[]') FROM monitoring_intervals WHERE id=1`).Scan(&raw)
	if err != nil {
		return DefaultPipelineSteps(), err
	}
	steps := ParsePipelineSteps(raw)
	if len(steps) == 0 {
		return DefaultPipelineSteps(), nil
	}
	return ensureBngPipelineStep(steps), nil
}

func hasPipelineKind(steps []PipelineStep, kind string) bool {
	for _, s := range steps {
		if s.Kind == kind {
			return true
		}
	}
	return false
}

// EnsureBngPipelineStep expõe inserção do passo BNG para API/UI.
func EnsureBngPipelineStep(steps []PipelineStep) []PipelineStep {
	return ensureBngPipelineStep(steps)
}

// ensureBngPipelineStep insere passo BNG após telemetria quando ausente (instalações anteriores à migração 070).
func ensureBngPipelineStep(steps []PipelineStep) []PipelineStep {
	if hasPipelineKind(steps, StepKindBng) {
		return steps
	}
	bng := PipelineStep{
		ID: "bng-subscribers", Kind: StepKindBng, Enabled: true,
		Scope: StepScope{Target: "category", Category: "bng"},
		Options: StepOptions{BngMode: "totals"},
	}
	for i, s := range steps {
		if s.Kind == StepKindTelemetry {
			out := make([]PipelineStep, 0, len(steps)+1)
			out = append(out, steps[:i+1]...)
			out = append(out, bng)
			out = append(out, steps[i+1:]...)
			return out
		}
	}
	return append([]PipelineStep{bng}, steps...)
}

func pipelineStepLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case StepKindPing:
		return "Ping (ICMP/TCP)"
	case StepKindTelemetry:
		return "Telemetria SNMP"
	case StepKindBng:
		return "BNG (logins / saúde)"
	case StepKindOltOnu:
		return "Coleta ONUs (OLT)"
	case StepKindMikrotik:
		return "MikroTik"
	case StepKindInterfacesOLT:
		return "Interfaces SNMP (OLT)"
	case StepKindInterfacesMikrotik:
		return "Interfaces SNMP (MikroTik)"
	default:
		return kind
	}
}

func scopeDeviceUUIDs(scope StepScope) []uuid.UUID {
	var out []uuid.UUID
	for _, s := range scope.DeviceIDs {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err == nil {
			out = append(out, id)
		}
	}
	return out
}

func isBngDevice(row pingableDeviceRow) bool {
	return row.bngEnabled || strings.EqualFold(strings.TrimSpace(row.category), "bng")
}

func deviceMatchesScope(row pingableDeviceRow, scope StepScope) bool {
	target := strings.ToLower(strings.TrimSpace(scope.Target))
	if target == "" || target == "all" {
		return true
	}
	if target == "category" {
		cat := strings.ToLower(strings.TrimSpace(scope.Category))
		if cat == "" || cat == "olt" {
			return strings.EqualFold(strings.TrimSpace(row.category), "olt")
		}
		if cat == "bng" {
			return isBngDevice(row)
		}
		return strings.EqualFold(strings.TrimSpace(row.category), scope.Category)
	}
	if target == "devices" {
		ids := scopeDeviceUUIDs(scope)
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			if id == row.id {
				return true
			}
		}
		return false
	}
	return true
}

func filterDevicesByScope(rows []pingableDeviceRow, scope StepScope) []pingableDeviceRow {
	if strings.TrimSpace(scope.Target) == "" || strings.EqualFold(strings.TrimSpace(scope.Target), "all") {
		return rows
	}
	out := make([]pingableDeviceRow, 0, len(rows))
	for _, r := range rows {
		if deviceMatchesScope(r, scope) {
			out = append(out, r)
		}
	}
	return out
}

func loadDevicesForPipelineStep(ctx context.Context, pool *pgxpool.Pool, step PipelineStep, only *uuid.UUID) ([]pingableDeviceRow, error) {
	var base []pingableDeviceRow
	var err error
	switch step.Kind {
	case StepKindOltOnu:
		base, err = loadOltDevicesForCollect(ctx, pool, only)
	case StepKindBng:
		base, err = loadBngDevicesForCollect(ctx, pool, only)
	default:
		base, err = loadPingableDevices(ctx, pool, only)
	}
	if err != nil {
		return nil, err
	}
	if only != nil {
		return base, nil
	}
	// Coleta ONU: por defeito TODAS as OLTs; filtro só se utilizador escolheu equipamentos específicos.
	if step.Kind == StepKindOltOnu && !strings.EqualFold(strings.TrimSpace(step.Scope.Target), "devices") {
		return base, nil
	}
	// BNG: por defeito todos os BNG com coleta activa; filtro só com alvo «equipamentos».
	if step.Kind == StepKindBng && !strings.EqualFold(strings.TrimSpace(step.Scope.Target), "devices") {
		return base, nil
	}
	return filterDevicesByScope(base, step.Scope), nil
}
