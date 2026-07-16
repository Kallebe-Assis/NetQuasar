package monitorview

// HealthStatus representa saúde de uma coleta SNMP/monitoramento.
type HealthStatus string

const (
	HealthUnknown HealthStatus = "unknown"
	HealthOK      HealthStatus = "ok"
	HealthPartial HealthStatus = "partial"
	HealthFailed  HealthStatus = "failed"
)

// Reachability estado de ping/ICMP do equipamento.
type Reachability struct {
	Online         bool    `json:"online"`
	LatencyMs      *int64  `json:"latency_ms,omitempty"`
	Method         string  `json:"method,omitempty"`
	PingFailStreak int     `json:"ping_fail_streak,omitempty"`
	CheckedAt      string  `json:"checked_at,omitempty"`
}

// DeviceKPIs métricas compactas para cards e tabelas de monitoramento.
type DeviceKPIs struct {
	CPUPercent    *float64 `json:"cpu_percent,omitempty"`
	MemoryPercent *float64 `json:"memory_percent,omitempty"`
	TemperatureC  *float64 `json:"temperature_c,omitempty"`
	Uptime        string   `json:"uptime,omitempty"`
	CollectedAt   string   `json:"collected_at,omitempty"`
}

// InterfaceRow linha normalizada de interface para tabelas e gráficos.
type InterfaceRow struct {
	Index       int     `json:"if_index"`
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name,omitempty"`
	Type        string  `json:"type,omitempty"`
	AdminStatus string  `json:"admin_status,omitempty"`
	OperStatus  string  `json:"oper_status,omitempty"`
	InBps       float64 `json:"in_bps,omitempty"`
	OutBps      float64 `json:"out_bps,omitempty"`
	RxDbm       *float64 `json:"rx_dbm,omitempty"`
	TxDbm       *float64 `json:"tx_dbm,omitempty"`
	SpeedBps    *float64 `json:"speed_bps,omitempty"`
}

// TrafficPoint ponto para gráfico RX/TX.
type TrafficPoint struct {
	Ts     int64   `json:"ts"`
	RxBps  float64 `json:"rx_bps"`
	TxBps  float64 `json:"tx_bps"`
}

// GaugePoint ponto para gráfico de gauge (CPU, memória).
type GaugePoint struct {
	Ts    int64   `json:"ts"`
	Value float64 `json:"value"`
}

// DeviceView visão unificada de um equipamento para a UI.
type DeviceView struct {
	ID           string       `json:"id"`
	Description  string       `json:"description,omitempty"`
	Category     string       `json:"category,omitempty"`
	Brand        string       `json:"brand,omitempty"`
	IP           string       `json:"ip,omitempty"`
	Reachability Reachability `json:"reachability"`
	KPIs         DeviceKPIs   `json:"kpis"`
	SNMPHealth   HealthStatus `json:"snmp_health_status,omitempty"`
	SNMPReason   string       `json:"snmp_health_reason,omitempty"`
}
