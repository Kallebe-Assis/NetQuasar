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
	if b, ok := summary["onu_metrics_incomplete"].(bool); ok && b {
		return true
	}
	boolFlags := []string{
		"onu_walk_truncated",
		"vsol_walk_truncated",
		"vsol_online_incomplete",
		"vsol_get_partial",
		"if_mib_refresh_truncated",
		"olt_refresh_timeout",
	}
	for _, k := range boolFlags {
		if b, ok := summary[k].(bool); ok && b {
			return true
		}
	}
	if note := strings.ToLower(strings.TrimSpace(fmt.Sprint(summary["onu_walk_note"]))); strings.Contains(note, "deadline") {
		return true
	}
	if note := strings.ToLower(strings.TrimSpace(fmt.Sprint(summary["onu_metrics_note"]))); strings.Contains(note, "deadline") {
		return true
	}

	walks := walkLogEntriesFromSummary(summary)
	if len(walks) == 0 {
		return false
	}

	statusRows, serialRows, rxRows := 0, 0, 0
	statusTrunc, serialTrunc := false, false
	for _, entry := range walks {
		metric := strings.TrimSpace(fmt.Sprint(entry["metric"]))
		matched := intFromWalkEntry(entry["matched_rows"])
		trunc, _ := entry["truncated"].(bool)
		note := strings.ToLower(strings.TrimSpace(fmt.Sprint(entry["note"])))
		deadlineNote := strings.Contains(note, "deadline") || strings.Contains(note, "timeout")

		if trunc && (metric == MetricStatus || metric == MetricSerial) {
			return true
		}
		if deadlineNote && (metric == MetricStatus || metric == MetricSerial) {
			return true
		}
		switch metric {
		case MetricStatus:
			statusRows = matched
			statusTrunc = statusTrunc || trunc || deadlineNote
		case MetricSerial:
			serialRows = matched
			serialTrunc = serialTrunc || trunc || deadlineNote
		case MetricRxPower:
			if matched > rxRows {
				rxRows = matched
			}
		}
	}
	if statusTrunc || serialTrunc {
		return true
	}
	if serialRows > 20 && statusRows > 0 && statusRows < serialRows*95/100 {
		return true
	}
	// Status parcial vs RX completo (ex.: status truncado a 200 linhas, RX com 378 ONUs).
	if rxRows > 50 && statusRows > 0 && statusRows < rxRows*90/100 {
		return true
	}
	return false
}

func walkLogEntriesFromSummary(summary map[string]any) []map[string]any {
	raw, ok := summary["onu_metrics_walks"]
	if !ok || raw == nil {
		return nil
	}
	switch arr := raw.(type) {
	case []map[string]any:
		return arr
	case []any:
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
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
