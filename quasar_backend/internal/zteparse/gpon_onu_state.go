package zteparse

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type PonOnuStateRow struct {
	Pon       string `json:"pon"`
	OnuTotal  int    `json:"onu_total"`
	OnuOnline int    `json:"onu_online"`
	OnuOffline int   `json:"onu_offline"`
}

var (
	reOnuRef = regexp.MustCompile(`(?i)(?:gpon-onu_|onu-)?(\d+)\/(\d+)\/(\d+):(\d+)`)
)

func ParseShowGponOnuState(raw string) []PonOnuStateRow {
	type agg struct {
		on  int
		off int
	}
	byPon := map[string]*agg{}
	lines := strings.Split(raw, "\n")
	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		m := reOnuRef.FindStringSubmatch(line)
		if len(m) != 5 {
			continue
		}
		pon := m[1] + "/" + m[2] + "/" + m[3]
		a := byPon[pon]
		if a == nil {
			a = &agg{}
			byPon[pon] = a
		}
		phase := detectPhaseState(line)
		low := strings.ToLower(line)
		isOffline := strings.Contains(phase, "los") ||
			strings.Contains(phase, "dying") ||
			strings.Contains(phase, "offline") ||
			strings.Contains(phase, "auth") ||
			strings.Contains(phase, "deactive") ||
			strings.Contains(low, " down ")
		isOnline := strings.Contains(phase, "working") ||
			strings.Contains(phase, "online") ||
			strings.Contains(low, " working ")
		if isOffline {
			a.off++
		} else if isOnline {
			a.on++
		} else {
			// sem estado explícito: considera offline para não inflar online.
			a.off++
		}
	}
	out := make([]PonOnuStateRow, 0, len(byPon))
	for pon, a := range byPon {
		out = append(out, PonOnuStateRow{
			Pon:       pon,
			OnuTotal:  a.on + a.off,
			OnuOnline: a.on,
			OnuOffline: a.off,
		})
	}
	sort.Slice(out, func(i, j int) bool { return lessPon(out[i].Pon, out[j].Pon) })
	return out
}

func detectPhaseState(line string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) >= 4 {
		// formato típico ZTE: OnuIndex Admin-state OMCC-state Phase-state Speed-mode
		return strings.ToLower(strings.TrimSpace(parts[3]))
	}
	low := strings.ToLower(line)
	switch {
	case strings.Contains(low, "working"):
		return "working"
	case strings.Contains(low, "dyinggasp"), strings.Contains(low, "dying"):
		return "dyinggasp"
	case strings.Contains(low, "los"):
		return "los"
	case strings.Contains(low, "offline"):
		return "offline"
	case strings.Contains(low, "online"):
		return "online"
	default:
		return ""
	}
}

func lessPon(a, b string) bool {
	pa := ponParts(a)
	pb := ponParts(b)
	n := len(pa)
	if len(pb) < n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		if pa[i] == pb[i] {
			continue
		}
		return pa[i] < pb[i]
	}
	if len(pa) != len(pb) {
		return len(pa) < len(pb)
	}
	return strings.ToLower(strings.TrimSpace(a)) < strings.ToLower(strings.TrimSpace(b))
}

func ponParts(s string) []int {
	parts := strings.Split(strings.TrimSpace(s), "/")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

