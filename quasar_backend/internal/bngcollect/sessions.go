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
	PortType       string `json:"port_type,omitempty"`
	PortTypeRaw    string `json:"port_type_raw,omitempty"`
	AuthState      string `json:"auth_state,omitempty"`
	AuthStateRaw   string `json:"auth_state_raw,omitempty"`
	AuthorState    string `json:"author_state,omitempty"`
	AuthorStateRaw string `json:"author_state_raw,omitempty"`
	AcctState      string `json:"acct_state,omitempty"`
	AcctStateRaw   string `json:"acct_state_raw,omitempty"`
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

func mapAcctStateLabel(v string) string {
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

func mergeSessionMaps(maps map[string]map[string]string) []SessionRow {
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
			row.Login = v
		}
		if v, ok := maps["access_ipv4"][idx]; ok {
			row.IPv4 = v
		}
		if v, ok := maps["access_mac"][idx]; ok {
			row.MAC = v
		}
		if v, ok := maps["access_ipv6"][idx]; ok {
			row.IPv6 = v
		}
		if v, ok := maps["access_port_type"][idx]; ok {
			row.PortTypeRaw = v
			if isPPPoEPortType(v) {
				row.PortType = "PPPoE"
			} else if v != "" {
				row.PortType = "Tipo " + v
			}
		}
		if v, ok := maps["auth_state"][idx]; ok {
			row.AuthStateRaw = v
			row.AuthState = mapAuthStateLabel(v)
		}
		if v, ok := maps["author_state"][idx]; ok {
			row.AuthorStateRaw = v
			row.AuthorState = mapAuthorStateLabel(v)
		}
		if v, ok := maps["acct_state"][idx]; ok {
			row.AcctStateRaw = v
			row.AcctState = mapAcctStateLabel(v)
		}
		if row.PortTypeRaw != "" && !isPPPoEPortType(row.PortTypeRaw) {
			continue
		}
		out = append(out, row)
	}
	return out
}
