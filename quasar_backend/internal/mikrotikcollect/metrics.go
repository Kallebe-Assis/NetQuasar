package mikrotikcollect

import (
	"encoding/json"
	"strings"
)

const (
	ModeSNMPGet           = "snmp_get"
	ModeSNMPWalk          = "snmp_walk"
	ModeIFMibTable        = "if_mib_table"
	ModeIFMibStatus       = "if_mib_status"
	ModeIFMibPPPoE        = "if_mib_pppoe"
	ModeOpticalSFPParse   = "optical_sfp_table"
	ModeOpticalSFPColumn  = "optical_sfp_column"
)

const (
	TargetTelemetry   = "telemetry"
	TargetInterfaces  = "interfaces"
)

// MetricDef configuração de uma métrica MikroTik.
type MetricDef struct {
	Enabled      bool   `json:"enabled"`
	OID          string `json:"oid"`
	CollectMode  string `json:"collect_mode"`
	ValueDivisor int    `json:"value_divisor,omitempty"`
}

// MetricsConfig mapa chave → definição.
type MetricsConfig map[string]MetricDef

// CatalogEntry metadados para UI e coleta.
type CatalogEntry struct {
	Key             string   `json:"key"`
	Section         string   `json:"section"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Placeholder     string   `json:"placeholder"`
	CollectModes    []string `json:"collect_modes"`
	DefaultMode     string   `json:"default_mode"`
	WalkTarget      string   `json:"walk_target"`
	Unit            string   `json:"unit,omitempty"`
	DefaultDivisor  int      `json:"default_divisor,omitempty"`
	ShowDivisor     bool     `json:"show_divisor,omitempty"`
	IFMibColumn     int      `json:"if_mib_column,omitempty"`
	OpticalColumn   int      `json:"optical_column,omitempty"`
}

var SectionLabels = map[string]string{
	"system":     "System",
	"health":     "Health",
	"interfaces": "Interfaces",
	"optical":    "Óptica / SFP",
	"wireless":   "Wireless",
	"ppp":       "PPP / Sessões",
	"users":     "Users (Hotspot)",
	"dhcp":      "DHCP",
	"ip":        "IP",
}

// MetricCatalog catálogo completo de métricas configuráveis.
var MetricCatalog = []CatalogEntry{
	// System
	{Key: "sys_descr", Section: "system", Label: "Descrição (sysDescr)", Description: "MIB-2 — descrição do sistema.", Placeholder: "1.3.6.1.2.1.1.1.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "sys_uptime", Section: "system", Label: "Uptime", Description: "Tempo ligado desde o último boot.", Placeholder: "1.3.6.1.2.1.1.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "ticks"},
	{Key: "sys_name", Section: "system", Label: "Nome (sysName)", Description: "Nome configurado do equipamento.", Placeholder: "1.3.6.1.2.1.1.5.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "serial_number", Section: "system", Label: "Número de série", Description: "Serial RouterBOARD (mtxrSerialNumber).", Placeholder: "1.3.6.1.4.1.14988.1.1.7.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "firmware_version", Section: "system", Label: "Versão firmware", Description: "Versão RouterOS em execução.", Placeholder: "1.3.6.1.4.1.14988.1.1.7.4.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "board_name", Section: "system", Label: "Board / modelo", Description: "Nome da placa (mtxrBoardName).", Placeholder: "1.3.6.1.4.1.14988.1.1.7.9.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "license_level", Section: "system", Label: "Nível licença", Description: "Nível da chave RouterOS.", Placeholder: "1.3.6.1.4.1.14988.1.1.4.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},

	// Health
	{Key: "cpu_load", Section: "health", Label: "CPU (%)", Description: "Carga CPU MikroTik (mtxrHlCpuLoad em dispositivos reais). Valor SNMP frequentemente ×10 (ex.: 450 → 45% com divisor 10).", Placeholder: "1.3.6.1.4.1.14988.1.1.3.10.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "%", DefaultDivisor: 10, ShowDivisor: true},
	{Key: "cpu_hr", Section: "health", Label: "CPU HOST-RESOURCES", Description: "hrProcessorLoad (alternativa universal).", Placeholder: "1.3.6.1.2.1.25.3.3.1.2.1", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "%", ShowDivisor: true},
	{Key: "memory_total", Section: "health", Label: "Memória total", Description: "Tamanho total de RAM (hrStorage ou hrMemorySize).", Placeholder: "1.3.6.1.2.1.25.2.3.1.5.65536", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "KB"},
	{Key: "memory_used", Section: "health", Label: "Memória usada", Description: "RAM em uso (hrStorageUsed).", Placeholder: "1.3.6.1.2.1.25.2.3.1.6.65536", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "KB"},
	{Key: "temperature", Section: "health", Label: "Temperatura", Description: "Sensor principal (dispositivos MikroTik usam frequentemente …3.14.0). Valor SNMP frequentemente ×10 (ex.: 350 → 35 °C com divisor 10).", Placeholder: "1.3.6.1.4.1.14988.1.1.3.14.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "°C", DefaultDivisor: 10, ShowDivisor: true},
	{Key: "board_temperature", Section: "health", Label: "Temp. placa", Description: "Temperatura da placa.", Placeholder: "1.3.6.1.4.1.14988.1.1.3.7.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "°C", DefaultDivisor: 10, ShowDivisor: true},
	{Key: "cpu_temperature", Section: "health", Label: "Temp. CPU", Description: "Temperatura junto ao CPU.", Placeholder: "1.3.6.1.4.1.14988.1.1.3.6.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "°C", DefaultDivisor: 10, ShowDivisor: true},
	{Key: "voltage", Section: "health", Label: "Voltagem", Description: "Voltagem principal (ex.: SNMP 237 → 23,7 V com divisor 10).", Placeholder: "1.3.6.1.4.1.14988.1.1.3.8.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "V", DefaultDivisor: 10, ShowDivisor: true},
	{Key: "power", Section: "health", Label: "Potência", Description: "Consumo em watts.", Placeholder: "1.3.6.1.4.1.14988.1.1.3.12.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "W"},
	{Key: "fan_speed", Section: "health", Label: "Velocidade ventoinha", Description: "RPM ventoinha 1.", Placeholder: "1.3.6.1.4.1.14988.1.1.3.17.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry, Unit: "rpm"},
	{Key: "health_gauge_table", Section: "health", Label: "Tabela de sensores (gauges)", Description: "Walk na mtxrGaugeTable — sensores dinâmicos por hardware.", Placeholder: "1.3.6.1.4.1.14988.1.1.3.100.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},

	// Interfaces
	{Key: "if_mib_table", Section: "interfaces", Label: "IF-MIB (ifTable)", Description: "Walk ifTable — nome, status, contadores por interface.", Placeholder: "1.3.6.1.2.1.2.2.1", CollectModes: []string{ModeSNMPWalk, ModeIFMibTable}, DefaultMode: ModeIFMibTable, WalkTarget: TargetInterfaces},
	{Key: "if_x_table", Section: "interfaces", Label: "IF-MIB estendido (ifXTable)", Description: "Walk ifXTable — nomes longos e contadores 64-bit.", Placeholder: "1.3.6.1.2.1.31.1.1.1", CollectModes: []string{ModeSNMPWalk, ModeIFMibTable}, DefaultMode: ModeIFMibTable, WalkTarget: TargetInterfaces},
	{Key: "if_oper_status", Section: "interfaces", Label: "Status operacional (ifOperStatus)", Description: "Estado da interface: up, down, dormant, etc. (IF-MIB col. 8). Valores: 1=up, 2=down, 3=testing, 4=unknown, 5=dormant, 6=notPresent, 7=lowerLayerDown.", Placeholder: IFOperStatusOID, CollectModes: []string{ModeIFMibStatus, ModeSNMPWalk, ModeIFMibTable}, DefaultMode: ModeIFMibStatus, WalkTarget: TargetInterfaces, IFMibColumn: IFColOperStatus},
	{Key: "if_admin_status", Section: "interfaces", Label: "Status administrativo (ifAdminStatus)", Description: "Estado admin da interface (IF-MIB col. 7). 1=up, 2=down, 3=testing.", Placeholder: IFAdminStatusOID, CollectModes: []string{ModeIFMibStatus, ModeSNMPWalk}, DefaultMode: ModeIFMibStatus, WalkTarget: TargetInterfaces, IFMibColumn: IFColAdminStatus},
	{Key: "interface_stats", Section: "interfaces", Label: "Estatísticas MikroTik (mtxrInterfaceStats)", Description: "Contadores detalhados por interface.", Placeholder: "1.3.6.1.4.1.14988.1.1.14.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetInterfaces},

	// Óptica / SFP (mtxrOpticalTable — colunas confirmadas em walk RouterOS)
	{Key: "optical_table", Section: "optical", Label: "Tabela óptica completa (SFP)", Description: "Coleta estruturada mtxrOpticalTable — parse por porta SFP (RX, TX, temp, voltagem, bias). Use o tipo «Tabela SFP parseada».", Placeholder: OpticalTableBaseOID, CollectModes: []string{ModeOpticalSFPParse, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPParse, WalkTarget: TargetInterfaces},
	{Key: "optical_name", Section: "optical", Label: "Nome da porta SFP", Description: "Coluna 2 — mtxrOpticalName. Tipo «Coluna SFP (derivada)» usa a tabela completa; «SNMP WALK» faz walk só desta coluna.", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.2", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, OpticalColumn: OptColName},
	{Key: "optical_rx_loss", Section: "optical", Label: "RX loss (alarme)", Description: "Coluna 3 — perda de sinal RX (0/1).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.3", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, OpticalColumn: OptColRxLoss},
	{Key: "optical_tx_fault", Section: "optical", Label: "TX fault (alarme)", Description: "Coluna 4 — falha TX (0/1).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.4", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, OpticalColumn: OptColTxFault},
	{Key: "optical_wavelength", Section: "optical", Label: "Comprimento de onda", Description: "Coluna 5 — nm (valor SNMP ÷ 100).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.5", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "nm", DefaultDivisor: 100, ShowDivisor: true, OpticalColumn: OptColWavelength},
	{Key: "optical_temperature", Section: "optical", Label: "Temperatura SFP", Description: "Coluna 6 — °C (ex.: SNMP 53 → 53 °C).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.6", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "°C", DefaultDivisor: 1, ShowDivisor: true, OpticalColumn: OptColTemperature},
	{Key: "optical_supply_voltage", Section: "optical", Label: "Voltagem alimentação SFP", Description: "Coluna 7 — volts (ex.: SNMP 3159 → 3,159 V com divisor 1000).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.7", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "V", DefaultDivisor: 1000, ShowDivisor: true, OpticalColumn: OptColSupplyVoltage},
	{Key: "optical_bias_current", Section: "optical", Label: "Corrente bias (TX)", Description: "Coluna 8 — mA (ex.: SNMP 28 → 28 mA).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.8", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "mA", DefaultDivisor: 1, ShowDivisor: true, OpticalColumn: OptColTxBias},
	{Key: "optical_tx_power", Section: "optical", Label: "Potência TX (dBm)", Description: "Coluna 9 — dBm (ex.: SNMP -5025 → -5,025 dBm com divisor 1000).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.9", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "dBm", DefaultDivisor: 1000, ShowDivisor: true, OpticalColumn: OptColTxPower},
	{Key: "optical_rx_power", Section: "optical", Label: "Potência RX (dBm)", Description: "Coluna 10 — dBm (ex.: SNMP -7637 → -7,637 dBm com divisor 1000).", Placeholder: "1.3.6.1.4.1.14988.1.1.19.1.1.10", CollectModes: []string{ModeOpticalSFPColumn, ModeSNMPWalk}, DefaultMode: ModeOpticalSFPColumn, WalkTarget: TargetInterfaces, Unit: "dBm", DefaultDivisor: 1000, ShowDivisor: true, OpticalColumn: OptColRxPower},

	// Wireless
	{Key: "wireless_registration", Section: "wireless", Label: "Clientes wireless (legado)", Description: "mtxrWlRtabTable — clientes AP modo legado.", Placeholder: "1.3.6.1.4.1.14988.1.1.1.2.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "wireless_ap", Section: "wireless", Label: "AP mode (legado)", Description: "mtxrWlApTable — SSID, client count, CCQ.", Placeholder: "1.3.6.1.4.1.14988.1.1.1.3.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "wireless_station", Section: "wireless", Label: "Station mode (legado)", Description: "mtxrWlStatTable — cliente conectado a AP remoto.", Placeholder: "1.3.6.1.4.1.14988.1.1.1.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "wifi_registration", Section: "wireless", Label: "WifiWave2 registration", Description: "mtxrWifiRegistrationTable — 802.11ax / WifiWave.", Placeholder: "1.3.6.1.4.1.14988.1.1.21.4.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "wireless_client_count", Section: "wireless", Label: "Contagem clientes wireless", Description: "Escalar mtxrWlRtabEntryCount.", Placeholder: "1.3.6.1.4.1.14988.1.1.1.4.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},

	// PPP
	{Key: "ppp_sessions_count", Section: "ppp", Label: "Total sessões PPP/Hotspot", Description: "CISCO-AAA-SESSION-MIB — casnActiveTableEntries.", Placeholder: "1.3.6.1.4.1.9.9.150.1.1.1.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "ppp_sessions_table", Section: "ppp", Label: "Tabela sessões ativas (AAA)", Description: "Walk casnActiveTable — user, IP, bytes por sessão (quando suportado).", Placeholder: "1.3.6.1.4.1.9.9.150.1.1.2.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "pppoe_active_sessions", Section: "ppp", Label: "Sessões PPPoE activas (IF-MIB)", Description: "Equivalente a «snmpwalk 1.3.6.1.2.1.2.2.1.2» e filtrar «pppoe»: só interfaces dinâmicas ligadas. Clientes cadastrados offline não aparecem (use «Filas simples» para a lista em queue).", Placeholder: IFTableBaseOID, CollectModes: []string{ModeIFMibPPPoE, ModeSNMPWalk}, DefaultMode: ModeIFMibPPPoE, WalkTarget: TargetTelemetry},
	{Key: "ppp_queue_table", Section: "ppp", Label: "Filas simples (mtxrQueueSimple)", Description: "Clientes em simple queue — inclui cadastrados offline (IP 0.0.0.0). Não é a mesma coisa que sessões PPPoE activas.", Placeholder: "1.3.6.1.4.1.14988.1.1.2.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},

	// Users (Hotspot)
	{Key: "hotspot_users", Section: "users", Label: "Usuários Hotspot ativos", Description: "mtxrHotspotActiveUsersTable.", Placeholder: "1.3.6.1.4.1.14988.1.1.5.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},

	// DHCP
	{Key: "dhcp_lease_count", Section: "dhcp", Label: "Contagem leases DHCP", Description: "mtxrDHCPLeaseCount (MikroTik).", Placeholder: "1.3.6.1.4.1.14988.1.1.6.1.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
	{Key: "dhcp_leases_table", Section: "dhcp", Label: "Tabela leases DHCP", Description: "DHCP-SERVER-MIB — IP, MAC, hostname.", Placeholder: "1.3.6.1.2.1.88.1.2.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},

	// IP
	{Key: "ip_addresses", Section: "ip", Label: "Endereços IP", Description: "ipAdEntTable — IPs por interface.", Placeholder: "1.3.6.1.2.1.4.20.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "ip_arp", Section: "ip", Label: "Tabela ARP", Description: "ipNetToMediaTable — vizinhos L2.", Placeholder: "1.3.6.1.2.1.4.22.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "ip_routes", Section: "ip", Label: "Rotas IPv4", Description: "ipRouteTable (legado).", Placeholder: "1.3.6.1.2.1.4.21.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, WalkTarget: TargetTelemetry},
	{Key: "tcp_established", Section: "ip", Label: "TCP established", Description: "Conexões TCP estabelecidas.", Placeholder: "1.3.6.1.2.1.6.9.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, WalkTarget: TargetTelemetry},
}

func DefaultMetrics() MetricsConfig {
	out := make(MetricsConfig, len(MetricCatalog))
	for _, e := range MetricCatalog {
		mode := e.DefaultMode
		if mode == "" {
			mode = ModeSNMPGet
		}
		enabled := e.Key == "cpu_load" || e.Key == "temperature" || e.Key == "sys_uptime" ||
			e.Key == "memory_used" || e.Key == "memory_total" ||
			e.Key == "if_mib_table" || e.Key == "if_x_table" || e.Key == "if_oper_status" ||
			e.Key == "optical_table" || e.Key == "optical_rx_power" || e.Key == "optical_tx_power" ||
			e.Key == "optical_temperature" || e.Key == "optical_supply_voltage" || e.Key == "optical_bias_current" ||
			e.Key == "pppoe_active_sessions"
		div := e.DefaultDivisor
		out[e.Key] = MetricDef{
			Enabled:      enabled,
			OID:          e.Placeholder,
			CollectMode:  mode,
			ValueDivisor: div,
		}
	}
	return out
}

func ParseMetrics(raw []byte) MetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var out MetricsConfig
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out.Normalize()
}

func (c MetricsConfig) Normalize() MetricsConfig {
	if len(c) == 0 {
		return nil
	}
	out := make(MetricsConfig, len(c))
	for k, v := range c {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		v.OID = strings.TrimSpace(v.OID)
		v.CollectMode = strings.TrimSpace(strings.ToLower(v.CollectMode))
		if v.CollectMode == "" {
			v.CollectMode = ModeSNMPGet
		}
		if v.ValueDivisor < 0 {
			v.ValueDivisor = 0
		}
		out[key] = v
	}
	return out
}

// MergeWithDefaults preenche chaves ausentes com defaults do catálogo.
func (c MetricsConfig) MergeWithDefaults() MetricsConfig {
	def := DefaultMetrics()
	out := make(MetricsConfig, len(MetricCatalog))
	for k, v := range def {
		out[k] = v
	}
	for k, v := range c {
		if cur, ok := out[k]; ok {
			if v.OID != "" {
				cur.OID = v.OID
			}
			cur.Enabled = v.Enabled
			if v.CollectMode != "" {
				cur.CollectMode = v.CollectMode
			}
			if v.ValueDivisor > 0 {
				cur.ValueDivisor = v.ValueDivisor
			}
			out[k] = cur
		} else {
			out[k] = v
		}
	}
	// Migração: optical_sfp → optical_table
	if old, ok := c["optical_sfp"]; ok {
		if cur, has := out["optical_table"]; has {
			if old.Enabled {
				cur.Enabled = true
			}
			if strings.TrimSpace(old.OID) != "" && strings.TrimSpace(cur.OID) == OpticalTableBaseOID {
				cur.OID = strings.TrimSpace(old.OID)
			}
			out["optical_table"] = cur
		}
	}
	// Migração: modos antigos → novos tipos de coleta SFP / status IF
	if cur, ok := out["optical_table"]; ok && cur.CollectMode == ModeSNMPWalk {
		cur.CollectMode = ModeOpticalSFPParse
		out["optical_table"] = cur
	}
	for _, e := range MetricCatalog {
		if e.Section != "optical" || e.Key == "optical_table" {
			continue
		}
		cur, ok := out[e.Key]
		if !ok {
			continue
		}
		if cur.CollectMode == ModeSNMPWalk || cur.CollectMode == "" {
			cur.CollectMode = ModeOpticalSFPColumn
			out[e.Key] = cur
		}
	}
	return out
}

func CatalogByKey() map[string]CatalogEntry {
	m := make(map[string]CatalogEntry, len(MetricCatalog))
	for _, e := range MetricCatalog {
		m[e.Key] = e
	}
	return m
}

func CatalogEntryFor(key string) (CatalogEntry, bool) {
	for _, e := range MetricCatalog {
		if e.Key == key {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

func HasEnabledMetrics(c MetricsConfig) bool {
	for _, e := range MetricCatalog {
		def, ok := c[e.Key]
		if !ok {
			continue
		}
		if def.Enabled && strings.TrimSpace(def.OID) != "" {
			return true
		}
	}
	return false
}

func EnabledGetOIDs(c MetricsConfig) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range MetricCatalog {
		def, ok := c[e.Key]
		if !ok || !def.Enabled {
			continue
		}
		oid := strings.TrimSpace(def.OID)
		if oid == "" {
			continue
		}
		mode := def.CollectMode
		if mode == "" {
			mode = e.DefaultMode
		}
		if mode != ModeSNMPGet {
			continue
		}
		if e.WalkTarget != TargetTelemetry {
			continue
		}
		if _, dup := seen[oid]; dup {
			continue
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	return out
}

func InterfaceWalkOIDs(c MetricsConfig) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(oid string) {
		oid = strings.TrimSpace(oid)
		if oid == "" {
			return
		}
		if _, dup := seen[oid]; dup {
			return
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	if root := OpticalWalkRoot(c); root != "" {
		add(root)
	}
	for _, e := range MetricCatalog {
		if e.Section == "optical" {
			continue // já coberto pelo walk único da tabela óptica
		}
		if e.WalkTarget != TargetInterfaces {
			continue
		}
		def, ok := c[e.Key]
		if !ok || !def.Enabled {
			continue
		}
		add(def.OID)
	}
	return out
}

func TelemetryWalkOIDs(c MetricsConfig) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range MetricCatalog {
		if e.WalkTarget != TargetTelemetry {
			continue
		}
		def, ok := c[e.Key]
		if !ok || !def.Enabled {
			continue
		}
		mode := def.CollectMode
		if mode == "" {
			mode = e.DefaultMode
		}
		if mode != ModeSNMPWalk {
			continue
		}
		oid := strings.TrimSpace(def.OID)
		if oid == "" {
			continue
		}
		if _, dup := seen[oid]; dup {
			continue
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	return out
}
