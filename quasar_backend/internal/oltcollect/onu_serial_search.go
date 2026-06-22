package oltcollect

import (
	"regexp"
	"strconv"
	"strings"
)

var vsolGponIndexRE = regexp.MustCompile(`(?i)^GPON\d+/(\d+):(\d+)$`)

// SerialSearchOnuEntry ONU parseada de uma linha de listagem telnet.
type SerialSearchOnuEntry struct {
	Pon     int
	Onu     int
	Serial  string
	Model   string
	Profile string
	Mode    string
	GponOnu string
}

// SerialSearchUsesSerialPlaceholder indica lookup directo por serial na OLT.
func (c OnuReportConfig) SerialSearchUsesSerialPlaceholder() bool {
	return strings.Contains(c.DefaultSerialSearchCommand(), "{serial}")
}

// SerialSearchUsesPonPlaceholder indica comando por porta PON.
func (c OnuReportConfig) SerialSearchUsesPonPlaceholder() bool {
	return strings.Contains(c.DefaultSerialSearchCommand(), "{pon}")
}

// ParsePonOnuFromVsolGponIndex extrai PON e ONU de GPON0/1:27.
func ParsePonOnuFromVsolGponIndex(gponIdx string) (pon, onu int) {
	m := vsolGponIndexRE.FindStringSubmatch(strings.TrimSpace(gponIdx))
	if len(m) < 3 {
		if g := ParseGponOnuFromOutput(gponIdx); g != "" {
			return ParsePonOnuFromGponOnu(g)
		}
		return 0, 0
	}
	pon, _ = strconv.Atoi(m[1])
	onu, _ = strconv.Atoi(m[2])
	return pon, onu
}

// ParseOnuListFromTelnetOutput interpreta linhas de listagem (ex.: show onu info).
func ParseOnuListFromTelnetOutput(output string) []SerialSearchOnuEntry {
	text := cleanTelnetCLIOutput(output)
	var out []SerialSearchOnuEntry
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "---") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(t), "onuindex") || strings.HasPrefix(strings.ToLower(t), "authpass time") {
			continue
		}
		entry, ok := parseOnuListLine(t)
		if !ok {
			continue
		}
		key := onuListEntryKey(entry)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, entry)
	}
	return out
}

func onuListEntryKey(e SerialSearchOnuEntry) string {
	if e.Pon > 0 && e.Onu > 0 {
		return strconv.Itoa(e.Pon) + ":" + strconv.Itoa(e.Onu)
	}
	return strings.ToLower(strings.TrimSpace(e.Serial))
}

func parseOnuListLine(line string) (SerialSearchOnuEntry, bool) {
	m := telnetVsolInfoRE.FindStringSubmatch(line)
	if m == nil {
		return SerialSearchOnuEntry{}, false
	}
	gponIdx := strings.TrimSpace(m[1])
	pon, onu := ParsePonOnuFromVsolGponIndex(gponIdx)
	mode := strings.TrimSpace(m[4])
	serial := strings.TrimSpace(m[5])
	if strings.EqualFold(mode, "sn") || strings.EqualFold(mode, "password") || strings.EqualFold(mode, "pwd") {
		// serial already in m[5]
	} else if looksLikeSerial(m[4]) && !looksLikeSerial(m[5]) {
		serial = strings.TrimSpace(m[4])
		mode = strings.TrimSpace(m[5])
	}
	entry := SerialSearchOnuEntry{
		GponOnu: gponIdx,
		Pon:     pon,
		Onu:     onu,
		Model:   strings.TrimSpace(m[2]),
		Profile: strings.TrimSpace(m[3]),
		Mode:    mode,
		Serial:  serial,
	}
	if entry.Serial == "" {
		return SerialSearchOnuEntry{}, false
	}
	return entry, true
}

func looksLikeSerial(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// FilterSerialSearchEntries filtra por serial (contém, case-insensitive) e opcionalmente PON.
func FilterSerialSearchEntries(entries []SerialSearchOnuEntry, serial string, pon int) []SerialSearchOnuEntry {
	serialQ := strings.ToLower(strings.TrimSpace(serial))
	if serialQ == "" {
		return nil
	}
	var out []SerialSearchOnuEntry
	for _, e := range entries {
		if pon > 0 && e.Pon != pon {
			continue
		}
		if strings.Contains(strings.ToLower(e.Serial), serialQ) {
			out = append(out, e)
		}
	}
	return out
}

// SerialSearchEntryToMap converte entrada para resposta JSON.
func SerialSearchEntryToMap(e SerialSearchOnuEntry) map[string]any {
	m := map[string]any{
		"serial": e.Serial,
		"model":  e.Model,
	}
	if e.Pon > 0 {
		m["pon"] = e.Pon
	}
	if e.Onu > 0 {
		m["onu"] = e.Onu
	}
	if e.Profile != "" {
		m["profile"] = e.Profile
	}
	if e.GponOnu != "" {
		m["gpon_onu"] = e.GponOnu
	}
	if e.Mode != "" {
		m["mode"] = e.Mode
	}
	return m
}
