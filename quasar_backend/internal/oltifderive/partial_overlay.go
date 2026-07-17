package oltifderive

import "strings"

// OverlayPartialPonSnapshot funde coleta leve sobre o snapshot anterior.
// Preserva campos de detalhe (óptica, nomes) quando o modo parcial não os actualiza.
func OverlayPartialPonSnapshot(prev, cur []map[string]any, mode string) []map[string]any {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if len(cur) == 0 {
		return prev
	}
	if len(prev) == 0 || !isPartialOltMode(mode) {
		return cur
	}

	prevBy := map[string]map[string]any{}
	prevOrder := make([]string, 0, len(prev))
	for _, p := range prev {
		k := StablePonRowKey(p)
		if k == "" {
			continue
		}
		prevBy[k] = cloneMap(p)
		prevOrder = append(prevOrder, k)
	}

	seen := map[string]struct{}{}
	out := make([]map[string]any, 0, len(cur)+len(prev))
	for _, row := range cur {
		k := StablePonRowKey(row)
		base := cloneMap(row)
		if k != "" {
			seen[k] = struct{}{}
			if old, ok := prevBy[k]; ok {
				base = overlayPonRowPartial(old, base, mode)
			}
		}
		out = append(out, base)
	}
	// Mantém PONs que não vieram na coleta leve (ex.: só status noutras portas).
	for _, k := range prevOrder {
		if _, ok := seen[k]; ok {
			continue
		}
		out = append(out, cloneMap(prevBy[k]))
	}
	return DedupePonMaps(out)
}

func isPartialOltMode(mode string) bool {
	switch mode {
	case "baseline", "pon_status", "onu_counts", "status_only", "status_rx":
		return true
	default:
		return false
	}
}

func overlayPonRowPartial(prev, cur map[string]any, mode string) map[string]any {
	out := cloneMap(prev)
	switch mode {
	case "baseline":
		// Linha-base: status ONU/PON e TX da PON; preserva RX/serial/modelo anteriores.
		copyPonKeys(out, cur,
			"oper_status", "admin_status", "if_oper_status", "status", "pon_status", "up", "link",
			"onu_online", "onu_offline", "onu_total", "onu_status_unknown", "source_slice",
			"tx_dbm", "pon_tx_dbm",
		)
	case "pon_status":
		copyPonKeys(out, cur, "oper_status", "admin_status", "if_oper_status", "status", "pon_status", "up", "link")
	case "onu_counts":
		copyPonKeys(out, cur, "onu_online", "onu_offline", "onu_total", "onu_status_unknown", "status", "source_slice")
	case "status_only":
		copyPonKeys(out, cur,
			"oper_status", "admin_status", "if_oper_status", "status", "pon_status", "up", "link",
			"onu_online", "onu_offline", "onu_total", "onu_status_unknown", "source_slice",
		)
	case "status_rx":
		copyPonKeys(out, cur,
			"oper_status", "admin_status", "if_oper_status", "status", "pon_status", "up", "link",
			"onu_online", "onu_offline", "onu_total", "onu_status_unknown", "source_slice",
			"rx_dbm", "pon_rx_dbm", "tx_dbm",
		)
	}
	// Identificadores / IF-MIB
	copyPonKeys(out, cur, "id", "name", "pon", "if_index", "pon_compact")
	return out
}

func copyPonKeys(dst, src map[string]any, keys ...string) {
	for _, k := range keys {
		if v, ok := src[k]; ok && v != nil {
			dst[k] = v
		}
	}
}

// StripPartialSummaryKeys remove chaves de detalhe ONU para o merge JSONB não apagar o snapshot anterior.
func StripPartialSummaryKeys(summary map[string]any, mode string) {
	if summary == nil || !isPartialOltMode(mode) {
		return
	}
	delete(summary, "vsol_onu_rows")
	delete(summary, "vsol_onu_count")
	if mode == "pon_status" {
		delete(summary, "vsol_onu_online")
		delete(summary, "vsol_onu_offline")
	}
}
