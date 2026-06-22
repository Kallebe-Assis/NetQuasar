package oltcollect

import (
	"fmt"
	"strings"
)

// IsOltSnapshotIncomplete indica se a coleta SNMP não é fiável para comparar contagens de ONU (alertas).
func IsOltSnapshotIncomplete(summary map[string]any) bool {
	if summary == nil {
		return false
	}
	boolFlags := []string{
		"onu_walk_truncated",
		"vsol_walk_truncated",
		"vsol_online_incomplete",
		"vsol_get_partial",
		"if_mib_refresh_truncated",
		"olt_refresh_timeout",
		"onu_metrics_incomplete",
	}
	for _, k := range boolFlags {
		if b, ok := summary[k].(bool); ok && b {
			return true
		}
	}
	if note := strings.ToLower(strings.TrimSpace(fmt.Sprint(summary["onu_walk_note"]))); strings.Contains(note, "deadline") {
		return true
	}
	walks, ok := summary["onu_metrics_walks"].([]any)
	if !ok {
		return false
	}
	statusRows, serialRows := 0, 0
	statusTrunc := false
	for _, raw := range walks {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		metric := strings.TrimSpace(fmt.Sprint(entry["metric"]))
		matched := intFromWalkEntry(entry["matched_rows"])
		trunc, _ := entry["truncated"].(bool)
		if trunc && (metric == MetricStatus || metric == MetricSerial) {
			return true
		}
		switch metric {
		case MetricStatus:
			statusRows = matched
			statusTrunc = statusTrunc || trunc
		case MetricSerial:
			serialRows = matched
		}
	}
	if statusTrunc {
		return true
	}
	if serialRows > 20 && statusRows > 0 && statusRows < serialRows*95/100 {
		return true
	}
	return false
}

func intFromWalkEntry(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
