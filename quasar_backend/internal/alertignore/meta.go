package alertignore

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MetaKeyFromAlert extrai chave estável para ignorar (meta.key, PON, interface, métrica).
func MetaKeyFromAlert(alertType string, meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if k := strings.TrimSpace(fmt.Sprint(meta["key"])); k != "" && k != "<nil>" {
		return k
	}
	switch strings.TrimSpace(alertType) {
	case "olt_onu_drop", "olt_onu_rise", "olt_onu_rx", "olt_onu_tx":
		if p := strings.TrimSpace(fmt.Sprint(meta["pon"])); p != "" && p != "<nil>" {
			if mid := strings.TrimSpace(fmt.Sprint(meta["metric_id"])); mid != "" && mid != "<nil>" {
				return mid + ":" + p
			}
			return p
		}
	case "telemetry_threshold":
		if m := strings.TrimSpace(fmt.Sprint(meta["metric_id"])); m != "" && m != "<nil>" {
			return "telemetry:" + m
		}
	case "interface_down_transition", "interface_down":
		if ix := meta["if_index"]; ix != nil {
			return "if:" + strings.TrimSpace(fmt.Sprint(ix))
		}
		if n := strings.TrimSpace(fmt.Sprint(meta["if_name"])); n != "" && n != "<nil>" {
			return "if:" + n
		}
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx":
		if n := strings.TrimSpace(fmt.Sprint(meta["if_name"])); n != "" && n != "<nil>" {
			return n
		}
	}
	return ""
}

// MetaKeyFromJSON igual a MetaKeyFromAlert para meta em bytes.
func MetaKeyFromJSON(alertType string, raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return MetaKeyFromAlert(alertType, m)
}
