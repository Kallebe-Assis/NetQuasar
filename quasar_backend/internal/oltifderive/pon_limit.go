package oltifderive

import (
	"fmt"
	"strconv"
	"strings"
)

// PonPortNumberFromRow devolve o número da porta PON (último segmento de "01" ou "1/1/16").
func PonPortNumberFromRow(m map[string]any) int {
	if m == nil {
		return 0
	}
	if c := strings.TrimSpace(fmt.Sprint(m["pon_compact"])); c != "" {
		if n := PonPortFromCompact(c); n > 0 {
			return n
		}
	}
	if v, ok := m["pon"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 && n <= 256 {
				return n
			}
		case float64:
			if n > 0 && n <= 256 {
				return int(n)
			}
		}
	}
	id := strings.TrimSpace(fmt.Sprint(m["id"]))
	if id == "" {
		id = strings.TrimSpace(fmt.Sprint(m["name"]))
	}
	if c := PonCompactFromPhy(id, id); c != "" {
		if n := PonPortFromCompact(c); n > 0 {
			return n
		}
	}
	if n, err := strconv.Atoi(strings.TrimLeft(id, "0")); err == nil && n > 0 {
		return n
	}
	return 0
}

// FilterPonRowsByMaxSlots mantém apenas portas PON com slot 1..maxPons.
func FilterPonRowsByMaxSlots(pons []map[string]any, maxPons int) []map[string]any {
	if maxPons <= 0 || len(pons) == 0 {
		return pons
	}
	out := make([]map[string]any, 0, len(pons))
	for _, p := range pons {
		if p == nil {
			continue
		}
		slot := PonPortNumberFromRow(p)
		if slot >= 1 && slot <= maxPons {
			out = append(out, p)
		}
	}
	return out
}

// FilterPonAnyRows aplica FilterPonRowsByMaxSlots a []any de mapas.
func FilterPonAnyRows(pons []any, maxPons int) []any {
	if maxPons <= 0 || len(pons) == 0 {
		return pons
	}
	out := make([]any, 0, len(pons))
	for _, row := range pons {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		slot := PonPortNumberFromRow(m)
		if slot >= 1 && slot <= maxPons {
			out = append(out, row)
		}
	}
	return out
}
