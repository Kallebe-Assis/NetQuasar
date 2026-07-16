package oltcollect

import (
	"fmt"
	"strings"
)

// DeriveSnmpHealthFromSummary calcula saúde SNMP da OLT a partir do snapshot (coleta ONU/PON),
// independente do ciclo genérico de telemetria em device_probe_cache.
func DeriveSnmpHealthFromSummary(summary map[string]any) (status, reason string) {
	if summary == nil {
		return "unknown", ""
	}

	if err := strings.TrimSpace(fmt.Sprint(summary["olt_profile_exec_error"])); err != "" && err != "<nil>" {
		return "failed", err
	}

	if note := strings.TrimSpace(fmt.Sprint(summary["onu_metrics_note"])); note != "" && note != "<nil>" {
		lower := strings.ToLower(note)
		if strings.Contains(lower, "falha") || strings.Contains(lower, "erro") || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
			return "failed", note
		}
	}

	if ok, exists := summaryBool(summary, "last_collect_ok"); exists {
		if !ok {
			reason := strings.TrimSpace(fmt.Sprint(summary["last_collect_error"]))
			if reason == "" || reason == "<nil>" {
				reason = "última coleta OLT falhou"
			}
			return "failed", reason
		}
	}

	if IsOltSnapshotIncomplete(summary) {
		reason := firstNonEmptySummary(summary,
			"onu_metrics_note",
			"onu_walk_note",
			"onu_delta_alerts_skipped",
			"olt_refresh_timeout_reason",
		)
		if reason == "" {
			reason = "coleta SNMP incompleta ou truncada"
		}
		return "partial", reason
	}

	if ok, exists := summaryBool(summary, "last_collect_ok"); exists && ok {
		return "ok", ""
	}

	return "unknown", ""
}

func summaryBool(summary map[string]any, key string) (bool, bool) {
	raw, ok := summary[key]
	if !ok || raw == nil {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "true" {
			return true, true
		}
		if s == "false" {
			return false, true
		}
	}
	return false, false
}

func firstNonEmptySummary(summary map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(fmt.Sprint(summary[k])); v != "" && v != "<nil>" {
			return v
		}
	}
	return ""
}
