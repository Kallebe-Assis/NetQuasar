package switchcollect

import (
	"encoding/json"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

// TelnetSectionLabels secções da UI de perfis Switch (Cisco NX-OS).
var TelnetSectionLabels = map[string]string{
	"system":     "Sistema",
	"interfaces": "Interfaces",
	"optical":    "Óptica / SFP",
}

// TelnetMetricCatalog métricas CLI para Cisco NX-OS / Nexus.
var TelnetMetricCatalog = []mikrotikcollect.TelnetCatalogEntry{
	{
		Key: "telnet_sys_uptime", Section: "system", Label: "Uptime",
		Description:    "Tempo ligado (System uptime) via show system uptime.",
		DefaultCommand: "show system uptime",
		Parser:         "nxos_system_uptime",
		Fields:         "uptime",
	},
	{
		Key: "telnet_sys_identity", Section: "system", Label: "Hostname",
		Description:    "Nome do equipamento (show hostname).",
		DefaultCommand: "show hostname",
		Parser:         "nxos_hostname",
		Fields:         "name",
	},
	{
		Key: "telnet_if_list", Section: "interfaces", Label: "Status das interfaces",
		Description:    "Tabela show interface status (porta, nome, status, VLAN, duplex, speed, tipo).",
		DefaultCommand: "show interface status",
		Parser:         "nxos_interface_status",
		Fields:         "name, descr, status, vlan, duplex, speed, type",
	},
	{
		Key: "telnet_if_status_up", Section: "interfaces", Label: "Interfaces UP",
		Description:    "Apenas portas connected (show interface status up).",
		DefaultCommand: "show interface status up",
		Parser:         "nxos_interface_status",
		Fields:         "name, status, vlan, speed",
	},
	{
		Key: "telnet_if_status_down", Section: "interfaces", Label: "Interfaces DOWN / ausentes",
		Description:    "Portas não connected (show interface status down).",
		DefaultCommand: "show interface status down",
		Parser:         "nxos_interface_status",
		Fields:         "name, status, vlan, speed",
	},
	{
		Key: "telnet_sfp_rx_power", Section: "optical", Label: "Potência RX óptica",
		Description:    "Rx Power (dBm) de show interface transceiver details.",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-rx-power",
		Fields:         "interface, sfp-rx-power",
	},
	{
		Key: "telnet_sfp_tx_power", Section: "optical", Label: "Potência TX óptica",
		Description:    "Tx Power (dBm) de show interface transceiver details.",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-tx-power",
		Fields:         "interface, sfp-tx-power",
	},
	{
		Key: "telnet_sfp_temperature", Section: "optical", Label: "Temperatura do módulo SFP",
		Description:    "Temperatura do transceiver.",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-temperature",
		Fields:         "interface, sfp-temperature",
	},
	{
		Key: "telnet_sfp_voltage", Section: "optical", Label: "Tensão do módulo SFP",
		Description:    "Tensão de alimentação do módulo (V).",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-supply-voltage",
		Fields:         "interface, sfp-supply-voltage",
	},
	{
		Key: "telnet_sfp_bias_current", Section: "optical", Label: "Corrente do laser",
		Description:    "Corrente de bias TX (mA).",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-tx-bias-current",
		Fields:         "interface, sfp-tx-bias-current",
	},
	{
		Key: "telnet_sfp_vendor", Section: "optical", Label: "Fabricante / modelo / serial SFP",
		Description:    "Vendor, part number e serial do módulo óptico.",
		DefaultCommand: "show interface transceiver details",
		Parser:         "nxos_transceiver:sfp-vendor",
		Fields:         "interface, sfp-vendor-name, sfp-vendor-part-number, sfp-serial",
	},
}

var nxosDefaultMetricKeys = map[string]bool{
	"telnet_sys_uptime":      true,
	"telnet_sys_identity":    true,
	"telnet_if_list":         true,
	"telnet_sfp_rx_power":    true,
	"telnet_sfp_tx_power":    true,
	"telnet_sfp_temperature": true,
	"telnet_sfp_voltage":     true,
	"telnet_sfp_bias_current": true,
	"telnet_sfp_vendor":      true,
}

// DefaultTelnetPreCommands comandos após login no NX-OS (desactiva paginação).
func DefaultTelnetPreCommands() []string {
	return []string{"terminal length 0"}
}

func DefaultTelnetMetrics() mikrotikcollect.TelnetMetricsConfig {
	out := make(mikrotikcollect.TelnetMetricsConfig, len(TelnetMetricCatalog))
	for _, e := range TelnetMetricCatalog {
		out[e.Key] = mikrotikcollect.TelnetMetricDef{
			Enabled: nxosDefaultMetricKeys[e.Key],
			Command: e.DefaultCommand,
		}
	}
	return out
}

func TelnetCatalogByKey() map[string]mikrotikcollect.TelnetCatalogEntry {
	m := make(map[string]mikrotikcollect.TelnetCatalogEntry, len(TelnetMetricCatalog))
	for _, e := range TelnetMetricCatalog {
		m[e.Key] = e
	}
	return m
}

func ParseTelnetMetrics(raw []byte) mikrotikcollect.TelnetMetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var out mikrotikcollect.TelnetMetricsConfig
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out.Normalize()
}

func MergeTelnetMetrics(c mikrotikcollect.TelnetMetricsConfig) mikrotikcollect.TelnetMetricsConfig {
	def := DefaultTelnetMetrics()
	out := make(mikrotikcollect.TelnetMetricsConfig, len(TelnetMetricCatalog))
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
			// Ignora chaves RouterOS antigas que não existem no catálogo NX-OS.
			if _, known := TelnetCatalogByKey()[k]; known {
				out[k] = v
			}
		}
	}
	return out
}

func HasEnabledTelnetMetrics(c mikrotikcollect.TelnetMetricsConfig) bool {
	for _, e := range TelnetMetricCatalog {
		if def, ok := c[e.Key]; ok && def.Enabled {
			return true
		}
	}
	return false
}
