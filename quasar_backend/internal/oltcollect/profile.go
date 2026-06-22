package oltcollect

import (
	"encoding/json"
	"strings"
)

// Métodos suportados na coleta manual OLT (perfil em Definições).
const (
	MethodIfMibRefresh    = "if_mib_refresh"
	MethodIfMibSnapshot   = "if_mib_snapshot"
	MethodVsolOnuCollect  = "vsol_onu_collect"
	MethodOnuSNMPWalk     = "onu_snmp_walk"
	MethodOnuMetricsCollect = "onu_metrics_collect"
	MethodSNMPWalk        = "snmp_walk"
	MethodSNMPGet         = "snmp_get"
	MethodTelnet          = "telnet"
	MethodDatacomBuildPons = "datacom_build_pons"
	MethodIfMibMergePons  = "if_mib_merge_pons"
	MethodStabilizePons   = "stabilize_pons"
	MethodOnuTelnetReport = "onu_telnet_report"
	MethodPonTelnetCollect = "pon_telnet_collect"
)

// ParserTelnet identifica como interpretar saída telnet.
const (
	ParserZteGponOnuState = "zte_gpon_onu_state"
)

// Step um passo do perfil de coleta.
type Step struct {
	ID          string         `json:"id,omitempty"`
	Method      string         `json:"method"`
	Enabled     *bool          `json:"enabled,omitempty"`
	OID         string         `json:"oid,omitempty"`
	OIDField    string         `json:"oid_field,omitempty"`
	OIDs        []string       `json:"oids,omitempty"`
	StoreAs     string         `json:"store_as,omitempty"`
	Command     string         `json:"command,omitempty"`
	PreCommands []string       `json:"pre_commands,omitempty"`
	Parser      string         `json:"parser,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

func (s Step) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// Profile perfil completo carregado da BD.
type Profile struct {
	Brand          string
	Model          string
	OnuOnlineOID   string
	PonStatusOID   string
	TransceiverOID string
	SNMPBaseOID    string
	Steps          []Step
	OnuMetrics     OnuMetricsConfig
	OnuReport      OnuReportConfig
	PonTelnet      PonTelnetConfig
}

// AppendOnuTelnetReportStep acrescenta passo de enriquecimento telnet quando activo no perfil.
func AppendOnuTelnetReportStep(steps []Step, profile Profile) []Step {
	if !profile.OnuReport.MonitorEnabled() {
		return steps
	}
	for _, s := range steps {
		if s.Method == MethodOnuTelnetReport {
			return steps
		}
	}
	en := true
	return append(steps, Step{
		ID:      "onu_telnet_report",
		Method:  MethodOnuTelnetReport,
		Enabled: &en,
	})
}

// AppendPonTelnetCollectStep acrescenta passo de métricas ópticas PON via telnet quando activo no perfil.
func AppendPonTelnetCollectStep(steps []Step, profile Profile) []Step {
	if !profile.PonTelnet.MonitorEnabled() {
		return steps
	}
	for _, s := range steps {
		if s.Method == MethodPonTelnetCollect {
			return steps
		}
	}
	en := true
	return append(steps, Step{
		ID:      "pon_telnet_collect",
		Method:  MethodPonTelnetCollect,
		Enabled: &en,
	})
}

// ResolveWalkOID OID do passo: step.oid, oid_field no perfil, snmp_base_oid, ou vazio.
func (p Profile) ResolveWalkOID(step Step) string {
	if o := strings.TrimSpace(step.OID); o != "" {
		return o
	}
	if step.OIDField != "" {
		if o := p.OIDForField(step.OIDField); o != "" {
			return o
		}
	}
	return strings.TrimSpace(p.SNMPBaseOID)
}

func (p Profile) OIDForField(field string) string {
	switch strings.TrimSpace(field) {
	case "onu_online_oid":
		return strings.TrimSpace(p.OnuOnlineOID)
	case "pon_status_oid":
		return strings.TrimSpace(p.PonStatusOID)
	case "transceiver_oid":
		return strings.TrimSpace(p.TransceiverOID)
	case "snmp_base_oid":
		return strings.TrimSpace(p.SNMPBaseOID)
	default:
		return ""
	}
}

// ParseSteps interpreta JSONB (array ou objeto com chave steps).
func ParseSteps(raw []byte) []Step {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return nil
	}
	var steps []Step
	if err := json.Unmarshal(raw, &steps); err == nil && len(steps) > 0 {
		return normalizeSteps(steps)
	}
	var wrap struct {
		Steps []Step `json:"steps"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && len(wrap.Steps) > 0 {
		return normalizeSteps(wrap.Steps)
	}
	return nil
}

func normalizeSteps(in []Step) []Step {
	out := make([]Step, 0, len(in))
	for _, s := range in {
		s.Method = strings.TrimSpace(strings.ToLower(s.Method))
		if s.Method == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IsSimpleOnuWalk perfil com um único passo onu_snmp_walk ou onu_metrics_collect.
func IsSimpleOnuWalk(steps []Step) bool {
	return IsSimpleOnuCollect(steps)
}

// EnabledSteps devolve apenas passos activos.
func EnabledSteps(steps []Step) []Step {
	out := make([]Step, 0, len(steps))
	for _, s := range steps {
		if s.IsEnabled() {
			out = append(out, s)
		}
	}
	return out
}

// EffectiveCollectionSteps devolve passos activos do perfil ou, se vazio, deriva de onu_metrics.
func EffectiveCollectionSteps(p Profile) []Step {
	steps := EnabledSteps(p.Steps)
	if len(steps) > 0 {
		return steps
	}
	return DefaultStepsFromMetrics(p.OnuMetrics)
}

// EffectivePeriodicSteps usa os mesmos passos que o refresh manual (paridade automático/manual).
func EffectivePeriodicSteps(p Profile) []Step {
	return EffectiveCollectionSteps(p)
}

// MethodLabels para UI.
var MethodLabels = map[string]string{
	MethodIfMibRefresh:     "SNMP — actualizar snapshot IF-MIB",
	MethodIfMibSnapshot:    "SNMP — ler IF-MIB (snapshot ou walk)",
	MethodVsolOnuCollect:   "VSOL — tabela gOnuAuthList (snmpwalk)",
	MethodOnuSNMPWalk:        "ONUs — snmpwalk no OID do perfil e contagem",
	MethodOnuMetricsCollect:  "ONUs — coletar métricas SNMP configuradas",
	MethodSNMPWalk:           "SNMP — snmpwalk (OID)",
	MethodSNMPGet:          "SNMP — snmpget (vários OIDs)",
	MethodTelnet:           "Telnet — comando CLI",
	MethodDatacomBuildPons: "Datacom — agregar PONs a partir do walk ONU",
	MethodIfMibMergePons:   "IF-MIB — derivar e fundir PONs no snapshot",
	MethodStabilizePons:    "Estabilizar PONs vs. snapshot anterior",
	MethodOnuTelnetReport:  "Telnet — enriquecer ONUs (perfil)",
	MethodPonTelnetCollect: "Telnet — métricas PON/SFP (perfil)",
}
