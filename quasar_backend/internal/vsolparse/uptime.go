package vsolparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// OIDVsolUptime tempo ligado em texto ("92 Days 20 Hours 28 Minutes 53 Seconds").
const OIDVsolUptime = "1.3.6.1.4.1.37950.1.1.5.10.12.5.8.0"

var vsolUptimePartRE = regexp.MustCompile(`(?i)(\d+)\s*(day|days|hour|hours|minute|minutes|second|seconds|dia|dias|hora|horas|minuto|minutos|segundo|segundos)`)

// OnuOnlineFromSta 4.1.8: 1 = online; 0, 2 ou outro = offline.
func OnuOnlineFromSta(st int) bool {
	return st == 1
}

// FormatUptimeDisplay sysUpTime (ticks) ou texto VSOL.
func FormatUptimeDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"`)
	if raw == "" {
		return ""
	}
	if ticks, err := strconv.ParseUint(raw, 10, 64); err == nil {
		return formatSysUpTimeTicks(ticks)
	}
	if looksLikeVsolUptimeText(raw) {
		return raw
	}
	return ""
}

func looksLikeVsolUptimeText(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "day") || strings.Contains(low, "hour") ||
		strings.Contains(low, "minute") || strings.Contains(low, "second") ||
		strings.Contains(low, "dia") || strings.Contains(low, "hora")
}

func formatSysUpTimeTicks(ticks uint64) string {
	sec := ticks / 100
	const day = uint64(86400)
	const hour = uint64(3600)
	const min = uint64(60)
	d := sec / day
	sec %= day
	h := sec / hour
	sec %= hour
	m := sec / min
	s := sec % min
	if d > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

// UptimeMinutesFromValue converte ticks sysUpTime ou texto VSOL em minutos.
func UptimeMinutesFromValue(raw string) (minutes float64, ok bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"`)
	if raw == "" {
		return 0, false
	}
	if ticks, err := strconv.ParseUint(raw, 10, 64); err == nil {
		return float64(ticks) / 100.0 / 60.0, true
	}
	sec, ok := vsolUptimeTextToSeconds(raw)
	if !ok || sec <= 0 {
		return 0, false
	}
	return sec / 60.0, true
}

func vsolUptimeTextToSeconds(s string) (float64, bool) {
	matches := vsolUptimePartRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0, false
	}
	var total float64
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		n, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(m[2]) {
		case "day", "days", "dia", "dias":
			total += n * 86400
		case "hour", "hours", "hora", "horas":
			total += n * 3600
		case "minute", "minutes", "minuto", "minutos":
			total += n * 60
		case "second", "seconds", "segundo", "segundos":
			total += n
		}
	}
	return total, total > 0
}

// IsVsolUptimeOID indica OID de uptime texto VSOL.
func IsVsolUptimeOID(oid string) bool {
	oid = strings.Trim(strings.TrimSpace(oid), ".")
	return oid == OIDVsolUptime || strings.HasSuffix(oid, "."+OIDVsolUptime)
}
