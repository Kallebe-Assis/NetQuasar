package oltifderive

import (
	"fmt"
	"strings"
)

// ApplyPonOperStatusFromOnuCounts define o status operacional da PON a partir de onu_online:
// ON (up) se ≥1 ONU online; OFF (down) se nenhuma ONU online.
// Preserva link_oper_status (ifOperStatus / status SNMP) antes de sobrescrever status da UI.
func ApplyPonOperStatusFromOnuCounts(row map[string]any) bool {
	preserveLinkOperStatus(row)
	on, ok := OnuOnlineFromRow(row)
	if !ok {
		return false
	}
	st := "down"
	if on >= 1 {
		st = "up"
	}
	row["pon_oper_status"] = st
	row["if_oper_status"] = st
	row["status"] = st
	row["pon_status_from_onu_counts"] = true
	return true
}

// ApplyPonOperStatusAll aplica ApplyPonOperStatusFromOnuCounts em cada linha PON.
func ApplyPonOperStatusAll(pons []map[string]any) {
	for _, p := range pons {
		ApplyPonOperStatusFromOnuCounts(p)
	}
}

// PreserveLinkOperStatusAll grava link_oper_status a partir do status SNMP actual.
func PreserveLinkOperStatusAll(pons []map[string]any) {
	for _, p := range pons {
		preserveLinkOperStatus(p)
	}
}

func preserveLinkOperStatus(row map[string]any) {
	if row == nil {
		return
	}
	if _, has := row["link_oper_status"]; has {
		return
	}
	for _, key := range []string{"status", "if_oper_status", "pon_oper_status", "oper_status"} {
		raw := strings.TrimSpace(strings.ToLower(anyToTrimmedString(row[key])))
		if raw == "" {
			continue
		}
		if up, ok := parsePonOperToken(raw); ok {
			if up {
				row["link_oper_status"] = "up"
			} else {
				row["link_oper_status"] = "down"
			}
			return
		}
	}
}

func anyToTrimmedString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == 1 {
			return "1"
		}
		if x == 2 {
			return "2"
		}
	case int:
		if x == 1 {
			return "1"
		}
		if x == 2 {
			return "2"
		}
	case int64:
		if x == 1 {
			return "1"
		}
		if x == 2 {
			return "2"
		}
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func parsePonOperToken(raw string) (up bool, ok bool) {
	switch raw {
	case "up", "pon_up", "1", "online", "on", "active":
		return true, true
	case "down", "pon_down", "2", "offline", "off", "inactive", "lowerlayerdown", "notpresent":
		return false, true
	default:
		return false, false
	}
}

// PonOperIsUp devolve se a PON está operacionalmente UP.
// Preferência: link_oper_status (SNMP) → se down, considera DOWN mesmo com ONUs;
// senão status / pon_oper_status / if_oper_status (UI / overlay ONU).
func PonOperIsUp(row map[string]any) (up bool, ok bool) {
	if row == nil {
		return false, false
	}
	if link := strings.TrimSpace(strings.ToLower(anyToTrimmedString(row["link_oper_status"]))); link != "" {
		if u, parsed := parsePonOperToken(link); parsed && !u {
			return false, true
		}
	}
	for _, key := range []string{"status", "pon_oper_status", "if_oper_status", "oper_status"} {
		raw := strings.TrimSpace(strings.ToLower(anyToTrimmedString(row[key])))
		if raw == "" {
			continue
		}
		if u, parsed := parsePonOperToken(raw); parsed {
			return u, true
		}
	}
	if link := strings.TrimSpace(strings.ToLower(anyToTrimmedString(row["link_oper_status"]))); link != "" {
		if u, parsed := parsePonOperToken(link); parsed {
			return u, true
		}
	}
	return false, false
}
