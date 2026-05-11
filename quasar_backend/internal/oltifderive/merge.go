package oltifderive

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func ponIDKey(m map[string]any) string {
	return strings.TrimSpace(fmt.Sprint(m["id"]))
}

// StablePonRowKey é a chave usada para alinhar snapshots, estabilização e alarmes (prev vs cur).
// Ordem: Canónica (GPON0/N, VSOL, etc.) → id → name.
func StablePonRowKey(m map[string]any) string {
	if m == nil {
		return ""
	}
	k := CanonicalPonRowKey(m)
	if k == "" {
		k = ponIDKey(m)
	}
	if k == "" {
		k = strings.TrimSpace(fmt.Sprint(m["name"]))
	}
	return k
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+4)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func rowKeyForMerge(m map[string]any) string {
	return StablePonRowKey(m)
}

func rowPickInt(m map[string]any, keys ...string) int {
	for _, want := range keys {
		wantNorm := strings.ToLower(strings.TrimSpace(want))
		for k, v := range m {
			if strings.ToLower(strings.TrimSpace(k)) == wantNorm {
				return rowToInt(v)
			}
		}
	}
	return 0
}

func rowToInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		n, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
		return n
	}
}

func preferPonDisplayName(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if b == "" {
		return a
	}
	if a == "" {
		return b
	}
	au, bu := strings.ToUpper(a), strings.ToUpper(b)
	if strings.Contains(bu, "GPON") && !strings.Contains(au, "GPON") {
		return b
	}
	if strings.Contains(au, "GPON") && !strings.Contains(bu, "GPON") {
		return a
	}
	if len(b) > len(a) {
		return b
	}
	return a
}

// mergePonRowPair funde duas linhas do mesmo PON (totais = máximo por campo; preferir GPON no nome).
func mergePonRowPair(a, b map[string]any) map[string]any {
	out := cloneMap(a)
	tot := rowPickInt(out, "onu_total", "total_onu", "onus", "onus_total", "onu_count")
	if v := rowPickInt(b, "onu_total", "total_onu", "onus", "onus_total", "onu_count"); v > tot {
		tot = v
	}
	on := rowPickInt(out, "onu_online", "online", "onu_ok")
	if v := rowPickInt(b, "onu_online", "online", "onu_ok"); v > on {
		on = v
	}
	off := rowPickInt(out, "onu_offline", "offline", "onu_down")
	if v := rowPickInt(b, "onu_offline", "offline", "onu_down"); v > off {
		off = v
	}
	out["onu_total"] = tot
	out["onu_online"] = on
	out["onu_offline"] = off
	na := fmt.Sprint(out["name"])
	nb := fmt.Sprint(b["name"])
	out["name"] = preferPonDisplayName(na, nb)
	stA := fmt.Sprint(out["status"])
	stB := fmt.Sprint(b["status"])
	if stB == "vsol_snmp" && stA != "vsol_snmp" {
		out["status"] = b["status"]
	} else if stA == "" && stB != "" {
		out["status"] = b["status"]
	}
	if ss, ok := b["source_slice"]; ok && fmt.Sprint(out["source_slice"]) == "" {
		out["source_slice"] = ss
	}
	if v, ok := b["tx_dbm"]; ok && v != nil {
		if prev, ok2 := out["tx_dbm"]; !ok2 || prev == nil {
			out["tx_dbm"] = v
		}
	}
	if v, ok := b["rx_dbm"]; ok && v != nil {
		if prev, ok2 := out["rx_dbm"]; !ok2 || prev == nil {
			out["rx_dbm"] = v
		}
	}
	return out
}

// DedupePonMaps remove linhas duplicadas do mesmo PON (ex.: VSOL + IF com chaves distintas).
func DedupePonMaps(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}
	idx := map[string]map[string]any{}
	var order []string
	for _, row := range rows {
		k := rowKeyForMerge(row)
		if k == "" {
			continue
		}
		if prev, ok := idx[k]; ok {
			idx[k] = mergePonRowPair(prev, row)
			continue
		}
		idx[k] = cloneMap(row)
		order = append(order, k)
	}
	out := make([]map[string]any, 0, len(order))
	for _, k := range order {
		row := idx[k]
		if k != "" {
			row["id"] = k
		}
		out = append(out, row)
	}
	return out
}

// MergePonRowsForIfaceRefresh actualiza contagens IF-MIB; preserva tx/rx VSOL se já existirem.
func MergePonRowsForIfaceRefresh(existing []map[string]any, fromIf []map[string]any) []map[string]any {
	idx := map[string]map[string]any{}
	var order []string
	for _, row := range existing {
		id := rowKeyForMerge(row)
		if id == "" {
			continue
		}
		if prev, ok := idx[id]; ok {
			idx[id] = mergePonRowPair(prev, row)
			continue
		}
		idx[id] = cloneMap(row)
		order = append(order, id)
	}
	for _, row := range fromIf {
		id := rowKeyForMerge(row)
		if id == "" {
			continue
		}
		prev := idx[id]
		if prev == nil {
			idx[id] = cloneMap(row)
			order = append(order, id)
			continue
		}
		prev["onu_total"] = row["onu_total"]
		prev["onu_online"] = row["onu_online"]
		prev["onu_offline"] = row["onu_offline"]
		prev["status"] = row["status"]
		prev["source_slice"] = row["source_slice"]
		if v, ok := row["tx_dbm"]; ok && v != nil {
			prev["tx_dbm"] = v
		}
		if v, ok := row["rx_dbm"]; ok && v != nil {
			prev["rx_dbm"] = v
		}
		if nm, ok := row["name"].(string); ok && strings.TrimSpace(nm) != "" {
			prev["name"] = preferPonDisplayName(fmt.Sprint(prev["name"]), nm)
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, id := range order {
		row := idx[id]
		if id != "" {
			row["id"] = id
		}
		out = append(out, row)
	}
	return out
}

// MergeSummaryJSON combina JSON de summary existente com patch (preserva vsol_onu_rows, etc.).
func MergeSummaryJSON(cur []byte, patch map[string]any) ([]byte, error) {
	var m map[string]any
	if len(cur) > 0 && json.Unmarshal(cur, &m) == nil && m != nil {
	} else {
		m = map[string]any{}
	}
	for k, v := range patch {
		m[k] = v
	}
	return json.Marshal(m)
}
