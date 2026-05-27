package api

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

// buildOltCollectionLog resume passos e métricas VSOL/ZTE para a UI de debug.
func buildOltCollectionLog(summary map[string]any) map[string]any {
	if summary == nil {
		return map[string]any{"steps": []any{}}
	}
	log := map[string]any{
		"scope":              stringOr(summary["olt_refresh_scope"], "full"),
		"elapsed_ms":         summary["olt_refresh_elapsed_ms"],
		"mode":               summary["olt_collection_mode"],
		"profile_error":      summary["olt_profile_error"],
		"profile_exec":       summary["olt_profile_exec_error"],
		"refresh_timeout":    summary["olt_refresh_timeout"],
		"refresh_cancelled":  summary["olt_refresh_cancelled"],
		"steps":           summary["olt_profile_steps"],
		"vsol_steps":      summary["vsol_collect_steps"],
		"vsol": map[string]any{
			"refs":             summary["vsol_onu_refs_count"],
			"onus_parsed":      summary["vsol_onu_table_count"],
			"snmp_vars":        summary["vsol_snmp_var_count"],
			"online_complete":  summary["vsol_online_complete"],
			"walk_truncated":   summary["vsol_walk_truncated"],
			"partial":          summary["vsol_get_partial"],
			"note":             firstNonEmpty(summary, "vsol_get_note", "vsol_walk_note"),
			"if_preloaded":     summary["vsol_if_preloaded"],
			"if_mib_source":    summary["if_mib_source"],
			"if_mib_onu_ifaces": summary["if_mib_onu_ifaces"],
		},
	}
	if zte, ok := summary["zte_telnet_onu_state_count"]; ok {
		log["zte"] = map[string]any{
			"telnet_rows":   zte,
			"telnet_applied": summary["zte_telnet_applied"],
			"telnet_note":   summary["telnet_output_note"],
			"telnet_error":  summary["telnet_output_error"],
		}
	}
	if dbg, ok := summary["snmp_debug"].(map[string]any); ok && dbg != nil {
		log["has_snmp_debug"] = true
	}
	return log
}

func enrichOltStepLogEntry(entry map[string]any, method string, summary map[string]any) {
	if entry == nil || summary == nil {
		return
	}
	detail := map[string]any{}
	switch strings.TrimSpace(method) {
	case oltcollect.MethodIfMibRefresh:
		detail["rows"] = summary["if_mib_refresh_rows"]
		detail["truncated"] = summary["if_mib_refresh_truncated"]
		detail["note"] = summary["if_mib_refresh_note"]
	case oltcollect.MethodIfMibSnapshot:
		detail["refs"] = summary["vsol_onu_refs_count"]
		detail["if_mib_source"] = summary["if_mib_source"]
		detail["if_mib_onu_ifaces"] = summary["if_mib_onu_ifaces"]
		detail["note"] = summary["if_mib_note"]
	case oltcollect.MethodOnuMetricsCollect:
		detail["metrics"] = summary["onu_metrics_count"]
		detail["onus_parsed"] = summary["vsol_onu_count"]
		detail["online"] = summary["vsol_onu_online"]
		detail["offline"] = summary["vsol_onu_offline"]
		detail["walks"] = summary["onu_metrics_walks"]
		detail["note"] = firstNonEmpty(summary, "onu_metrics_note", "vsol_get_note")
	case oltcollect.MethodOnuSNMPWalk:
		detail["oid"] = summary["onu_walk_oid"]
		detail["vars"] = summary["onu_walk_var_count"]
		detail["onus_parsed"] = summary["vsol_onu_count"]
		detail["online"] = summary["vsol_onu_online"]
		detail["offline"] = summary["vsol_onu_offline"]
		detail["elapsed_ms"] = summary["vsol_walk_elapsed_ms"]
		detail["note"] = firstNonEmpty(summary, "onu_walk_note", "vsol_get_note")
		detail["truncated"] = summary["onu_walk_truncated"]
	case oltcollect.MethodVsolOnuCollect:
		detail["refs"] = summary["vsol_onu_refs_count"]
		detail["onus_parsed"] = summary["vsol_onu_table_count"]
		detail["snmp_vars"] = summary["vsol_snmp_var_count"]
		detail["online_complete"] = summary["vsol_online_complete"]
		detail["truncated"] = summary["vsol_walk_truncated"]
		detail["note"] = firstNonEmpty(summary, "vsol_get_note", "vsol_walk_note")
		if steps, ok := summary["vsol_collect_steps"]; ok {
			detail["snmp_steps"] = steps
		}
	case oltcollect.MethodTelnet:
		detail["rows"] = summary["zte_telnet_onu_state_count"]
		detail["applied"] = summary["zte_telnet_applied"]
		detail["note"] = firstNonEmpty(summary, "telnet_output_note", "pons_note")
		detail["error"] = summary["telnet_output_error"]
	case oltcollect.MethodSNMPWalk:
		store := strings.TrimSpace(anyString(entry["id"]))
		if store == "" {
			store = "snmp_walk"
		}
		detail["note"] = summary[store+"_note"]
		detail["truncated"] = summary[store+"_truncated"]
	}
	if len(detail) > 0 {
		entry["detail"] = detail
	}
}

func firstNonEmpty(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && strings.TrimSpace(anyString(v)) != "" {
			return v
		}
	}
	return nil
}

func stringOr(v any, def string) string {
	s := strings.TrimSpace(anyString(v))
	if s == "" {
		return def
	}
	return s
}
