package bngcollect

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatDurationSeconds formata segundos em minutos, horas ou dias.
func FormatDurationSeconds(sec int) string {
	if sec <= 0 {
		return ""
	}
	days := sec / 86400
	hours := (sec % 86400) / 3600
	mins := (sec % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%d d %d h", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d h %d min", hours, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%d min", mins)
	}
	return fmt.Sprintf("%d s", sec)
}

// FormatKbitRate formata taxa CIR Huawei (preferência Mbps para planos típicos).
func FormatKbitRate(kbps int) string {
	if kbps <= 0 {
		return "Sem limite"
	}
	bps := cirBitsPerSecond(kbps)
	mbps := bps / 1_000_000
	if mbps >= 100 {
		return fmt.Sprintf("%.0f Mbps", mbps)
	}
	if mbps >= 10 {
		return fmt.Sprintf("%.1f Mbps", mbps)
	}
	if mbps >= 1 {
		return fmt.Sprintf("%.2f Mbps", mbps)
	}
	return FormatBitrateBps(int64(bps))
}

// FormatFlow64Volume formata contador Huawei hwAccess*Flow64 (unidades de 64 bytes).
func FormatFlow64Volume(raw string) string {
	n, ok := parseInt64Metric(raw)
	if !ok || n < 0 {
		return ""
	}
	bytes := float64(n) * 64
	return formatByteVolume(bytes)
}

func formatByteVolume(bytes float64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := bytes
	i := 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	digits := 0
	if v < 10 {
		digits = 2
	} else if v < 100 {
		digits = 1
	}
	return fmt.Sprintf("%.*f %s", digits, v, units[i])
}

func parseInt64Metric(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// sanitizeSNMPDisplay normaliza valores SNMP vazios ou «<nil>» para string vazia.
func sanitizeSNMPDisplay(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "<nil>" || strings.EqualFold(s, "null") || strings.EqualFold(s, "nil") {
		return ""
	}
	return s
}

func sanitizeSessionRowFields(row *SessionRow) {
	if row == nil {
		return
	}
	row.Index = sanitizeSNMPDisplay(row.Index)
	row.Login = sanitizeSNMPDisplay(row.Login)
	row.IPv4 = sanitizeSNMPDisplay(row.IPv4)
	row.MAC = sanitizeSNMPDisplay(row.MAC)
	row.IPv6 = sanitizeSNMPDisplay(row.IPv6)
	row.IPv6PD = sanitizeSNMPDisplay(row.IPv6PD)
	row.IPType = sanitizeSNMPDisplay(row.IPType)
	row.IPTypeRaw = sanitizeSNMPDisplay(row.IPTypeRaw)
	row.OnlineTimeSec = sanitizeSNMPDisplay(row.OnlineTimeSec)
	row.OnlineTime = sanitizeSNMPDisplay(row.OnlineTime)
	row.PortType = sanitizeSNMPDisplay(row.PortType)
	row.PortTypeRaw = sanitizeSNMPDisplay(row.PortTypeRaw)
	row.AuthState = sanitizeSNMPDisplay(row.AuthState)
	row.AuthStateRaw = sanitizeSNMPDisplay(row.AuthStateRaw)
	row.AuthorState = sanitizeSNMPDisplay(row.AuthorState)
	row.AuthorStateRaw = sanitizeSNMPDisplay(row.AuthorStateRaw)
	row.AcctState = sanitizeSNMPDisplay(row.AcctState)
	row.AcctStateRaw = sanitizeSNMPDisplay(row.AcctStateRaw)
	row.VLAN = sanitizeSNMPDisplay(row.VLAN)
	row.Interface = sanitizeSNMPDisplay(row.Interface)
	row.Domain = sanitizeSNMPDisplay(row.Domain)
	row.UpFlowBytes = sanitizeSNMPDisplay(row.UpFlowBytes)
	row.DnFlowBytes = sanitizeSNMPDisplay(row.DnFlowBytes)
	row.CarUpCIRKbps = sanitizeSNMPDisplay(row.CarUpCIRKbps)
	row.CarDnCIRKbps = sanitizeSNMPDisplay(row.CarDnCIRKbps)
	row.QoSProfile = sanitizeSNMPDisplay(row.QoSProfile)
	row.Status = sanitizeSNMPDisplay(row.Status)
}

// EnrichSessionRow adiciona campos formatados para API/UI.
func EnrichSessionRow(row SessionRow) map[string]any {
	sanitizeSessionRowFields(&row)
	finalizeSessionRow(&row)
	onlineSec, _ := parseIntMetric(row.OnlineTimeSec)
	upCIR, _ := parseIntMetric(row.CarUpCIRKbps)
	dnCIR, _ := parseIntMetric(row.CarDnCIRKbps)

	out := map[string]any{
		"index":            row.Index,
		"login":            row.Login,
		"ipv4":             row.IPv4,
		"mac":              row.MAC,
		"ipv6":             row.IPv6,
		"ipv6_pd":          row.IPv6PD,
		"ip_type":          row.IPType,
		"ip_type_raw":      row.IPTypeRaw,
		"online_time_sec":  row.OnlineTimeSec,
		"online_time":      FormatDurationSeconds(onlineSec),
		"port_type":        row.PortType,
		"port_type_raw":    row.PortTypeRaw,
		"auth_state":       row.AuthState,
		"auth_state_raw":   row.AuthStateRaw,
		"author_state":     row.AuthorState,
		"author_state_raw": row.AuthorStateRaw,
		"acct_state":       row.AcctState,
		"acct_state_raw":   row.AcctStateRaw,
		"vlan":             row.VLAN,
		"interface":        row.Interface,
		"domain":           row.Domain,
		"up_flow_bytes":    row.UpFlowBytes,
		"dn_flow_bytes":    row.DnFlowBytes,
		"up_flow_display":  FormatFlow64Volume(row.UpFlowBytes),
		"dn_flow_display":  FormatFlow64Volume(row.DnFlowBytes),
		"car_up_cir_kbps":  row.CarUpCIRKbps,
		"car_dn_cir_kbps":  row.CarDnCIRKbps,
		"car_up_cir_display": FormatKbitRate(upCIR),
		"car_dn_cir_display": FormatKbitRate(dnCIR),
		"qos_profile":      row.QoSProfile,
		"status":           row.Status,
	}
	if out["online_time"] == "" && row.OnlineTime != "" {
		out["online_time"] = row.OnlineTime
	}
	return out
}

func mapIntField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case string:
		n, _ := parseIntMetric(v)
		return n
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

// EnrichSessionMap normaliza mapas vindos de snapshot JSON.
func EnrichSessionMap(m map[string]any) map[string]any {
	if m == nil {
		return m
	}
	row := SessionRow{
		Index:          sanitizeSNMPDisplay(fmt.Sprint(m["index"])),
		Login:          sanitizeSNMPDisplay(fmt.Sprint(m["login"])),
		IPv4:           sanitizeSNMPDisplay(fmt.Sprint(m["ipv4"])),
		MAC:            sanitizeSNMPDisplay(fmt.Sprint(m["mac"])),
		IPv6:           sanitizeSNMPDisplay(fmt.Sprint(m["ipv6"])),
		IPv6PD:         sanitizeSNMPDisplay(fmt.Sprint(m["ipv6_pd"])),
		IPType:         sanitizeSNMPDisplay(fmt.Sprint(m["ip_type"])),
		IPTypeRaw:      sanitizeSNMPDisplay(fmt.Sprint(m["ip_type_raw"])),
		OnlineTimeSec:  sanitizeSNMPDisplay(fmt.Sprint(m["online_time_sec"])),
		OnlineTime:     sanitizeSNMPDisplay(fmt.Sprint(m["online_time"])),
		CarUpCIRKbps:   sanitizeSNMPDisplay(fmt.Sprint(m["car_up_cir_kbps"])),
		CarDnCIRKbps:   sanitizeSNMPDisplay(fmt.Sprint(m["car_dn_cir_kbps"])),
	}
	sanitizeSessionRowFields(&row)
	finalizeSessionRow(&row)
	m["ipv4"] = row.IPv4
	m["mac"] = row.MAC
	m["ipv6"] = row.IPv6
	m["ipv6_pd"] = row.IPv6PD
	m["ip_type"] = row.IPType

	onlineSec, _ := parseIntMetric(row.OnlineTimeSec)
	if onlineSec <= 0 {
		onlineSec = mapIntField(m, "access_online_time")
	}
	if onlineSec > 0 {
		m["online_time_sec"] = fmt.Sprintf("%d", onlineSec)
		m["online_time"] = FormatDurationSeconds(onlineSec)
	} else if s, ok := m["online_time"].(string); !ok || strings.TrimSpace(s) == "" {
		m["online_time"] = ""
	}

	upCIR, _ := parseIntMetric(row.CarUpCIRKbps)
	dnCIR, _ := parseIntMetric(row.CarDnCIRKbps)
	if upCIR <= 0 {
		upCIR = mapIntField(m, "car_up_cir_kbps")
	}
	if dnCIR <= 0 {
		dnCIR = mapIntField(m, "car_dn_cir_kbps")
	}
	if upCIR > 0 {
		m["car_up_cir_display"] = FormatKbitRate(upCIR)
	} else {
		m["car_up_cir_display"] = FormatKbitRate(0)
	}
	if dnCIR > 0 {
		m["car_dn_cir_display"] = FormatKbitRate(dnCIR)
	} else {
		m["car_dn_cir_display"] = FormatKbitRate(0)
	}

	if raw, ok := m["up_flow_bytes"]; ok && fmt.Sprint(raw) != "" {
		m["up_flow_display"] = FormatFlow64Volume(fmt.Sprint(raw))
	}
	if raw, ok := m["dn_flow_bytes"]; ok && fmt.Sprint(raw) != "" {
		m["dn_flow_display"] = FormatFlow64Volume(fmt.Sprint(raw))
	}
	return m
}

func EnrichSessionMaps(list []map[string]any) []map[string]any {
	for i := range list {
		list[i] = EnrichSessionMap(list[i])
	}
	return list
}
