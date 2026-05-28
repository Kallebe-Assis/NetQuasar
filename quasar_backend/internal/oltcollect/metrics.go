package oltcollect

import (
	"encoding/json"
	"strings"
)

// Chaves de métricas ONU configuráveis na UI.
const (
	MetricSerial      = "serial"
	MetricStatus      = "status"
	MetricRxPower     = "rx_power"
	MetricTxPower     = "tx_power"
	MetricPonStatus   = "pon_status"
	MetricPonRxPower  = "pon_rx_power"
	MetricPonTxPower  = "pon_tx_power"
	MetricPonVoltage  = "pon_voltage"
	MetricPonCurrent  = "pon_current"
	MetricPonTemp     = "pon_temperature"
	MetricTemperature = "temperature"
	MetricModel       = "model"
)

const (
	StatusModePonOnuSuffix = "pon_onu_suffix"
	StatusModeIfMibIndex   = "if_mib_index"
	StatusModePonCounts    = "pon_online_offline"
)

// OnuMetricDef uma métrica SNMP (tabela → snmpwalk → sufixo .PON.ONU).
type OnuMetricDef struct {
	Enabled         bool   `json:"enabled"`
	OID             string `json:"oid"`
	ValueDivisor    int    `json:"value_divisor,omitempty"`
	OnlineValues    []int  `json:"online_values,omitempty"`
	OfflineValues   []int  `json:"offline_values,omitempty"`
	StatusMode      string `json:"status_mode,omitempty"`
	IfDescrOID      string `json:"ifdescr_oid,omitempty"`
	IfOperOID       string `json:"ifoper_oid,omitempty"`
	OnlineCountOID  string `json:"online_count_oid,omitempty"`
	OfflineCountOID string `json:"offline_count_oid,omitempty"`
}

// OnuMetricsConfig mapa métrica → definição.
type OnuMetricsConfig map[string]OnuMetricDef

// MetricCatalog metadados para a UI.
var MetricCatalog = []struct {
	Key         string
	Label       string
	Description string
	Placeholder string
}{
	{MetricSerial, "Número de série", "Tabela SNMP do serial da ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5"},
	{MetricStatus, "Estado (online/offline)", "Tabela SNMP do estado; valores 3=online e 4=offline (ajustável)", "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2"},
	{MetricRxPower, "RX da ONU (dBm)", "Tabela SNMP da potência recebida na ONU; sufixo .PON.ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7"},
	{MetricTxPower, "TX da ONU", "Tabela SNMP da potência transmitida pela ONU; sufixo .PON.ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6"},
	{MetricTemperature, "Temperatura", "Tabela SNMP da temperatura", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.3"},
	{MetricModel, "Modelo", "Tabela SNMP do modelo; sufixo .PON.ONU (ex.: …2.1.6.3.10)", "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6"},
}

// PonMetricCatalog métricas por porta PON (sufixo .PON apenas).
var PonMetricCatalog = []struct {
	Key         string
	Label       string
	Description string
	Placeholder string
}{
	{MetricPonStatus, "Estado da PON (OLT)", "Tabela SNMP do estado da interface PON (ex.: ifOperStatus por ifIndex da PON)", "1.3.6.1.2.1.2.2.1.8"},
	{MetricPonRxPower, "RX da PON (OLT)", "Tabela SNMP da potência recebida na porta PON da OLT; sufixo .PON", ""},
	{MetricPonTxPower, "TX da PON (OLT)", "Tabela SNMP da potência transmitida na porta PON da OLT; sufixo .PON", ""},
	{MetricPonVoltage, "Voltagem da PON (OLT)", "Tabela SNMP da voltagem por porta PON da OLT; sufixo .PON", ""},
	{MetricPonCurrent, "Corrente da PON (OLT)", "Tabela SNMP da corrente por porta PON da OLT; sufixo .PON", ""},
	{MetricPonTemp, "Temperatura da PON (OLT)", "Tabela SNMP da temperatura por porta PON da OLT; sufixo .PON", ""},
}

func metricCatalogEntries() []struct {
	Key         string
	Label       string
	Description string
	Placeholder string
} {
	return append(MetricCatalog, PonMetricCatalog...)
}

func ParseOnuMetrics(raw []byte) OnuMetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var out OnuMetricsConfig
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out.Normalize()
}

func (c OnuMetricsConfig) Normalize() OnuMetricsConfig {
	if len(c) == 0 {
		return nil
	}
	out := make(OnuMetricsConfig, len(c))
	for k, v := range c {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		v.OID = strings.TrimSpace(v.OID)
		if v.ValueDivisor < 0 {
			v.ValueDivisor = 0
		}
		v.StatusMode = strings.TrimSpace(v.StatusMode)
		v.IfDescrOID = strings.TrimSpace(v.IfDescrOID)
		v.IfOperOID = strings.TrimSpace(v.IfOperOID)
		v.OnlineCountOID = strings.TrimSpace(v.OnlineCountOID)
		v.OfflineCountOID = strings.TrimSpace(v.OfflineCountOID)
		out[key] = v
	}
	return out
}

func (c OnuMetricsConfig) EnabledMetrics() []string {
	if len(c) == 0 {
		return nil
	}
	var keys []string
	for _, m := range metricCatalogEntries() {
		def, ok := c[m.Key]
		if !ok || !def.Enabled {
			continue
		}
		if m.Key == MetricStatus && strings.EqualFold(strings.TrimSpace(def.StatusMode), StatusModePonCounts) {
			if strings.TrimSpace(def.OnlineCountOID) != "" && strings.TrimSpace(def.OfflineCountOID) != "" {
				keys = append(keys, m.Key)
			}
			continue
		}
		oid := strings.TrimSpace(def.OID)
		if m.Key == MetricStatus && strings.EqualFold(strings.TrimSpace(def.StatusMode), StatusModeIfMibIndex) {
			if oid == "" {
				oid = strings.TrimSpace(def.IfOperOID)
			}
		}
		if oid != "" {
			keys = append(keys, m.Key)
		}
	}
	return keys
}

func (c OnuMetricsConfig) HasAnyEnabled() bool {
	return len(c.EnabledMetrics()) > 0
}

func DefaultStepsFromMetrics(c OnuMetricsConfig) []Step {
	if !c.HasAnyEnabled() {
		return nil
	}
	en := true
	return []Step{{ID: "onu_metrics", Method: MethodOnuMetricsCollect, Enabled: &en}}
}

func IsMetricsCollectProfile(steps []Step) bool {
	en := EnabledSteps(steps)
	if len(en) == 0 {
		return false
	}
	for _, s := range en {
		if s.Method != MethodOnuMetricsCollect && s.Method != MethodOnuSNMPWalk {
			return false
		}
	}
	for _, s := range en {
		if s.Method == MethodOnuMetricsCollect {
			return true
		}
	}
	return false
}

func IsSimpleOnuCollect(steps []Step) bool {
	en := EnabledSteps(steps)
	if len(en) == 0 {
		return false
	}
	for _, s := range en {
		switch s.Method {
		case MethodOnuMetricsCollect, MethodOnuSNMPWalk:
		default:
			return false
		}
	}
	return true
}
