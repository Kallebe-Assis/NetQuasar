package mikrotikcollect

import (
	"regexp"
	"strconv"
	"strings"
)

var rosKV = regexp.MustCompile(`([a-zA-Z0-9_-]+)=("([^"]*)"|([^\s]+))`)

func parseRouterOSKeyValues(line string) map[string]string {
	out := map[string]string{}
	for _, m := range rosKV.FindAllStringSubmatch(line, -1) {
		if len(m) < 3 {
			continue
		}
		val := m[2]
		if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
			val = strings.Trim(val, `"`)
		}
		out[strings.ToLower(m[1])] = val
	}
	return out
}

// parseRouterOSLine interpreta uma linha RouterOS em formato key=value (print tabular)
// ou key: value (monitor, resource print, health).
func parseRouterOSLine(line string) map[string]string {
	out := parseRouterOSKeyValues(line)
	if len(out) > 0 {
		return out
	}
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return out
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	if key == "" || val == "" {
		return out
	}
	out[strings.ToLower(key)] = val
	return out
}

func parseRouterOSRecords(output string) []map[string]string {
	lines := strings.Split(output, "\n")
	var records []map[string]string
	var cur map[string]string
	flush := func() {
		if len(cur) > 0 {
			records = append(records, cur)
		}
		cur = nil
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Flags:") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			flush()
			cur = parseRouterOSLine(strings.TrimPrefix(line, "#"))
			continue
		}
		if strings.Contains(line, "name=") || strings.Contains(line, "NAME=") {
			flush()
			cur = parseRouterOSLine(line)
			continue
		}
		if cur == nil {
			cur = map[string]string{}
		}
		for k, v := range parseRouterOSLine(line) {
			cur[k] = v
		}
	}
	flush()
	return records
}

func parseSystemIdentity(output string) map[string]any {
	recs := parseRouterOSRecords(output)
	if len(recs) == 0 {
		return map[string]any{"raw": strings.TrimSpace(output)}
	}
	if n, ok := recs[0]["name"]; ok {
		return map[string]any{"name": n}
	}
	return map[string]any{"raw": strings.TrimSpace(output)}
}

func parseSystemResource(output string) map[string]any {
	recs := parseRouterOSRecords(output)
	if len(recs) == 0 {
		return map[string]any{"raw": strings.TrimSpace(output)}
	}
	r := recs[0]
	out := map[string]any{}
	for _, k := range []string{
		"uptime", "version", "cpu-load", "cpu-frequency", "cpu", "cpu-count",
		"free-memory", "total-memory", "free-hdd-space", "total-hdd-space",
		"board-name", "platform", "architecture-name",
	} {
		if v, ok := r[k]; ok && v != "" {
			out[k] = v
		}
	}
	return out
}

func parseSystemResourceField(output, field string) any {
	full := parseSystemResource(output)
	if field == "" {
		return full
	}
	if v, ok := full[field]; ok {
		return map[string]any{field: v}
	}
	// RouterOS antigo usa "cpu" em vez de cpu-load
	if field == "cpu-load" {
		if v, ok := full["cpu"]; ok {
			return map[string]any{"cpu-load": v}
		}
	}
	return map[string]any{field: nil}
}

func healthSensorMap(output string) map[string]string {
	recs := parseRouterOSRecords(output)
	out := map[string]string{}
	for _, r := range recs {
		name := strings.ToLower(strings.TrimSpace(r["name"]))
		val := strings.TrimSpace(r["value"])
		if name != "" && val != "" {
			out[name] = val
			continue
		}
		for k, v := range r {
			if k == "name" || k == "value" || k == "type" || k == "status" {
				continue
			}
			if strings.Contains(k, "temp") || strings.Contains(k, "voltage") {
				out[k] = v
			}
		}
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, "temperature") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				out["temperature"] = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(lower, "voltage") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				out["voltage"] = strings.TrimSpace(parts[1])
			}
		}
	}
	return out
}

func parseSystemHealth(output string) map[string]any {
	sensors := healthSensorMap(output)
	items := make([]map[string]string, 0)
	for k, v := range sensors {
		items = append(items, map[string]string{"name": k, "value": v})
	}
	return map[string]any{"sensors": items}
}

func parseSystemHealthField(output, field string) any {
	sensors := healthSensorMap(output)
	if field == "" {
		return parseSystemHealth(output)
	}
	aliases := map[string][]string{
		"temperature": {"temperature", "cpu-temperature", "board-temperature", "sfp-temperature"},
		"voltage":     {"voltage", "board-voltage", "psu-voltage", "input-voltage"},
	}
	for _, key := range aliases[field] {
		if v, ok := sensors[key]; ok {
			return map[string]any{field: v}
		}
	}
	for k, v := range sensors {
		if strings.Contains(k, field) || (field == "temperature" && strings.Contains(k, "temp")) {
			return map[string]any{field: v}
		}
	}
	return map[string]any{field: nil}
}

func parseInterfaceList(output string) []map[string]any {
	recs := parseRouterOSRecords(output)
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := map[string]any{}
		for _, k := range []string{"name", "type", "mtu", "running", "disabled", "slave", "comment"} {
			if v, ok := r[k]; ok && v != "" {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func parseInterfaceRecords(output string) []map[string]string {
	return parseRouterOSRecords(output)
}

func parseInterfaceStats(output string) []map[string]any {
	recs := parseRouterOSRecords(output)
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := map[string]any{}
		if n, ok := r["name"]; ok {
			row["name"] = n
		}
		for _, k := range []string{
			"rx-byte", "tx-byte", "rx-packet", "tx-packet",
			"rx-error", "tx-error", "rx-drop", "tx-drop", "link-downs",
		} {
			if v, ok := r[k]; ok && v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					row[k] = n
				} else {
					row[k] = v
				}
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func parseInterfaceTraffic(output string, iface string) map[string]any {
	recs := parseRouterOSRecords(output)
	row := map[string]any{"interface": iface}
	if len(recs) > 0 {
		r := recs[0]
		for _, k := range []string{"rx-bits-per-second", "tx-bits-per-second", "rx-packets-per-second", "tx-packets-per-second"} {
			if v, ok := r[k]; ok && v != "" {
				row[k] = v
			}
		}
	}
	if len(row) <= 1 {
		for k, v := range parseRouterOSLine(output) {
			row[k] = v
		}
	}
	return row
}

func parseEthernetMonitor(output string, iface string) map[string]any {
	recs := parseRouterOSRecords(output)
	row := map[string]any{"interface": iface}
	if len(recs) > 0 {
		for k, v := range recs[0] {
			if v != "" {
				row[k] = v
			}
		}
	} else {
		for k, v := range parseRouterOSLine(output) {
			row[k] = v
		}
	}
	return row
}

func parseEthernetMonitorField(output, iface, field string) any {
	full := parseEthernetMonitor(output, iface)
	if field == "" {
		return full
	}
	if field == "status" {
		out := map[string]any{"interface": iface}
		for _, k := range []string{"status", "rate", "full-duplex"} {
			if v, ok := full[k]; ok {
				out[k] = v
			}
		}
		return out
	}
	if field == "sfp-vendor" {
		out := map[string]any{"interface": iface}
		for _, k := range []string{"sfp-vendor-name", "sfp-vendor-part-number", "sfp-serial", "sfp-vendor-revision"} {
			if v, ok := full[k]; ok {
				out[k] = v
			}
		}
		return out
	}
	if v, ok := full[field]; ok {
		return map[string]any{"interface": iface, field: v}
	}
	return map[string]any{"interface": iface, field: nil}
}

func parseWirelessRecords(output string) []map[string]any {
	recs := parseRouterOSRecords(output)
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		row := map[string]any{}
		for _, k := range []string{"name", "ssid", "channel", "frequency", "band", "protocol", "mode", "running", "disabled", "mac-address"} {
			if v, ok := r[k]; ok && v != "" {
				row[k] = v
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func splitParser(parser string) (base, field string) {
	parts := strings.SplitN(strings.TrimSpace(parser), ":", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func parseTelnetOutput(parser, output string) any {
	return parseTelnetOutputForIface(parser, output, "")
}

func parseTelnetOutputForIface(parser, output, iface string) any {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	base, field := splitParser(parser)
	switch base {
	case "system_identity":
		return parseSystemIdentity(output)
	case "system_resource":
		return parseSystemResourceField(output, field)
	case "system_health":
		return parseSystemHealthField(output, field)
	case "interface_list":
		return parseInterfaceList(output)
	case "interface_detail", "interface_ethernet":
		return parseInterfaceRecords(output)
	case "interface_stats":
		return parseInterfaceStats(output)
	case "interface_traffic":
		return parseInterfaceTraffic(output, iface)
	case "ethernet_monitor":
		return parseEthernetMonitorField(output, iface, field)
	case "sfp_monitor":
		return parseInterfaceRecords(output)
	case "wireless_detail", "wifiwave2_detail":
		return parseWirelessRecords(output)
	case "nxos_system_uptime":
		return ParseNxosSystemUptime(output)
	case "nxos_hostname":
		return ParseNxosHostname(output)
	case "nxos_interface_status":
		return ParseNxosInterfaceStatus(output)
	case "nxos_transceiver":
		return ExtractNxosTransceiverField(output, field)
	default:
		return map[string]any{"raw": output}
	}
}

// DiscoverInterfacesFromPrint extrai nomes de interfaces do output de /interface print.
func DiscoverInterfacesFromPrint(output string) []interfaceDiscovery {
	recs := parseRouterOSRecords(output)
	out := make([]interfaceDiscovery, 0, len(recs))
	for _, r := range recs {
		name := strings.TrimSpace(r["name"])
		if name == "" {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(r["type"]))
		running := strings.EqualFold(r["running"], "true") || r["running"] == "yes" || r["running"] == "R"
		disabled := strings.EqualFold(r["disabled"], "true") || r["disabled"] == "yes" || r["disabled"] == "X"
		slave := strings.EqualFold(r["slave"], "true") || r["slave"] == "yes"
		if disabled || slave {
			continue
		}
		isEther := typ == "ether" || strings.HasPrefix(typ, "ether")
		isSFP := strings.Contains(strings.ToLower(name), "sfp")
		out = append(out, interfaceDiscovery{
			Name: name, Type: typ, Running: running, IsEthernet: isEther, IsSFP: isSFP,
		})
	}
	return out
}

type interfaceDiscovery struct {
	Name       string
	Type       string
	Running    bool
	IsEthernet bool
	IsSFP      bool
}
