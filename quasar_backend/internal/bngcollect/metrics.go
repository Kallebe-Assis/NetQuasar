package bngcollect

import (
	"encoding/json"
	"strings"
)

const (
	ModeSNMPGet        = "snmp_get"
	ModeSNMPWalk       = "snmp_walk"
	ModeAccessSessions = "access_sessions"
)

// MetricDef configuração de uma métrica BNG.
type MetricDef struct {
	Enabled     bool   `json:"enabled"`
	OID         string `json:"oid"`
	CollectMode string `json:"collect_mode"`
}

// MetricsConfig mapa chave → definição.
type MetricsConfig map[string]MetricDef

// CatalogEntry metadados para UI e coleta.
type CatalogEntry struct {
	Key         string   `json:"key"`
	Section     string   `json:"section"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Placeholder string   `json:"placeholder"`
	CollectModes []string `json:"collect_modes"`
	DefaultMode string   `json:"default_mode"`
	Unit        string   `json:"unit,omitempty"`
	Recommended bool     `json:"recommended,omitempty"`
}

var SectionLabels = map[string]string{
	"system":  "Sistema / inventário",
	"health":  "Saúde do equipamento",
	"subscribers": "Totais de logins (escalares)",
	"pppoe":   "Sessões PPPoE (walk — pesado)",
}

// MetricCatalog catálogo de métricas BNG (Huawei AAA / NE8000 por defeito).
var MetricCatalog = []CatalogEntry{
	{Key: "sys_descr", Section: "system", Label: "Descrição (sysDescr)", Description: "MIB-2 — texto completo do sistema.", Placeholder: "1.3.6.1.2.1.1.1.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "sys_name", Section: "system", Label: "Nome (sysName)", Description: "Nome configurado do host.", Placeholder: "1.3.6.1.2.1.1.5.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "sys_uptime", Section: "system", Label: "Uptime", Description: "Tempo ligado desde o último boot.", Placeholder: "1.3.6.1.2.1.1.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "ticks", Recommended: true},
	{Key: "hw_model", Section: "system", Label: "Modelo (hwEntitySystemModel)", Description: "Modelo Huawei (entPhysical).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.6.5.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "hw_software", Section: "system", Label: "Versão software", Description: "hwEntitySoftwareVersion.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.6.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},

	{Key: "cpu_usage", Section: "health", Label: "CPU (%)", Description: "hwEntityCpuUsage — substitua o índice da placa (ex.: …5.17367041).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5.17367041", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "%", Recommended: true},
	{Key: "memory_usage", Section: "health", Label: "Memória (%)", Description: "hwEntityMemUsage — índice da placa IPU.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7.17367041", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "%"},
	{Key: "temperature", Section: "health", Label: "Temperatura", Description: "hwEntityTemperature — índice da placa.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.11.17367041", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "°C"},

	{Key: "total_online", Section: "subscribers", Label: "Total online", Description: "hwTotalOnlineNum — todos os utilizadores online.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.14.1.1.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "pppoe_online", Section: "subscribers", Label: "PPPoE online", Description: "hwTotalPPPoeOnlineNum.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.14.1.2.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "ipv4_online", Section: "subscribers", Label: "IPv4 online", Description: "hwTotalIPv4OnlineNum.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.14.1.15.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "ipv6_online", Section: "subscribers", Label: "IPv6 online", Description: "hwTotalIPv6OnlineNum.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.14.1.16.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},
	{Key: "dual_stack_online", Section: "subscribers", Label: "Dual-stack (v4+v6)", Description: "hwTotalDualStackOnlineNum.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.14.1.17.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Recommended: true},

	{Key: "access_login", Section: "pppoe", Label: "Login (hwAccessUserName)", Description: "Walk coluna de utilizadores — hwAccessTable.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.3", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_ipv4", Section: "pppoe", Label: "IPv4 (hwAccessIPAddress)", Description: "Endereço IPv4 por sessão (CGNAT ou público).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.15", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_mac", Section: "pppoe", Label: "MAC (hwAccessMACAddress)", Description: "MAC do cliente PPPoE.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.17", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_ipv6", Section: "pppoe", Label: "IPv6 WAN", Description: "hwAccessIPv6WanAddress — endereço IPv6 atribuído na WAN.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.59", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_ipv6_pd", Section: "pppoe", Label: "IPv6 PD/LAN", Description: "hwAccessIPv6LanAddress — prefixo/delegação IPv6 (PD) para a LAN do cliente.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.61", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_ip_type", Section: "pppoe", Label: "Tipo IP (v4/v6)", Description: "hwAccessBasicIPType — flags ASCII/binários (ex.: «100» = IPv4 activo, sem IPv6).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.63", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_port_type", Section: "pppoe", Label: "Tipo de porta", Description: "hwAccessPortType — valor 2 = PPP/PPPoE.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.5", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_online_time", Section: "pppoe", Label: "Tempo online", Description: "hwAccessOnlineTime — segundos desde o login (Gauge32).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.16.1.18", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk, Unit: "s"},
	{Key: "access_vlan", Section: "pppoe", Label: "VLAN (hwAccessVLANID)", Description: "VLAN de acesso do utilizador (QinQ/PPPoE).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.11", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_interface", Section: "pppoe", Label: "Interface de acesso", Description: "hwAccessInterface — interface física/lógica de entrada.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.57", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_domain", Section: "pppoe", Label: "Domínio AAA", Description: "hwAccessDomainName — domínio RADIUS/local do login.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.16.1.7", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_up_flow", Section: "pppoe", Label: "Tráfego upstream (bytes)", Description: "hwAccessUpFlow64 — contador acumulado; calcular taxa por delta entre coletas.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.36", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_dn_flow", Section: "pppoe", Label: "Tráfego downstream (bytes)", Description: "hwAccessDnFlow64 — contador acumulado; calcular taxa por delta entre coletas.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.37", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "access_car_up_cir", Section: "pppoe", Label: "Limite upstream (CIR kbit/s)", Description: "hwAccessCARUpCIR — taxa contratada/limitada upstream (não é taxa instantânea).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.45", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk, Unit: "kbit/s"},
	{Key: "access_car_dn_cir", Section: "pppoe", Label: "Limite downstream (CIR kbit/s)", Description: "hwAccessCARDnCIR — taxa contratada/limitada downstream.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.49", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk, Unit: "kbit/s"},
	{Key: "access_qos_profile", Section: "pppoe", Label: "Perfil QoS", Description: "hwAccessQosProfile — perfil de QoS aplicado ao login.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.15.1.56", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "auth_state", Section: "pppoe", Label: "Estado autenticação", Description: "hwAuthenticationState (hwAccessExtTable).", Placeholder: "1.3.6.1.4.1.2011.5.2.1.16.1.4", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "author_state", Section: "pppoe", Label: "Estado autorização", Description: "hwAuthorizationState.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.16.1.5", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
	{Key: "acct_state", Section: "pppoe", Label: "Estado accounting", Description: "hwAccountingState.", Placeholder: "1.3.6.1.4.1.2011.5.2.1.16.1.6", CollectModes: []string{ModeSNMPWalk, ModeAccessSessions}, DefaultMode: ModeSNMPWalk},
}

func DefaultMetrics() MetricsConfig {
	out := make(MetricsConfig, len(MetricCatalog))
	for _, e := range MetricCatalog {
		mode := e.DefaultMode
		if mode == "" {
			mode = ModeSNMPGet
		}
		enabled := e.Recommended
		out[e.Key] = MetricDef{
			Enabled:     enabled,
			OID:         e.Placeholder,
			CollectMode: mode,
		}
	}
	return out
}

func ParseMetrics(raw []byte) MetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var m MetricsConfig
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func (m MetricsConfig) MergeWithDefaults() MetricsConfig {
	base := DefaultMetrics()
	for k, v := range m {
		if def, ok := base[k]; ok {
			if v.OID == "" {
				v.OID = def.OID
			}
			if v.CollectMode == "" {
				v.CollectMode = def.CollectMode
			}
			base[k] = v
		}
	}
	return base
}

func (m MetricsConfig) Normalize() MetricsConfig {
	if m == nil {
		return MetricsConfig{}
	}
	out := make(MetricsConfig)
	for _, e := range MetricCatalog {
		if def, ok := m[e.Key]; ok {
			if def.CollectMode == "" {
				def.CollectMode = e.DefaultMode
			}
			if def.CollectMode == "" {
				def.CollectMode = ModeSNMPGet
			}
			out[e.Key] = def
		}
	}
	return out
}

func HasEnabledMetrics(m MetricsConfig) bool {
	for _, e := range MetricCatalog {
		if def, ok := m[e.Key]; ok && def.Enabled && strings.TrimSpace(def.OID) != "" {
			return true
		}
	}
	return false
}

func SessionWalkKeys() []string {
	return []string{
		"access_login", "access_ipv4", "access_mac", "access_ipv6", "access_ipv6_pd",
		"access_ip_type", "access_port_type", "access_online_time",
		"access_vlan", "access_interface", "access_domain",
		"access_up_flow", "access_dn_flow", "access_car_up_cir", "access_car_dn_cir", "access_qos_profile",
		"auth_state", "author_state", "acct_state",
	}
}

func PeriodicTotalKeys() []string {
	return []string{"total_online", "pppoe_online", "ipv4_online", "ipv6_online", "dual_stack_online"}
}
