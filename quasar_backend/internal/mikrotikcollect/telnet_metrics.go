package mikrotikcollect

import (
	"encoding/json"
	"strings"
)

const ModeTelnet = "telnet"

// Scope de coleta telnet.
const (
	TelnetScopeGlobal       = ""
	TelnetScopePerInterface = "per_interface"
	TelnetScopePerEthernet  = "per_ethernet"
	TelnetScopePerSFP       = "per_sfp"
)

// TelnetMetricDef configuração de uma métrica coletada via CLI RouterOS (telnet).
type TelnetMetricDef struct {
	Enabled bool   `json:"enabled"`
	Command string `json:"command,omitempty"`
}

// TelnetMetricsConfig mapa chave → definição telnet.
type TelnetMetricsConfig map[string]TelnetMetricDef

// TelnetCatalogEntry metadados para UI e coleta telnet.
type TelnetCatalogEntry struct {
	Key            string `json:"key"`
	Section        string `json:"section"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	DefaultCommand string `json:"default_command"`
	Parser         string `json:"parser"`
	Scope          string `json:"scope,omitempty"`
	Fields         string `json:"fields,omitempty"`
}

var TelnetSectionLabels = map[string]string{
	"system":     "Sistema",
	"health":     "Saúde",
	"interfaces": "Interfaces",
	"optical":    "Óptica / SFP",
	"wireless":   "Wireless",
}

// TelnetMetricCatalog métricas NOC via telnet RouterOS.
var TelnetMetricCatalog = []TelnetCatalogEntry{
	// Sistema
	{Key: "telnet_sys_identity", Section: "system", Label: "Nome do equipamento", Description: "Identidade RouterOS (hostname).", DefaultCommand: "/system identity print without-paging", Parser: "system_identity", Fields: "name"},
	{Key: "telnet_sys_uptime", Section: "system", Label: "Uptime", Description: "Tempo ligado desde o último reboot.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:uptime", Fields: "uptime"},
	{Key: "telnet_sys_cpu_load", Section: "system", Label: "CPU (%)", Description: "Carga de CPU actual.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:cpu-load", Fields: "cpu-load"},
	{Key: "telnet_sys_cpu_frequency", Section: "system", Label: "Frequência da CPU", Description: "Frequência actual do processador.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:cpu-frequency", Fields: "cpu-frequency"},
	{Key: "telnet_sys_free_memory", Section: "system", Label: "Memória livre", Description: "RAM livre.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:free-memory", Fields: "free-memory"},
	{Key: "telnet_sys_total_memory", Section: "system", Label: "Memória total", Description: "RAM total instalada.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:total-memory", Fields: "total-memory"},
	{Key: "telnet_sys_free_hdd", Section: "system", Label: "Disco livre", Description: "Espaço livre em disco.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:free-hdd-space", Fields: "free-hdd-space"},
	{Key: "telnet_sys_total_hdd", Section: "system", Label: "Disco total", Description: "Capacidade total em disco.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:total-hdd-space", Fields: "total-hdd-space"},
	{Key: "telnet_sys_version", Section: "system", Label: "Versão RouterOS", Description: "Versão do sistema em execução.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:version", Fields: "version"},
	{Key: "telnet_sys_board", Section: "system", Label: "Placa / modelo", Description: "Nome da placa (board-name).", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:board-name", Fields: "board-name"},
	{Key: "telnet_sys_platform", Section: "system", Label: "Plataforma", Description: "Plataforma RouterOS.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:platform", Fields: "platform"},
	{Key: "telnet_sys_architecture", Section: "system", Label: "Arquitectura", Description: "Arquitectura da CPU.", DefaultCommand: "/system resource print without-paging", Parser: "system_resource:architecture-name", Fields: "architecture-name"},
	// Saúde
	{Key: "telnet_sys_temperature", Section: "health", Label: "Temperatura do equipamento", Description: "Sensor de temperatura da placa.", DefaultCommand: "/system health print without-paging", Parser: "system_health:temperature", Fields: "temperature"},
	{Key: "telnet_sys_voltage", Section: "health", Label: "Tensão de alimentação", Description: "Tensão de alimentação (V).", DefaultCommand: "/system health print without-paging", Parser: "system_health:voltage", Fields: "voltage"},
	// Interfaces
	{Key: "telnet_if_list", Section: "interfaces", Label: "Lista de interfaces", Description: "Nome, tipo, MTU e estado running.", DefaultCommand: "/interface print without-paging", Parser: "interface_list", Fields: "name, type, mtu, running"},
	{Key: "telnet_if_traffic", Section: "interfaces", Label: "Tráfego instantâneo", Description: "RX/TX em bits/s por interface (monitor-traffic once). Use {interface} no comando.", DefaultCommand: "/interface monitor-traffic {interface} once", Parser: "interface_traffic", Scope: TelnetScopePerInterface, Fields: "rx-bits-per-second, tx-bits-per-second"},
	{Key: "telnet_if_stats", Section: "interfaces", Label: "Estatísticas acumuladas", Description: "Contadores RX/TX e erros por interface.", DefaultCommand: "/interface print stats without-paging", Parser: "interface_stats", Fields: "rx-byte, tx-byte, rx-packet, tx-packet, rx-error, tx-error"},
	{Key: "telnet_if_eth_status", Section: "interfaces", Label: "Status da interface (ethernet)", Description: "Status, rate e duplex por porta ethernet. Use {interface}.", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:status", Scope: TelnetScopePerEthernet, Fields: "status, rate, full-duplex"},
	// Óptica / SFP
	{Key: "telnet_sfp_temperature", Section: "optical", Label: "Temperatura do módulo SFP", Description: "Temperatura do transceiver. Use {interface} para a porta SFP.", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-temperature", Scope: TelnetScopePerSFP, Fields: "sfp-temperature"},
	{Key: "telnet_sfp_rx_power", Section: "optical", Label: "Potência RX óptica", Description: "Potência de recepção (dBm).", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-rx-power", Scope: TelnetScopePerSFP, Fields: "sfp-rx-power"},
	{Key: "telnet_sfp_tx_power", Section: "optical", Label: "Potência TX óptica", Description: "Potência de transmissão (dBm).", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-tx-power", Scope: TelnetScopePerSFP, Fields: "sfp-tx-power"},
	{Key: "telnet_sfp_voltage", Section: "optical", Label: "Tensão do módulo SFP", Description: "Tensão de alimentação do módulo.", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-supply-voltage", Scope: TelnetScopePerSFP, Fields: "sfp-supply-voltage"},
	{Key: "telnet_sfp_bias_current", Section: "optical", Label: "Corrente do laser", Description: "Corrente de bias TX do laser.", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-tx-bias-current", Scope: TelnetScopePerSFP, Fields: "sfp-tx-bias-current"},
	{Key: "telnet_sfp_vendor", Section: "optical", Label: "Fabricante / modelo / serial SFP", Description: "Dados do vendor do módulo óptico.", DefaultCommand: "/interface ethernet monitor {interface} once", Parser: "ethernet_monitor:sfp-vendor", Scope: TelnetScopePerSFP, Fields: "sfp-vendor-name, sfp-vendor-part-number, sfp-serial"},
	// Wireless (opcional — fora do pacote NOC base)
	{Key: "telnet_wireless", Section: "wireless", Label: "Wireless (legado)", Description: "SSID, canal, protocolo e estado.", DefaultCommand: "/interface wireless print detail without-paging", Parser: "wireless_detail"},
	{Key: "telnet_wifiwave2", Section: "wireless", Label: "WifiWave2", Description: "SSID, canal, banda e clientes (802.11ax).", DefaultCommand: "/interface wifiwave2 print detail without-paging", Parser: "wifiwave2_detail"},
}

// nocDefaultMetricKeys métricas activas por defeito no perfil NOC.
var nocDefaultMetricKeys = map[string]bool{
	"telnet_sys_identity":      true,
	"telnet_sys_uptime":          true,
	"telnet_sys_cpu_load":        true,
	"telnet_sys_cpu_frequency":   true,
	"telnet_sys_free_memory":     true,
	"telnet_sys_total_memory":    true,
	"telnet_sys_free_hdd":        true,
	"telnet_sys_total_hdd":       true,
	"telnet_sys_version":         true,
	"telnet_sys_board":           true,
	"telnet_sys_platform":        true,
	"telnet_sys_architecture":    true,
	"telnet_sys_temperature":     true,
	"telnet_sys_voltage":         true,
	"telnet_if_list":             true,
	"telnet_if_traffic":          true,
	"telnet_if_stats":            true,
	"telnet_if_eth_status":       true,
	"telnet_sfp_temperature":     true,
	"telnet_sfp_rx_power":        true,
	"telnet_sfp_tx_power":        true,
	"telnet_sfp_voltage":         true,
	"telnet_sfp_bias_current":    true,
	"telnet_sfp_vendor":          true,
}

func DefaultTelnetMetrics() TelnetMetricsConfig {
	out := make(TelnetMetricsConfig, len(TelnetMetricCatalog))
	for _, e := range TelnetMetricCatalog {
		out[e.Key] = TelnetMetricDef{
			Enabled: nocDefaultMetricKeys[e.Key],
			Command: e.DefaultCommand,
		}
	}
	return out
}

func ParseTelnetMetrics(raw []byte) TelnetMetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var out TelnetMetricsConfig
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out.Normalize()
}

func (c TelnetMetricsConfig) Normalize() TelnetMetricsConfig {
	if len(c) == 0 {
		return nil
	}
	out := make(TelnetMetricsConfig, len(c))
	for k, v := range c {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		v.Command = strings.TrimSpace(v.Command)
		out[key] = v
	}
	return out
}

func (c TelnetMetricsConfig) MergeWithDefaults() TelnetMetricsConfig {
	def := DefaultTelnetMetrics()
	out := make(TelnetMetricsConfig, len(TelnetMetricCatalog))
	for k, v := range def {
		out[k] = v
	}
	for k, v := range c {
		if cur, ok := out[k]; ok {
			cur.Enabled = v.Enabled
			if strings.TrimSpace(v.Command) != "" {
				cur.Command = strings.TrimSpace(v.Command)
			}
			out[k] = cur
		} else {
			out[k] = v
		}
	}
	return out
}

func HasEnabledTelnetMetrics(c TelnetMetricsConfig) bool {
	for _, e := range TelnetMetricCatalog {
		if def, ok := c[e.Key]; ok && def.Enabled {
			return true
		}
	}
	return false
}

func TelnetCatalogByKey() map[string]TelnetCatalogEntry {
	m := make(map[string]TelnetCatalogEntry, len(TelnetMetricCatalog))
	for _, e := range TelnetMetricCatalog {
		m[e.Key] = e
	}
	return m
}

func (c TelnetMetricsConfig) CommandFor(key string) string {
	if def, ok := c[key]; ok && strings.TrimSpace(def.Command) != "" {
		return strings.TrimSpace(def.Command)
	}
	if e, ok := TelnetCatalogByKey()[key]; ok {
		return e.DefaultCommand
	}
	return ""
}

func catalogEntryForKey(key string) (TelnetCatalogEntry, bool) {
	e, ok := TelnetCatalogByKey()[key]
	return e, ok
}
