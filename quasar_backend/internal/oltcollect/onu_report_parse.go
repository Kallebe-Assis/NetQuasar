package oltcollect

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	telnetKvLineRE     = regexp.MustCompile(`^\s{0,6}([A-Za-z0-9 /_.-]{2,48}):\s+(.+?)\s*$`)
	telnetKvLabelOnlyRE = regexp.MustCompile(`^\s{0,6}([A-Za-z0-9 /_.-]{2,48}):\s*$`)
	telnetGponPowerRE  = regexp.MustCompile(`gpon_onu[^\n]+\s+(-?\d+(?:\.\d+)?)\s*\(dbm\)`)
	telnetVsolInfoRE     = regexp.MustCompile(`^(GPON[\d/:\w-]+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)`)
	// show onu auto-find: OnuIndex  Sn  State  (ex.: GPON0/4:1  ZTEGCFAA2AB1  unknow)
	telnetVsolAutoFindRE = regexp.MustCompile(`^(?i)(GPON\d+/\d+:\d+)\s+(\S+)\s+(\S+)\s*$`)
	// show pon onu uncfg: gpon_olt-1/1/9  Model  SERIAL  PW
	telnetZtePonUncfgRE  = regexp.MustCompile(`^(?i)(gpon_olt-\d+/\d+/(\d+))\s+(\S+)\s+([A-Za-z0-9]{6,})\s+(\S+)\s*$`)
	telnetVsolStateRE    = regexp.MustCompile(`^(\d+\/\d+\/\d+:\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+?)(?:\s+ONU Number:|$)`)
	telnetOnuNumberRE  = regexp.MustCompile(`ONU Number:\s*(\S+)`)
)

var telnetLabelPT = map[string]string{
	"onuindex": "Índice", "onu interface": "Interface", "interface": "Interface",
	"name": "Nome", "type": "Modelo", "model": "Modelo",
	"onu type configured": "Modelo", "onu type reported": "Modelo reportado",
	"profile": "Profile", "mode": "Modo",
	"authinfo": "SN", "auth information": "SN", "serial number": "SN",
	"sn reported": "SN", "sn bind": "SN bind",
	"admin state": "Admin", "admin": "Admin",
	"omcc state": "OMCC", "omcc": "OMCC",
	"phase state": "Estado", "state": "Estado",
	"channel": "Canal", "onu id": "ONU ID", "onu distance": "Distância",
	"distance": "Distância", "online duration": "Tempo online",
	"hardware version": "HW", "software version": "SW",
	"rx": "RX", "tx": "TX",
	"rx optical level": "RX", "tx optical level": "TX",
	"rx power": "RX", "tx power": "TX",
	"authentication mode": "Autenticação",
	"configured speed mode": "Velocidade config.",
	"current speed mode": "Velocidade actual",
	"config state": "Config", "onu status": "Status ONU", "fec": "FEC",
	"onu number": "ONU Number", "temperature": "Temperatura", "temp": "Temperatura",
	"voltage": "Voltagem", "power feed voltage": "Voltagem",
	"laser bias current": "Bias",
	"onu pon interface": "Interface PON",
}

func normalizeTelnetLabel(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if pt, ok := telnetLabelPT[key]; ok {
		return pt
	}
	return strings.TrimSpace(raw)
}

func normalizeTelnetValue(label, value string) string {
	v := strings.TrimSpace(value)
	if m := regexp.MustCompile(`(?i)sn\(([A-Za-z0-9]+)\)`).FindStringSubmatch(v); len(m) > 1 {
		return m[1]
	}
	if label == "RX" || label == "TX" {
		v = stripUnitParen(v, "dbm")
	}
	if label == "Voltagem" {
		v = stripUnitParen(v, "v")
	}
	if label == "Temperatura" {
		v = stripUnitParen(v, "c")
	}
	if label == "Bias" {
		v = stripUnitParen(v, "ma")
	}
	return strings.TrimSpace(v)
}

// stripUnitParen normaliza " -23.280(dBm) ", "3.28(V)", "31.430(C)".
func stripUnitParen(raw, unit string) string {
	v := strings.TrimSpace(raw)
	v = strings.ReplaceAll(v, ",", ".")
	lower := strings.ToLower(v)
	unit = strings.ToLower(strings.TrimSpace(unit))
	if idx := strings.Index(lower, "("+unit); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	lower = strings.ToLower(v)
	v = strings.TrimSpace(strings.TrimSuffix(lower, unit))
	return strings.TrimSpace(v)
}

func cleanTelnetCLIOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	var kept []string
	echoSkips := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			kept = append(kept, line)
			continue
		}
		if echoSkips < 3 && (strings.HasPrefix(t, "show ") || strings.HasPrefix(t, "terminal ") || strings.HasPrefix(t, "scroll ")) {
			echoSkips++
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func extractTelnetKVFields(text string) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "---") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(t), "authpass time") {
			break
		}

		labelRaw := ""
		valueRaw := ""
		if m := telnetKvLineRE.FindStringSubmatch(line); m != nil {
			labelRaw = strings.TrimSpace(m[1])
			valueRaw = m[2]
		} else if m := telnetKvLabelOnlyRE.FindStringSubmatch(line); m != nil {
			labelRaw = strings.TrimSpace(m[1])
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				if telnetKvLineRE.MatchString(lines[j]) || telnetKvLabelOnlyRE.MatchString(lines[j]) {
					break
				}
				if strings.HasPrefix(next, "---") {
					break
				}
				valueRaw = next
				i = j
				break
			}
		} else {
			continue
		}
		if strings.TrimSpace(valueRaw) == "" {
			continue
		}
		label := normalizeTelnetLabel(labelRaw)
		value := normalizeTelnetValue(label, valueRaw)
		if value == "" {
			continue
		}
		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true
		out[label] = value
	}
	return out
}

// ExtractTelnetKVFieldsPublic expõe o parser de campos chave:valor da saída telnet.
func ExtractTelnetKVFieldsPublic(text string) map[string]string {
	return extractTelnetKVFields(cleanTelnetCLIOutput(text))
}

func extractTelnetPowerFields(text string) map[string]string {
	out := map[string]string{}
	if m := telnetGponPowerRE.FindStringSubmatch(text); len(m) > 1 {
		isRx := strings.Contains(strings.ToLower(text), "onu-rx") ||
			strings.Contains(strings.ToLower(text), "rx power")
		if !isRx {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil && f > 0 {
				isRx = false
			} else {
				isRx = true
			}
		}
		if isRx {
			out["RX"] = m[1] + " dBm"
		} else {
			out["TX"] = m[1] + " dBm"
		}
	}
	return out
}

func dataRowsAfterHeader(text string, headerRe *regexp.Regexp) string {
	lines := strings.Split(text, "\n")
	headerIdx := -1
	for i, line := range lines {
		if headerRe.MatchString(line) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return ""
	}
	var parts []string
	for i := headerIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "---") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(t), "authpass time") {
			break
		}
		if strings.HasPrefix(t, "gpon-olt") || strings.HasPrefix(t, "olt-zte") || strings.HasPrefix(t, "$") || strings.HasPrefix(t, "#") {
			break
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}

func extractVsolOnuInfoFields(text string) map[string]string {
	row := strings.TrimSpace(dataRowsAfterHeader(text, regexp.MustCompile(`(?i)Onuindex.*Model.*Profile`)))
	if row == "" {
		return nil
	}
	m := telnetVsolInfoRE.FindStringSubmatch(row)
	if m == nil {
		return nil
	}
	return map[string]string{
		"Índice": m[1], "Modelo": m[2], "Profile": m[3], "Modo": m[4], "SN": m[5],
	}
}

func extractVsolOnuStateFields(text string) map[string]string {
	row := strings.TrimSpace(dataRowsAfterHeader(text, regexp.MustCompile(`(?i)OnuIndex.*Admin State`)))
	out := map[string]string{}
	if row != "" {
		if m := telnetVsolStateRE.FindStringSubmatch(row); m != nil {
			out["Admin"] = m[2]
			out["OMCC"] = m[3]
			out["Estado"] = m[4]
			out["Canal"] = strings.TrimSpace(m[5])
		}
	}
	if m := telnetOnuNumberRE.FindStringSubmatch(text); len(m) > 1 {
		out["ONU Number"] = m[1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseTelnetReportSteps agrega campos de todos os passos telnet de uma ONU.
func ParseTelnetReportSteps(steps []struct {
	Command string
	Output  string
}) map[string]string {
	merged := map[string]string{}
	for _, step := range steps {
		cleaned := cleanTelnetCLIOutput(step.Output)
		cmd := strings.ToLower(strings.TrimSpace(step.Command))
		var fields map[string]string
		switch {
		case strings.Contains(cmd, "show onu info"):
			fields = extractVsolOnuInfoFields(cleaned)
		case strings.Contains(cmd, "show onu state"):
			fields = extractVsolOnuStateFields(cleaned)
		default:
			fields = extractTelnetKVFields(cleaned)
			for k, v := range extractTelnetPowerFields(cleaned) {
				if _, ok := fields[k]; !ok {
					fields[k] = v
				}
			}
		}
		if fields == nil {
			fields = extractTelnetKVFields(cleaned)
		}
		for k, v := range fields {
			if v == "" {
				continue
			}
			if prev, ok := merged[k]; !ok || len(v) > len(prev) {
				merged[k] = v
			}
		}
	}
	return merged
}

func mergeTelnetFieldsIntoOnuRow(row map[string]any, fields map[string]string, reportedAt string) {
	if row == nil || len(fields) == 0 {
		return
	}
	setIfEmpty := func(key, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		if cur, ok := row[key]; ok && strings.TrimSpace(stringFromAny(cur)) != "" {
			return
		}
		row[key] = val
	}
	if v := fields["SN"]; v != "" {
		setIfEmpty("serial", v)
	}
	if v := fields["Modelo"]; v != "" {
		setIfEmpty("model", v)
	}
	if v := fields["Profile"]; v != "" {
		setIfEmpty("profile_name", v)
	}
	if v := fields["Estado"]; v != "" {
		setIfEmpty("phase_sta", v)
	}
	if v := fields["RX"]; v != "" {
		row["rx_pwr"] = v
		if dbm := parseDbmValue(v); dbm != nil {
			row["rx_dbm"] = *dbm
		}
	}
	if v := fields["TX"]; v != "" {
		row["tx_pwr"] = v
		if dbm := parseDbmValue(v); dbm != nil {
			row["tx_dbm"] = *dbm
		}
	}
	if v := fields["Temperatura"]; v != "" {
		row["temp"] = v
	}
	if v := fields["Voltagem"]; v != "" {
		row["voltage"] = v
	}
	if v := fields["Canal"]; v != "" {
		setIfEmpty("channel", v)
	}
	row["telnet_report_at"] = reportedAt
	row["data_source_telnet"] = true
	if extra, ok := row["telnet_fields"].(map[string]any); ok {
		for k, v := range fields {
			extra[k] = v
		}
	} else {
		extra := map[string]any{}
		for k, v := range fields {
			extra[k] = v
		}
		row["telnet_fields"] = extra
	}
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func parseDbmValue(s string) *float64 {
	s = stripUnitParen(s, "dbm")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}
