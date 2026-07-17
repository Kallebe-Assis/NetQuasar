package mikrotikcollect

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reNxosIfHeaderInline = regexp.MustCompile(`(?i)^(Ethernet\d+/\d+|Eth\d+/\d+|mgmt\d+|port-channel\d+|Po\d+)\b`)
	reNxosUptimePart     = regexp.MustCompile(`(?i)(\d+)\s*(days?|hours?|minutes?|seconds?)`)
	reNxosDiagValue      = regexp.MustCompile(`(?i)^\s*(Temperature|Voltage|Current|Tx Power|Rx Power)\s+([-+]?\d+(?:\.\d+)?)\s*([A-Za-z]+)?`)
)

// ParseNxosSystemUptime extrai System uptime → formato compacto (145d 00h 17m).
func ParseNxosSystemUptime(output string) map[string]any {
	lines := strings.Split(output, "\n")
	var systemLine string
	for _, line := range lines {
		low := strings.ToLower(line)
		if idx := strings.Index(low, "system uptime:"); idx >= 0 {
			systemLine = strings.TrimSpace(line[idx+len("system uptime:"):])
			break
		}
	}
	if systemLine == "" {
		for _, line := range lines {
			if reNxosUptimePart.MatchString(line) && strings.Contains(strings.ToLower(line), "day") {
				systemLine = strings.TrimSpace(line)
				break
			}
		}
	}
	if systemLine == "" {
		return map[string]any{"uptime": nil, "raw": strings.TrimSpace(output)}
	}
	compact := compactNxosUptime(systemLine)
	if compact == "" {
		return map[string]any{"uptime": strings.TrimSpace(systemLine)}
	}
	return map[string]any{"uptime": compact}
}

func compactNxosUptime(s string) string {
	matches := reNxosUptimePart.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return ""
	}
	var days, hours, mins, secs int
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		unit := strings.ToLower(m[2])
		switch {
		case strings.HasPrefix(unit, "day"):
			days = n
		case strings.HasPrefix(unit, "hour"):
			hours = n
		case strings.HasPrefix(unit, "minute"):
			mins = n
		case strings.HasPrefix(unit, "second"):
			secs = n
		}
	}
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", hours, mins, secs)
	}
	return fmt.Sprintf("%dm %02ds", mins, secs)
}

// ParseNxosHostname interpreta "show hostname".
func ParseNxosHostname(output string) map[string]any {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "hostname") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return map[string]any{"name": parts[len(parts)-1]}
			}
		}
		if !strings.Contains(line, " ") && !strings.Contains(line, "#") && len(line) < 64 {
			return map[string]any{"name": line}
		}
	}
	return map[string]any{"name": nil, "raw": strings.TrimSpace(output)}
}

type nxosInterfaceStatusRow struct {
	Name   string
	Descr  string
	Status string
	Vlan   string
	Duplex string
	Speed  string
	Type   string
}

// ParseNxosInterfaceStatus interpreta show interface status.
func ParseNxosInterfaceStatus(output string) []map[string]any {
	rows := parseNxosInterfaceStatusRows(output)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		running := strings.EqualFold(r.Status, "connected")
		out = append(out, map[string]any{
			"name":        r.Name,
			"descr":       r.Descr,
			"status":      r.Status,
			"vlan":        r.Vlan,
			"duplex":      r.Duplex,
			"speed":       r.Speed,
			"type":        r.Type,
			"running":     running,
			"oper_status": mapNxosOperStatus(r.Status),
		})
	}
	return out
}

func mapNxosOperStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "connected":
		return "up"
	case "notconnected", "notconnec", "disabled", "err-disabled", "sfpabsent", "xcvrabsent":
		return "down"
	default:
		low := strings.ToLower(status)
		if strings.Contains(low, "connect") && !strings.HasPrefix(low, "not") {
			return "up"
		}
		return "down"
	}
}

func parseNxosInterfaceStatusRows(output string) []nxosInterfaceStatusRow {
	var out []nxosInterfaceStatusRow
	headerSeen := false
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "---") {
			continue
		}
		low := strings.ToLower(trim)
		if strings.HasPrefix(low, "port") && strings.Contains(low, "status") {
			headerSeen = true
			continue
		}
		if !headerSeen && !looksLikeNxosPortLine(trim) {
			continue
		}
		if row, ok := parseNxosStatusLine(trim); ok {
			out = append(out, row)
		}
	}
	return out
}

func looksLikeNxosPortLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return false
	}
	p := strings.ToLower(fields[0])
	return strings.HasPrefix(p, "eth") || strings.HasPrefix(p, "po") ||
		strings.HasPrefix(p, "mgmt") || strings.HasPrefix(p, "port-channel")
}

func parseNxosStatusLine(line string) (nxosInterfaceStatusRow, bool) {
	if len(line) >= 60 {
		port := strings.TrimSpace(clipStr(line, 0, 14))
		name := strings.TrimSpace(clipStr(line, 14, 33))
		status := strings.TrimSpace(clipStr(line, 33, 43))
		vlan := strings.TrimSpace(clipStr(line, 43, 53))
		duplex := strings.TrimSpace(clipStr(line, 53, 61))
		speed := strings.TrimSpace(clipStr(line, 61, 69))
		typ := strings.TrimSpace(clipStr(line, 69, len(line)))
		if port != "" && status != "" && looksLikeNxosStatusToken(status) {
			if name == "--" {
				name = ""
			}
			if typ == "--" {
				typ = ""
			}
			return nxosInterfaceStatusRow{
				Name: NormalizeNxosIfName(port), Descr: name, Status: status,
				Vlan: vlan, Duplex: duplex, Speed: speed, Type: typ,
			}, true
		}
	}
	fields := strings.Fields(line)
	if len(fields) < 6 || !looksLikeNxosPortLine(line) {
		return nxosInterfaceStatusRow{}, false
	}
	statusIdx := -1
	for i := 1; i < len(fields); i++ {
		if looksLikeNxosStatusToken(fields[i]) {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 || statusIdx+3 >= len(fields) {
		return nxosInterfaceStatusRow{}, false
	}
	descr := ""
	if statusIdx > 1 {
		descr = strings.Join(fields[1:statusIdx], " ")
		if descr == "--" {
			descr = ""
		}
	}
	typ := ""
	if statusIdx+4 < len(fields) {
		typ = strings.Join(fields[statusIdx+4:], " ")
		if typ == "--" {
			typ = ""
		}
	}
	return nxosInterfaceStatusRow{
		Name: NormalizeNxosIfName(fields[0]), Descr: descr, Status: fields[statusIdx],
		Vlan: fields[statusIdx+1], Duplex: fields[statusIdx+2], Speed: fields[statusIdx+3], Type: typ,
	}, true
}

func looksLikeNxosStatusToken(s string) bool {
	low := strings.ToLower(s)
	switch low {
	case "connected", "notconnec", "notconnected", "sfpabsent", "disabled", "err-disabled", "down", "up", "xcvrabsent":
		return true
	default:
		return false
	}
}

func clipStr(s string, start, end int) string {
	if start >= len(s) {
		return ""
	}
	if end > len(s) {
		end = len(s)
	}
	if start < 0 {
		start = 0
	}
	return s[start:end]
}

// NormalizeNxosIfName Eth1/23 → Ethernet1/23; Po1 → port-channel1.
func NormalizeNxosIfName(name string) string {
	name = strings.TrimSpace(name)
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "ethernet"):
		return "Ethernet" + name[len("Ethernet"):]
	case strings.HasPrefix(low, "eth"):
		rest := name[3:]
		if strings.HasPrefix(rest, "/") || (len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9') {
			return "Ethernet" + rest
		}
	case strings.HasPrefix(low, "port-channel"):
		return "port-channel" + name[len("port-channel"):]
	case strings.HasPrefix(low, "po") && len(name) > 2:
		rest := name[2:]
		if rest[0] >= '0' && rest[0] <= '9' {
			return "port-channel" + rest
		}
	}
	return name
}

// NxosIfNameAliases nomes alternativos para cruzar com IF-MIB / UI.
func NxosIfNameAliases(name string) []string {
	n := NormalizeNxosIfName(name)
	out := []string{n, name}
	low := strings.ToLower(n)
	if strings.HasPrefix(low, "ethernet") {
		out = append(out, "Eth"+n[len("Ethernet"):])
	}
	if strings.HasPrefix(low, "port-channel") {
		out = append(out, "Po"+n[len("port-channel"):])
	}
	seen := map[string]struct{}{}
	var uniq []string
	for _, a := range out {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		k := strings.ToLower(a)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, a)
	}
	return uniq
}

// ParseNxosTransceiverDetails interpreta show interface transceiver details.
func ParseNxosTransceiverDetails(output string) []map[string]any {
	blocks := splitNxosTransceiverBlocks(output)
	out := make([]map[string]any, 0, len(blocks))
	for iface, body := range blocks {
		if row := parseNxosTransceiverBlock(iface, body); row != nil {
			out = append(out, row)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			a, _ := out[i]["interface"].(string)
			b, _ := out[j]["interface"].(string)
			if strings.ToLower(a) > strings.ToLower(b) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func splitNxosTransceiverBlocks(output string) map[string]string {
	out := map[string]string{}
	var curIface string
	var buf []string
	flush := func() {
		if curIface == "" {
			return
		}
		out[curIface] = strings.Join(buf, "\n")
		buf = nil
	}
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if fields := strings.Fields(trim); len(fields) == 1 && reNxosIfHeaderInline.MatchString(fields[0]) {
			flush()
			curIface = NormalizeNxosIfName(fields[0])
			buf = nil
			continue
		}
		if curIface != "" {
			buf = append(buf, line)
		}
	}
	flush()
	return out
}

func parseNxosTransceiverBlock(iface, body string) map[string]any {
	lowBody := strings.ToLower(body)
	if strings.Contains(lowBody, "transceiver is not present") {
		return nil
	}
	row := map[string]any{"interface": iface}
	present := strings.Contains(lowBody, "transceiver is present")
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		low := strings.ToLower(trim)
		switch {
		case strings.HasPrefix(low, "type is "):
			row["sfp-type"] = strings.TrimSpace(trim[len("type is "):])
		case strings.HasPrefix(low, "name is "):
			row["sfp-vendor-name"] = strings.TrimSpace(trim[len("name is "):])
		case strings.HasPrefix(low, "part number is "):
			row["sfp-vendor-part-number"] = strings.TrimSpace(trim[len("part number is "):])
		case strings.HasPrefix(low, "serial number is "):
			row["sfp-serial"] = strings.TrimSpace(trim[len("serial number is "):])
		case strings.HasPrefix(low, "revision is "):
			row["sfp-vendor-revision"] = strings.TrimSpace(trim[len("revision is "):])
		}
		if m := reNxosDiagValue.FindStringSubmatch(trim); m != nil {
			val := m[2]
			switch strings.ToLower(m[1]) {
			case "temperature":
				row["sfp-temperature"] = val
			case "voltage":
				row["sfp-supply-voltage"] = val
			case "current":
				row["sfp-tx-bias-current"] = val
			case "tx power":
				row["sfp-tx-power"] = val
			case "rx power":
				row["sfp-rx-power"] = val
			}
		}
	}
	if !present && row["sfp-tx-power"] == nil && row["sfp-rx-power"] == nil {
		return nil
	}
	return row
}

// ExtractNxosTransceiverField filtra o campo pedido (arrays por interface).
func ExtractNxosTransceiverField(output, field string) any {
	rows := ParseNxosTransceiverDetails(output)
	if field == "" || field == "all" {
		return rows
	}
	if field == "sfp-vendor" {
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			item := map[string]any{"interface": r["interface"]}
			for _, k := range []string{"sfp-vendor-name", "sfp-vendor-part-number", "sfp-serial", "sfp-vendor-revision", "sfp-type"} {
				if v, ok := r[k]; ok {
					item[k] = v
				}
			}
			out = append(out, item)
		}
		return out
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"interface": r["interface"], field: r[field]})
	}
	return out
}

// DiscoverInterfacesFromNxosStatus discovery para scopes per-interface no NX-OS.
func DiscoverInterfacesFromNxosStatus(output string) []interfaceDiscovery {
	rows := parseNxosInterfaceStatusRows(output)
	out := make([]interfaceDiscovery, 0, len(rows))
	for _, r := range rows {
		running := strings.EqualFold(r.Status, "connected")
		isEther := strings.HasPrefix(strings.ToLower(r.Name), "ethernet")
		hasModule := r.Type != "" && !strings.EqualFold(r.Status, "sfpAbsent")
		out = append(out, interfaceDiscovery{
			Name: r.Name, Type: r.Type, Running: running, IsEthernet: isEther, IsSFP: hasModule || isEther,
		})
	}
	return out
}
