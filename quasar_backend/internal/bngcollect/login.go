package bngcollect

import (
	"encoding/hex"
	"strings"
)

// decodeColonHexASCII converte login SNMP em hex («6970:6570:616d…» ou «69:70:65:61…») para texto ASCII.
func decodeColonHexASCII(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || !strings.Contains(s, ":") {
		return ""
	}
	if strings.Contains(s, "@") {
		return ""
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return ""
	}
	raw := make([]byte, 0, len(parts)*2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch len(p) {
		case 2:
			b, err := hex.DecodeString(p)
			if err != nil || len(b) != 1 {
				return ""
			}
			raw = append(raw, b[0])
		case 4:
			b, err := hex.DecodeString(p)
			if err != nil || len(b) != 2 {
				return ""
			}
			raw = append(raw, b...)
		default:
			return ""
		}
	}
	if len(raw) < 4 {
		return ""
	}
	for _, b := range raw {
		if b < 32 || b > 126 {
			return ""
		}
	}
	out := strings.TrimSpace(string(raw))
	if out == "" || strings.Count(out, "@") > 1 {
		return ""
	}
	return out
}

// NormalizeSNMPLoginValue decodifica login SNMP (hex ASCII) e remove sufixo RADIUS.
func NormalizeSNMPLoginValue(raw, stripSuffix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if decoded := decodeColonHexASCII(raw); decoded != "" {
		raw = decoded
	}
	return NormalizePPPoELogin(raw, stripSuffix)
}

// CollectionOptions opções globais da coleta BNG (não-OID).
type CollectionOptions struct {
	PPPoELoginStripSuffix string   `json:"pppoe_login_strip_suffix,omitempty"`
	UplinkInterfaces      []string `json:"uplink_interfaces,omitempty"`
}

func normalizeStripSuffix(suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return ""
	}
	if !strings.HasPrefix(suffix, "@") {
		suffix = "@" + suffix
	}
	return strings.ToLower(suffix)
}

// NormalizePPPoELogin remove sufixo RADIUS configurado (ex.: @g2.com.br) para exibição e pesquisa.
func NormalizePPPoELogin(raw, stripSuffix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sfx := normalizeStripSuffix(stripSuffix)
	if sfx == "" {
		return raw
	}
	lower := strings.ToLower(raw)
	if strings.HasSuffix(lower, sfx) {
		return strings.TrimSpace(raw[:len(raw)-len(sfx)])
	}
	return raw
}

// MatchPPPoELogin compara login pesquisado com valor SNMP (com normalização de sufixo).
func MatchPPPoELogin(search, snmpValue, stripSuffix string) bool {
	search = strings.TrimSpace(search)
	snmpValue = strings.TrimSpace(snmpValue)
	if search == "" || snmpValue == "" {
		return false
	}
	ns := strings.ToLower(NormalizeSNMPLoginValue(search, stripSuffix))
	nv := strings.ToLower(NormalizeSNMPLoginValue(snmpValue, stripSuffix))
	if ns == nv {
		return true
	}
	return strings.Contains(nv, ns) || strings.Contains(ns, nv)
}

// PPPoELoginLookupTargets gera variantes para procurar no SNMP (com/sem sufixo).
func PPPoELoginLookupTargets(search, stripSuffix string) []string {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[strings.ToLower(v)]; ok {
			return
		}
		seen[strings.ToLower(v)] = struct{}{}
		out = append(out, v)
	}
	add(search)
	add(NormalizePPPoELogin(search, stripSuffix))
	sfx := normalizeStripSuffix(stripSuffix)
	if sfx != "" && !strings.Contains(strings.ToLower(search), sfx) {
		add(search + sfx)
		if !strings.HasPrefix(sfx, "@") {
			add(search + "@" + strings.TrimPrefix(sfx, "@"))
		}
	}
	return out
}

func ApplyLoginStripToSessions(sessions []SessionRow, stripSuffix string) []SessionRow {
	for i := range sessions {
		if sessions[i].Login != "" {
			sessions[i].Login = NormalizeSNMPLoginValue(sessions[i].Login, stripSuffix)
		}
	}
	return sessions
}
