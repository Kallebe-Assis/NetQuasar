package bngcollect

import (
	"strconv"
	"strings"
)

// SessionRow sessão PPPoE normalizada.
type SessionRow struct {
	Index          string `json:"index"`
	Login          string `json:"login,omitempty"`
	IPv4           string `json:"ipv4,omitempty"`
	MAC            string `json:"mac,omitempty"`
	IPv6           string `json:"ipv6,omitempty"`
	IPv6PD         string `json:"ipv6_pd,omitempty"`
	IPType         string `json:"ip_type,omitempty"`
	IPTypeRaw      string `json:"ip_type_raw,omitempty"`
	OnlineTimeSec  string `json:"online_time_sec,omitempty"`
	OnlineTime     string `json:"online_time,omitempty"`
	PortType       string `json:"port_type,omitempty"`
	PortTypeRaw    string `json:"port_type_raw,omitempty"`
	AuthState      string `json:"auth_state,omitempty"`
	AuthStateRaw   string `json:"auth_state_raw,omitempty"`
	AuthorState    string `json:"author_state,omitempty"`
	AuthorStateRaw string `json:"author_state_raw,omitempty"`
	AcctState      string `json:"acct_state,omitempty"`
	AcctStateRaw   string `json:"acct_state_raw,omitempty"`
	VLAN           string `json:"vlan,omitempty"`
	Interface      string `json:"interface,omitempty"`
	Domain         string `json:"domain,omitempty"`
	UpFlowBytes    string `json:"up_flow_bytes,omitempty"`
	DnFlowBytes    string `json:"dn_flow_bytes,omitempty"`
	CarUpCIRKbps   string `json:"car_up_cir_kbps,omitempty"`
	CarDnCIRKbps   string `json:"car_dn_cir_kbps,omitempty"`
	QoSProfile     string `json:"qos_profile,omitempty"`
	Status         string `json:"status"`
}

const (
	portTypePPP = "2"
)

func extractIndexFromOID(oid, base string) string {
	oid = strings.TrimSpace(oid)
	base = strings.TrimPrefix(strings.TrimSpace(base), ".")
	if base == "" || oid == "" {
		return ""
	}
	prefix := base + "."
	if !strings.HasPrefix(oid, prefix) {
		return ""
	}
	return strings.TrimPrefix(oid, prefix)
}

func mapAuthStateLabel(v string) string {
	v = sanitizeSNMPDisplay(v)
	switch strings.TrimSpace(v) {
	case "1":
		return "Inicial"
	case "2":
		return "A autenticar"
	case "3":
		return "Autenticado"
	case "4":
		return "Falha auth"
	default:
		if v == "" {
			return ""
		}
		return "Estado " + v
	}
}

func mapAuthorStateLabel(v string) string {
	v = sanitizeSNMPDisplay(v)
	switch strings.TrimSpace(v) {
	case "1":
		return "Inicial"
	case "2":
		return "A autorizar"
	case "3":
		return "Autorizado"
	case "4":
		return "Falha authz"
	default:
		if v == "" {
			return ""
		}
		return "Estado " + v
	}
}

func formatOnlineTimeSeconds(v string) string {
	n, ok := parseIntMetric(v)
	if !ok || n <= 0 {
		return ""
	}
	return FormatDurationSeconds(n)
}

func mapAcctStateLabel(v string) string {
	v = sanitizeSNMPDisplay(v)
	switch strings.TrimSpace(v) {
	case "1":
		return "Inicial"
	case "2":
		return "A contabilizar"
	case "3":
		return "Contabilizado"
	case "4":
		return "Falha acct"
	default:
		if v == "" {
			return ""
		}
		return "Estado " + v
	}
}

func isPPPoEPortType(v string) bool {
	return strings.TrimSpace(v) == portTypePPP
}

func parseIntMetric(v string) (int, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func mergeSessionMaps(maps map[string]map[string]string, skipPortTypeFilter ...bool) []SessionRow {
	skipPortFilter := len(skipPortTypeFilter) > 0 && skipPortTypeFilter[0]
	if len(maps) == 0 {
		return nil
	}
	indices := make(map[string]struct{})
	for _, m := range maps {
		for idx := range m {
			indices[idx] = struct{}{}
		}
	}
	out := make([]SessionRow, 0, len(indices))
	for idx := range indices {
		row := SessionRow{
			Index:  idx,
			Status: "Up",
		}
		if v, ok := maps["access_login"][idx]; ok {
			row.Login = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_ipv4"][idx]; ok {
			row.IPv4 = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_mac"][idx]; ok {
			row.MAC = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_ipv6"][idx]; ok {
			row.IPv6 = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_ipv6_pd"][idx]; ok {
			row.IPv6PD = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_ip_type"][idx]; ok {
			row.IPTypeRaw = sanitizeSNMPDisplay(v)
			row.IPType = mapIPTypeLabel(row.IPTypeRaw)
		}
		if v, ok := maps["access_online_time"][idx]; ok {
			row.OnlineTimeSec = sanitizeSNMPDisplay(v)
			row.OnlineTime = formatOnlineTimeSeconds(row.OnlineTimeSec)
		}
		if v, ok := maps["access_port_type"][idx]; ok {
			row.PortTypeRaw = sanitizeSNMPDisplay(v)
			if isPPPoEPortType(v) {
				row.PortType = "PPPoE"
			} else if row.PortTypeRaw != "" {
				row.PortType = "Tipo " + row.PortTypeRaw
			}
		}
		if v, ok := maps["auth_state"][idx]; ok {
			row.AuthStateRaw = sanitizeSNMPDisplay(v)
			row.AuthState = mapAuthStateLabel(row.AuthStateRaw)
		}
		if v, ok := maps["author_state"][idx]; ok {
			row.AuthorStateRaw = sanitizeSNMPDisplay(v)
			row.AuthorState = mapAuthorStateLabel(row.AuthorStateRaw)
		}
		if v, ok := maps["acct_state"][idx]; ok {
			row.AcctStateRaw = sanitizeSNMPDisplay(v)
			row.AcctState = mapAcctStateLabel(row.AcctStateRaw)
		}
		if v, ok := maps["access_vlan"][idx]; ok {
			row.VLAN = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_interface"][idx]; ok {
			row.Interface = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_domain"][idx]; ok {
			row.Domain = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_up_flow"][idx]; ok {
			row.UpFlowBytes = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_dn_flow"][idx]; ok {
			row.DnFlowBytes = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_car_up_cir"][idx]; ok {
			row.CarUpCIRKbps = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_car_dn_cir"][idx]; ok {
			row.CarDnCIRKbps = sanitizeSNMPDisplay(v)
		}
		if v, ok := maps["access_qos_profile"][idx]; ok {
			row.QoSProfile = sanitizeSNMPDisplay(v)
		}
		if !skipPortFilter && row.PortTypeRaw != "" && !isPPPoEPortType(row.PortTypeRaw) {
			continue
		}
		finalizeSessionRow(&row)
		out = append(out, row)
	}
	return out
}
