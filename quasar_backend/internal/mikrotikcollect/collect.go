package mikrotikcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const maxWalkVarsStored = 800

// SkippedField métrica não coletada e motivo.
type SkippedField struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Reason string `json:"reason"`
}

// WalkSummary resumo de um snmpwalk.
type WalkSummary struct {
	RowCount  int               `json:"row_count"`
	Truncated bool              `json:"truncated"`
	Vars      []probing.SNMPVar `json:"vars,omitempty"`
}

// FieldResult resultado por métrica.
type FieldResult struct {
	Key          string           `json:"key"`
	Label        string           `json:"label"`
	OK           bool             `json:"ok"`
	CollectMode  string           `json:"collect_mode"`
	OID          string           `json:"oid"`
	Value        any              `json:"value,omitempty"`
	ValueDivisor int              `json:"value_divisor,omitempty"`
	Walk         *WalkSummary          `json:"walk,omitempty"`
	OpticalPorts []OpticalPortRow      `json:"optical_ports,omitempty"`
	InterfaceStatus []InterfaceStatusRow `json:"interface_status,omitempty"`
	PPPoESessions   []PPPoESessionRow    `json:"pppoe_sessions,omitempty"`
	Error        string                `json:"error,omitempty"`
}

// CollectionStatus estado geral da coleta.
type CollectionStatus struct {
	Skipped    []SkippedField `json:"skipped"`
	MissingOID []string       `json:"missing_oid"`
	Enabled    int            `json:"enabled"`
	Collected  int            `json:"collected"`
	Failed     int            `json:"failed"`
	Message    string         `json:"message,omitempty"`
}

// CollectOutput resultado completo.
type CollectOutput struct {
	Fields map[string]FieldResult `json:"fields"`
	Status CollectionStatus       `json:"status"`
}

type CollectOpts struct {
	WalkTarget string
	Timeout    time.Duration
	// Sections, se não vazio, limita a coleta a estas secções do catálogo (ex.: "optical").
	Sections []string
}

func sectionAllowed(section string, sections []string) bool {
	if len(sections) == 0 {
		return true
	}
	for _, s := range sections {
		if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(section)) {
			return true
		}
	}
	return false
}

func getOIDTimeout(total time.Duration) time.Duration {
	t := total
	if t <= 0 {
		t = 12 * time.Second
	}
	if t > 4*time.Second {
		return 4 * time.Second
	}
	if t < 2*time.Second {
		return 2 * time.Second
	}
	return t
}

// snmpGetScalar tenta GET; se falhar e o OID não terminar em .0, retenta com .0 (OIDs escalares comuns).
func snmpGetScalar(ctx context.Context, host, community, oid string, timeout time.Duration) (probing.SNMPGetResult, string) {
	oid = strings.TrimSpace(strings.TrimPrefix(oid, "."))
	to := getOIDTimeout(timeout)
	try := func(o string) probing.SNMPGetResult {
		return probing.SNMPGet(ctx, probing.SNMPGetParams{
			Host: host, Community: community, OIDs: []string{o},
			Version: "2c", Timeout: to, Retries: 0,
		})
	}
	res := try(oid)
	usable := res.OK && len(res.Vars) > 0 && probing.SNMPValueUsable(fmt.Sprint(res.Vars[0].Value))
	if usable {
		return res, oid
	}
	if !strings.HasSuffix(oid, ".0") {
		alt := oid + ".0"
		res2 := try(alt)
		if res2.OK && len(res2.Vars) > 0 && probing.SNMPValueUsable(fmt.Sprint(res2.Vars[0].Value)) {
			return res2, alt
		}
	}
	if res.OK && len(res.Vars) > 0 {
		return res, oid
	}
	return res, oid
}

// CollectMetrics executa coleta conforme perfil — só métricas enabled com OID preenchido.
func CollectMetrics(ctx context.Context, host, community string, profile Profile, opts CollectOpts) CollectOutput {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	out := CollectOutput{
		Fields: make(map[string]FieldResult),
		Status: CollectionStatus{},
	}
	if host == "" || community == "" {
		out.Status.Message = "host ou community SNMP em falta"
		return out
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	targetFilter := strings.TrimSpace(opts.WalkTarget)

	type pending struct {
		entry CatalogEntry
		def   MetricDef
		label string
		mode  string
		oid   string
		div   int
	}
	var gets []pending
	var rest []pending

	for _, entry := range profile.CatalogEntries() {
		if targetFilter != "" && entry.WalkTarget != targetFilter {
			continue
		}
		if !sectionAllowed(entry.Section, opts.Sections) {
			continue
		}
		def, ok := profile.Metrics[entry.Key]
		if !ok {
			def = profile.MetricsDefaults()[entry.Key]
		}
		label := entry.Label
		if !def.Enabled {
			out.Status.Skipped = append(out.Status.Skipped, SkippedField{
				Key: entry.Key, Label: label, Reason: "disabled",
			})
			continue
		}
		// Sub-métricas SFP em modo «coluna derivada» vêm do walk da tabela completa.
		if entry.Section == "optical" && entry.Key != "optical_table" {
			subMode := strings.TrimSpace(def.CollectMode)
			if subMode == "" {
				subMode = entry.DefaultMode
			}
			if subMode == ModeOpticalSFPColumn {
				if tbl, ok := profile.Metrics["optical_table"]; ok && tbl.Enabled && strings.TrimSpace(tbl.OID) != "" {
					out.Status.Skipped = append(out.Status.Skipped, SkippedField{
						Key: entry.Key, Label: label, Reason: "parsed_from_optical_table",
					})
					continue
				}
			}
		}
		out.Status.Enabled++
		oid := strings.TrimSpace(def.OID)
		if oid == "" {
			out.Status.Skipped = append(out.Status.Skipped, SkippedField{
				Key: entry.Key, Label: label, Reason: "oid_missing",
			})
			out.Status.MissingOID = append(out.Status.MissingOID, entry.Key)
			out.Fields[entry.Key] = FieldResult{
				Key: entry.Key, Label: label, OK: false, OID: "",
				CollectMode: def.CollectMode, Error: "OID não configurado — coleta ignorada",
			}
			continue
		}
		mode := strings.TrimSpace(def.CollectMode)
		if mode == "" {
			mode = entry.DefaultMode
		}
		if mode == ModeIFMibTable {
			mode = ModeSNMPWalk
		}
		div := EffectiveDivisor(def, entry)
		p := pending{entry: entry, def: def, label: label, mode: mode, oid: oid, div: div}
		if mode == ModeSNMPGet {
			gets = append(gets, p)
		} else {
			rest = append(rest, p)
		}
	}

	// Fase 1: GETs escalares (CPU, memória, temperatura, uptime) antes de walks pesados.
	for _, p := range gets {
		if ctx.Err() != nil {
			fr := FieldResult{Key: p.entry.Key, Label: p.label, CollectMode: p.mode, OID: p.oid, ValueDivisor: p.div, OK: false, Error: ctx.Err().Error()}
			out.Fields[p.entry.Key] = fr
			out.Status.Failed++
			continue
		}
		res, usedOID := snmpGetScalar(ctx, host, community, p.oid, timeout)
		fr := FieldResult{Key: p.entry.Key, Label: p.label, CollectMode: p.mode, OID: usedOID, ValueDivisor: p.div}
		if !res.OK || len(res.Vars) == 0 || !probing.SNMPValueUsable(fmt.Sprint(res.Vars[0].Value)) {
			fr.OK = false
			fr.Error = res.Error
			if fr.Error == "" {
				fr.Error = "SNMP GET sem resposta"
			}
			out.Status.Failed++
		} else {
			fr.OK = true
			fr.Value = applyDivisor(res.Vars[0].Value, p.div)
			out.Status.Collected++
		}
		out.Fields[p.entry.Key] = fr
	}

	// Fase 2: walks / óptico / IF-MIB / PPPoE.
	for _, p := range rest {
		if ctx.Err() != nil {
			fr := FieldResult{Key: p.entry.Key, Label: p.label, CollectMode: p.mode, OID: p.oid, ValueDivisor: p.div, OK: false, Error: ctx.Err().Error()}
			out.Fields[p.entry.Key] = fr
			out.Status.Failed++
			continue
		}
		fr := FieldResult{Key: p.entry.Key, Label: p.label, CollectMode: p.mode, OID: p.oid, ValueDivisor: p.div}
		switch p.mode {
		case ModeSNMPWalk, ModeIFMibTable:
			fr = collectSNMPWalk(ctx, host, community, timeout, p.entry, p.oid, p.div, profile.Metrics, fr, &out.Status)
		case ModeIFMibStatus:
			fr = collectIFMibStatus(ctx, host, community, timeout, p.entry, p.oid, fr, &out.Status)
		case ModeOpticalSFPParse:
			fr = collectOpticalSFPParse(ctx, host, community, timeout, profile, p.oid, fr, &out.Status)
		case ModeOpticalSFPColumn:
			fr = collectOpticalColumnWalk(ctx, host, community, timeout, p.entry, p.oid, p.div, fr, &out.Status)
		case ModeIFMibPPPoE:
			fr = collectIFMibPPPoE(ctx, host, community, timeout, p.oid, fr, &out.Status)
		default:
			fr.OK = false
			fr.Error = "modo de coleta desconhecido: " + p.mode
			out.Status.Failed++
		}
		out.Fields[p.entry.Key] = fr
	}

	// Propagar sub-métricas ópticas a partir do walk da tabela completa.
	if tbl, ok := out.Fields["optical_table"]; ok && tbl.OK && len(tbl.OpticalPorts) > 0 {
		for _, e := range profile.CatalogEntries() {
			if e.Section != "optical" || e.Key == "optical_table" {
				continue
			}
			def, ok := profile.Metrics[e.Key]
			if !ok || !def.Enabled {
				continue
			}
			subMode := strings.TrimSpace(def.CollectMode)
			if subMode == "" {
				subMode = e.DefaultMode
			}
			if subMode != ModeOpticalSFPColumn {
				continue
			}
			sub := FieldResult{
				Key: e.Key, Label: e.Label, OK: true, CollectMode: ModeOpticalSFPColumn,
				OID: strings.TrimSpace(def.OID), ValueDivisor: EffectiveDivisor(def, e),
				Value: len(tbl.OpticalPorts),
			}
			sub.OpticalPorts = filterOpticalPortsByColumn(tbl.OpticalPorts, e.OpticalColumn)
			out.Fields[e.Key] = sub
		}
	}

	for _, step := range profile.CollectionSteps {
		if !step.IsEnabled() {
			continue
		}
		// Passos manuais só no ciclo de telemetria completo (não em coleta filtrada por secção).
		if len(opts.Sections) > 0 {
			continue
		}
		oid := strings.TrimSpace(step.OID)
		if oid == "" {
			continue
		}
		storeAs := strings.TrimSpace(step.StoreAs)
		if storeAs == "" {
			storeAs = "step_" + strings.TrimSpace(step.ID)
		}
		out.Status.Enabled++
		fr := FieldResult{Key: storeAs, Label: storeAs, OID: oid, CollectMode: step.Method}
		switch step.Method {
		case MethodSNMPGet:
			res := probing.SNMPGet(ctx, probing.SNMPGetParams{
				Host: host, Community: community, OIDs: []string{oid},
				Version: "2c", Timeout: timeout, Retries: 0,
			})
			if res.OK && len(res.Vars) > 0 {
				fr.OK = true
				fr.Value = res.Vars[0].Value
				out.Status.Collected++
			} else {
				fr.OK = false
				fr.Error = res.Error
				out.Status.Failed++
			}
		case MethodSNMPWalk:
			vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: host, Port: 161, Community: community, RootOID: oid,
				Version: "2c", Timeout: timeout, Retries: 0, MaxRows: 8000,
			})
			if len(vars) == 0 {
				fr.OK = false
				fr.Error = "walk vazio"
				if walkNote != "" {
					fr.Error = walkNote
				}
				out.Status.Failed++
			} else {
				fr.OK = true
				stored := vars
				if len(stored) > maxWalkVarsStored {
					stored = stored[:maxWalkVarsStored]
				}
				fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: truncated, Vars: stored}
				fr.Value = len(vars)
				out.Status.Collected++
			}
		}
		out.Fields[storeAs] = fr
	}

	if out.Status.Enabled == 0 {
		out.Status.Message = "Nenhuma métrica activa — configure e active campos no perfil de coleta."
	} else if len(out.Status.MissingOID) > 0 {
		out.Status.Message = "Algumas métricas activas não têm OID configurado; não foram colectadas."
	} else if out.Status.Collected == 0 && out.Status.Failed > 0 {
		out.Status.Message = "Coleta falhou para todas as métricas activas."
	}
	return out
}

func walkMaxRows(entry CatalogEntry) int {
	if entry.WalkTarget == TargetInterfaces {
		return 42000
	}
	return 4000
}

func collectSNMPWalk(ctx context.Context, host, community string, timeout time.Duration, entry CatalogEntry, oid string, div int, metrics MetricsConfig, fr FieldResult, st *CollectionStatus) FieldResult {
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: oid,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: walkMaxRows(entry),
	})
	if len(vars) == 0 {
		fr.OK = false
		errMsg := "walk vazio — OID pode estar incorrecto ou indisponível neste equipamento"
		if walkNote != "" {
			errMsg = walkNote
		}
		fr.Error = errMsg
		st.Failed++
		return fr
	}
	fr.OK = true
	stored := vars
	if div > 1 && entry.Section != "optical" {
		stored = transformWalkVars(vars, div)
	}
	if len(stored) > maxWalkVarsStored {
		stored = stored[:maxWalkVarsStored]
	}
	fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: truncated, Vars: stored}
	if entry.Key == "optical_table" || strings.HasPrefix(oid, OpticalTableBaseOID) {
		fr.OpticalPorts = ParseOpticalPorts(vars, metrics)
		fr.Value = len(fr.OpticalPorts)
	} else if entry.OpticalColumn > 0 {
		fr.OpticalPorts = parseOpticalColumnOnly(vars, entry.OpticalColumn, div)
		fr.Value = len(fr.OpticalPorts)
	} else {
		fr.Value = len(vars)
	}
	st.Collected++
	return fr
}

func collectIFMibStatus(ctx context.Context, host, community string, timeout time.Duration, entry CatalogEntry, oid string, fr FieldResult, st *CollectionStatus) FieldResult {
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: oid,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: walkMaxRows(entry),
	})
	if len(vars) == 0 {
		fr.OK = false
		fr.Error = "walk vazio — OID pode estar incorrecto ou indisponível neste equipamento"
		if walkNote != "" {
			fr.Error = walkNote
		}
		st.Failed++
		return fr
	}
	fr.OK = true
	switch entry.IFMibColumn {
	case IFColAdminStatus:
		fr.InterfaceStatus = ParseInterfaceAdminStatus(vars)
	default:
		fr.InterfaceStatus = ParseInterfaceOperStatus(vars)
	}
	fr.Value = len(fr.InterfaceStatus)
	if truncated {
		fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: true}
	}
	st.Collected++
	return fr
}

func collectOpticalSFPParse(ctx context.Context, host, community string, timeout time.Duration, profile Profile, oid string, fr FieldResult, st *CollectionStatus) FieldResult {
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: oid,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: 42000,
	})
	if len(vars) == 0 {
		fr.OK = false
		fr.Error = "walk vazio — tabela óptica indisponível neste equipamento"
		if walkNote != "" {
			fr.Error = walkNote
		}
		st.Failed++
		return fr
	}
	fr.OK = true
	fr.OpticalPorts = ParseOpticalPorts(vars, profile.Metrics)
	fr.Value = len(fr.OpticalPorts)
	if truncated {
		fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: true}
	}
	st.Collected++
	return fr
}

func collectOpticalColumnWalk(ctx context.Context, host, community string, timeout time.Duration, entry CatalogEntry, oid string, div int, fr FieldResult, st *CollectionStatus) FieldResult {
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: oid,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: 42000,
	})
	if len(vars) == 0 {
		fr.OK = false
		fr.Error = "walk vazio — coluna SFP indisponível"
		if walkNote != "" {
			fr.Error = walkNote
		}
		st.Failed++
		return fr
	}
	fr.OK = true
	col := entry.OpticalColumn
	if col <= 0 {
		col = OptColRxPower
	}
	fr.OpticalPorts = parseOpticalColumnOnly(vars, col, div)
	fr.Value = len(fr.OpticalPorts)
	if truncated {
		fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: true}
	}
	st.Collected++
	return fr
}

func collectIFMibPPPoE(ctx context.Context, host, community string, timeout time.Duration, oid string, fr FieldResult, st *CollectionStatus) FieldResult {
	root := strings.TrimSpace(oid)
	if root == "" {
		root = IFTableBaseOID
	}
	// Walk na raiz ifTable (ou coluna ifDescr) — o parse filtra «pppoe».
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: root,
		Version: "2c", Timeout: timeout, Retries: 0, MaxRows: 42000,
	})
	if len(vars) == 0 {
		fr.OK = false
		fr.Error = "walk vazio — IF-MIB indisponível"
		if walkNote != "" {
			fr.Error = walkNote
		}
		st.Failed++
		return fr
	}
	fr.OK = true
	fr.PPPoESessions = ParsePPPoESessionsFromIFMib(vars)
	fr.Value = len(fr.PPPoESessions)
	if truncated {
		fr.Walk = &WalkSummary{RowCount: len(vars), Truncated: true}
	}
	st.Collected++
	return fr
}

func applyDivisor(v any, div int) any {
	if div <= 1 {
		return v
	}
	switch x := v.(type) {
	case float64:
		return x / float64(div)
	case int:
		return float64(x) / float64(div)
	case int64:
		return float64(x) / float64(div)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f / float64(div)
		}
	}
	return v
}

// BuildTelemetryMetricsJSON serializa resultado para telemetry_samples.
func BuildTelemetryMetricsJSON(collect CollectOutput, snmpVars []probing.SNMPVar, telnet TelnetCollectOutput) ([]byte, error) {
	doc := map[string]any{
		"mikrotik_collection": collect,
		"snmp": map[string]any{
			"vars": snmpVars,
		},
	}
	if telnet.ProfileID != "" || telnet.Collected > 0 || telnet.Failed > 0 || telnet.Message != "" {
		doc["mikrotik_telnet_collection"] = telnet
	}
	return json.Marshal(doc)
}

// ScalarFromFields extrai valor escalar de um campo colectado.
func ScalarFromFields(fields map[string]FieldResult, key string) (float64, bool) {
	fr, ok := fields[key]
	if !ok || !fr.OK || fr.Value == nil {
		return 0, false
	}
	switch x := fr.Value.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
