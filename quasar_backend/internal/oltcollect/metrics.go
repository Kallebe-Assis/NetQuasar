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
	MetricVlan        = "vlan"
)

// VSOL gOnuStaInfoPonInx — índice PON por ONU (sufixo .ONU) para tabelas com instância única.
const VSOLOnuPonIndexOID = "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.1.1"

const (
	StatusModePonOnuSuffix     = "pon_onu_suffix"
	StatusModeIfMibIndex       = "if_mib_index"
	StatusModePonCounts        = "pon_online_offline"
	StatusModeRxPowerThreshold = "rx_power_threshold"
)

// DefaultOfflineRxDbm limiar dBm (≤ valor ⇒ ONU offline). -70 cobre típico -80 em ZTE offline.
const DefaultOfflineRxDbm = -70.0

// OnuMetricDef uma métrica SNMP (tabela → snmpwalk → sufixo .PON.ONU).
type OnuMetricDef struct {
	Enabled         bool    `json:"enabled"`
	OID             string  `json:"oid"`
	ValueDivisor    int     `json:"value_divisor,omitempty"`
	OnlineValues    []int   `json:"online_values,omitempty"`
	OfflineValues   []int   `json:"offline_values,omitempty"`
	StatusMode      string  `json:"status_mode,omitempty"`
	IfDescrOID      string  `json:"ifdescr_oid,omitempty"`
	IfNameOID       string  `json:"ifname_oid,omitempty"`
	IfOperOID       string  `json:"ifoper_oid,omitempty"`
	OnlineCountOID  string  `json:"online_count_oid,omitempty"`
	OfflineCountOID string  `json:"offline_count_oid,omitempty"`
	OfflineRxDbm    float64 `json:"offline_rx_dbm,omitempty"`
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
	{MetricStatus, "Status (online / offline)", "Tabela SNMP, IF-MIB, contagem por PON ou RX: online se potência RX for maior que o limiar (ex. > -70 dBm; ZTE offline ~ -80). VSOL fase: …1.1.1.1.5 (working=3).", "1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5"},
	{MetricRxPower, "RX da ONU (dBm)", "Tabela SNMP da potência recebida na ONU; sufixo .PON.ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.7"},
	{MetricTxPower, "TX da ONU", "Tabela SNMP da potência transmitida pela ONU; sufixo .PON.ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.6"},
	{MetricTemperature, "Temperatura", "Tabela SNMP da temperatura", "1.3.6.1.4.1.37950.1.1.6.1.1.3.1.3"},
	{MetricModel, "Modelo da ONU", "Tabela SNMP do modelo; sufixo .PON.ONU (ex.: …2.1.6.3.10)", "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.6"},
	{MetricVlan, "VLAN (ONU)", "VLAN padrão da porta ONU (gOnuCfgPortVlanDefVlan); sufixo .PON.ONU", "1.3.6.1.4.1.37950.1.1.6.1.1.7.5.8"},
}

// PonMetricCatalog métricas por porta PON (sufixo .PON apenas).
var PonMetricCatalog = []struct {
	Key         string
	Label       string
	Description string
	Placeholder string
}{
	{MetricPonStatus, "Status da PON (OLT)", "Tabela SNMP do estado da interface PON (ex.: ifOperStatus por ifIndex da PON)", "1.3.6.1.2.1.2.2.1.8"},
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
		v.IfNameOID = strings.TrimSpace(v.IfNameOID)
		v.IfOperOID = strings.TrimSpace(v.IfOperOID)
		v.OnlineCountOID = strings.TrimSpace(v.OnlineCountOID)
		v.OfflineCountOID = strings.TrimSpace(v.OfflineCountOID)
		if key == MetricStatus || key == MetricPonStatus {
			if strings.EqualFold(v.StatusMode, StatusModeIfMibIndex) {
				if len(v.OnlineValues) == 0 || (len(v.OnlineValues) == 1 && v.OnlineValues[0] == 3) {
					v.OnlineValues = []int{1}
				}
			}
			if key == MetricPonStatus && v.StatusMode == "" {
				v.StatusMode = StatusModeIfMibIndex
			}
		}
		out[key] = v
	}
	if st, ok := out[MetricStatus]; ok && strings.EqualFold(st.StatusMode, StatusModeRxPowerThreshold) {
		if strings.TrimSpace(st.OID) == "" {
			if rx, ok := out[MetricRxPower]; ok && strings.TrimSpace(rx.OID) != "" {
				st.OID = rx.OID
			}
		}
		if rx, ok := out[MetricRxPower]; ok {
			if st.ValueDivisor <= 1 && rx.ValueDivisor > 1 {
				st.ValueDivisor = rx.ValueDivisor
			}
			if rx.ValueDivisor <= 1 && st.ValueDivisor > 1 {
				rx.ValueDivisor = st.ValueDivisor
				out[MetricRxPower] = rx
			}
		}
		if st.OfflineRxDbm == 0 {
			st.OfflineRxDbm = DefaultOfflineRxDbm
		}
		out[MetricStatus] = st
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
		if m.Key == MetricStatus && strings.EqualFold(strings.TrimSpace(def.StatusMode), StatusModeRxPowerThreshold) {
			if oid == "" {
				if rx, ok := c[MetricRxPower]; ok {
					oid = strings.TrimSpace(rx.OID)
				}
			}
		}
		if m.Key == MetricPonStatus && strings.EqualFold(strings.TrimSpace(def.StatusMode), StatusModeIfMibIndex) {
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

// FilterOnuMetricsByMode reduz métricas para coleta periódica simplificada.
// Modos: full | baseline | status_only | status_rx | pon_status | onu_counts
func FilterOnuMetricsByMode(c OnuMetricsConfig, mode string) OnuMetricsConfig {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "full" {
		return c
	}
	out := make(OnuMetricsConfig, len(c))
	for k, v := range c {
		cp := v
		cp.Enabled = false
		out[k] = cp
	}
	enable := func(keys ...string) {
		for _, k := range keys {
			if def, ok := c[k]; ok {
				def.Enabled = true
				out[k] = def
			}
		}
	}
	switch mode {
	case "baseline":
		// Linha-base obrigatória do ciclo: estado de cada ONU e estado/TX das PONs.
		enable(MetricStatus, MetricPonStatus, MetricPonTxPower)
	case "pon_status":
		enable(MetricPonStatus)
	case "onu_counts":
		enable(MetricStatus)
	case "status_only":
		enable(MetricStatus, MetricPonStatus)
	case "status_rx":
		enable(MetricStatus, MetricPonStatus, MetricRxPower, MetricPonRxPower)
	default:
		return c
	}
	return out
}

// IsPartialOnuCollectMode indica coleta leve (sem detalhe ONU / telnet).
func IsPartialOnuCollectMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "baseline", "pon_status", "onu_counts", "status_only", "status_rx":
		return true
	default:
		return false
	}
}

// IncludesTelnetOnuCollectMode — telnet e métricas completas só no modo full.
func IncludesTelnetOnuCollectMode(mode string) bool {
	m := strings.ToLower(strings.TrimSpace(mode))
	return m == "" || m == "full"
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
