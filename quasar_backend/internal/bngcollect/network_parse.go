package bngcollect

import (
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// cirBitsPerSecond interpreta hwAccessCAR*CIR (MIB: kbit/s; firmwares variam).
func cirBitsPerSecond(raw int) float64 {
	if raw <= 0 {
		return 0
	}
	if raw >= 10_000_000 {
		return float64(raw)
	}
	if raw < 10_000 {
		return float64(raw) * 1_000_000
	}
	return float64(raw) * 1000
}

func mapIPTypeLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	b := []byte(v)
	if len(b) >= 1 && len(b) <= 3 {
		if isHuaweiIPTypeFlags(b) {
			return huaweiIPTypeFromFlags(b)
		}
	}
	if len(v) == 3 && v[0] >= '0' && v[0] <= '1' && v[1] >= '0' && v[1] <= '1' {
		return huaweiIPTypeFromASCIIFlags(v)
	}
	switch v {
	case "1", "01", "10":
		return "ipv4"
	case "2", "02", "11":
		return "ipv6"
	case "3", "03":
		return "ipv4/v6"
	default:
		lower := strings.ToLower(v)
		if strings.Contains(lower, "dual") || strings.Contains(lower, "v4/v6") {
			return "ipv4/v6"
		}
		if strings.Contains(lower, "ipv6") {
			return "ipv6"
		}
		if strings.Contains(lower, "ipv4") {
			return "ipv4"
		}
		if v != "" {
			return "tipo " + v
		}
		return ""
	}
}

func isHuaweiIPTypeFlags(b []byte) bool {
	for _, c := range b {
		if c != 0 && c != 1 {
			return false
		}
	}
	return true
}

func huaweiIPTypeFromFlags(b []byte) string {
	v4 := len(b) > 0 && b[0] == 1
	v6 := len(b) > 1 && b[1] == 1
	if v4 && v6 {
		return "ipv4/v6"
	}
	if v6 {
		return "ipv6"
	}
	if v4 {
		return "ipv4"
	}
	return ""
}

func huaweiIPTypeFromASCIIFlags(v string) string {
	v4 := v[0] == '1'
	v6 := v[1] == '1'
	if v4 && v6 {
		return "ipv4/v6"
	}
	if v6 {
		return "ipv6"
	}
	if v4 {
		return "ipv4"
	}
	return ""
}

func deriveIPTypeFromAddresses(ipv4, ipv6, ipv6pd string) string {
	has4 := strings.TrimSpace(ipv4) != "" && strings.TrimSpace(ipv4) != "0.0.0.0"
	has6 := isUsableIPv6(ipv6) || isUsableIPv6(ipv6pd)
	if has4 && has6 {
		return "ipv4/v6"
	}
	if has6 {
		return "ipv6"
	}
	if has4 {
		return "ipv4"
	}
	return ""
}

func isUsableIPv6(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "::" {
		return false
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To16() != nil && !ip.IsUnspecified()
}

func normalizeIPv6Address(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if ip := net.ParseIP(s); ip != nil && ip.To16() != nil {
		if ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	}
	if prefix := parseColonHexIPv6Prefix(s); prefix != "" {
		return prefix
	}
	if parsed := parseColonByteHexIPv6(s); parsed != "" {
		return parsed
	}
	h := strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(s, " ", ""), "0x"))
	h = strings.ReplaceAll(h, ":", "")
	if len(h)%2 != 0 {
		return s
	}
	raw, err := hex.DecodeString(h)
	if err != nil {
		return s
	}
	switch len(raw) {
	case 16:
		ip := net.IP(raw)
		if ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	case 17, 18:
		// Alguns firmwares Huawei prefixam tipo/tamanho antes dos 16 octetos.
		ip := net.IP(raw[len(raw)-16:])
		if ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	case 4:
		return net.IP(raw).String()
	default:
		return s
	}
}

func parseColonByteHexIPv6(s string) string {
	if !strings.Contains(s, ":") {
		return ""
	}
	parts := strings.Split(s, ":")
	if len(parts) == 16 {
		raw := make([]byte, 16)
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if len(p) != 2 {
				return ""
			}
			b, err := hex.DecodeString(p)
			if err != nil || len(b) != 1 {
				return ""
			}
			raw[i] = b[0]
		}
		ip := net.IP(raw)
		if ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	}
	return ""
}

// parseColonHexIPv6Prefix interpreta prefixos Huawei «28:04:4d:68:04:51:fb» (len + octetos).
func parseColonHexIPv6Prefix(s string) string {
	if !strings.Contains(s, ":") {
		return ""
	}
	parts := strings.Split(s, ":")
	if len(parts) < 3 || len(parts) > 9 {
		return ""
	}
	raw := make([]byte, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) != 2 {
			return ""
		}
		b, err := hex.DecodeString(p)
		if err != nil || len(b) != 1 {
			return ""
		}
		raw = append(raw, b[0])
	}
	if len(raw) < 2 {
		return ""
	}
	plen := int(raw[0])
	if plen >= 8 && plen <= 128 && len(raw) >= 2 {
		addr := make([]byte, 16)
		copy(addr, raw[1:])
		ip := net.IP(addr)
		if !ip.IsUnspecified() {
			return ip.String() + "/" + strconv.Itoa(plen)
		}
	}
	if len(raw) == 8 {
		ip := net.IP(append(raw, make([]byte, 8)...))
		if !ip.IsUnspecified() {
			return ip.String() + "/64"
		}
	}
	return ""
}

func normalizeMACAddress(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.Count(lower, ":") == 5 {
		return lower
	}
	h := strings.ReplaceAll(lower, ":", "")
	h = strings.ReplaceAll(h, "-", "")
	h = strings.ReplaceAll(h, ".", "")
	if len(h) != 12 {
		return s
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return s
		}
	}
	var parts []string
	for i := 0; i < 12; i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

func finalizeSessionRow(row *SessionRow) {
	row.IPv4 = strings.TrimSpace(row.IPv4)
	row.IPv6 = normalizeIPv6Address(row.IPv6)
	row.IPv6PD = normalizeIPv6Address(row.IPv6PD)
	row.MAC = normalizeMACAddress(row.MAC)
	if derived := deriveIPTypeFromAddresses(row.IPv4, row.IPv6, row.IPv6PD); derived != "" {
		row.IPType = derived
	} else if row.IPType == "" && row.IPTypeRaw != "" {
		row.IPType = mapIPTypeLabel(row.IPTypeRaw)
	}
}
