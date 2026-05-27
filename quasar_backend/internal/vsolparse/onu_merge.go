package vsolparse

import (
	"fmt"
	"strings"
)

// MergeOnuRowsTelemetry mantém RX/modelo/etc. do snapshot anterior e actualiza estado online.
func MergeOnuRowsTelemetry(prev, fresh []map[string]any) []map[string]any {
	if len(fresh) == 0 {
		return prev
	}
	if len(prev) == 0 {
		return fresh
	}
	byKey := make(map[string]map[string]any, len(prev))
	for _, r := range prev {
		if k := onuRowKey(r); k != "" {
			byKey[k] = r
		}
	}
	out := make([]map[string]any, 0, len(fresh))
	for _, r := range fresh {
		k := onuRowKey(r)
		if k == "" {
			out = append(out, r)
			continue
		}
		old, ok := byKey[k]
		if !ok {
			out = append(out, r)
			continue
		}
		out = append(out, mergeOnuRow(old, r))
	}
	return out
}

func onuRowKey(r map[string]any) string {
	pon, onu := intVal(r["pon"]), intVal(r["onu"])
	if pon < 1 || onu < 1 {
		return ""
	}
	return fmt.Sprintf("%d.%d", pon, onu)
}

func mergeOnuRow(old, neu map[string]any) map[string]any {
	out := make(map[string]any, len(old)+4)
	for k, v := range old {
		out[k] = v
	}
	for k, v := range neu {
		switch k {
		case "online", "onu_online_sta":
			if sta, ok := neu["onu_online_sta"]; ok && intVal(sta) != fieldUnset {
				out["online"] = neu["online"]
				out["onu_online_sta"] = sta
			} else {
				out["online"] = false
				out["onu_online_sta"] = fieldUnset
			}
		case "phase_sta", "admin_sta", "omcc_sta":
			out[k] = v
		case "rx_pwr", "tx_pwr", "voltage", "temp", "bias", "model", "serial", "vendor", "version", "profile_name", "auth_mode":
			if strVal(v) != "" {
				out[k] = v
			}
		default:
			if strVal(v) != "" {
				out[k] = v
			}
		}
	}
	return out
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
