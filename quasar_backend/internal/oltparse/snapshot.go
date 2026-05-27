package oltparse

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
)

func keyNorm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// SnapshotComputed extrai totais a partir de summary (objecto) e pons (array de objectos) com chaves flexíveis.
func SnapshotComputed(summaryJSON, ponsJSON []byte) map[string]any {
	out := map[string]any{
		"pon_count":       0,
		"onu_total_sum":   0,
		"onu_online_sum":  0,
		"onu_offline_sum": 0,
	}
	var sumObj map[string]any
	if len(summaryJSON) > 0 && json.Unmarshal(summaryJSON, &sumObj) == nil {
		for k, v := range sumObj {
			switch keyNorm(k) {
			case "pon_count", "pons", "total_pons":
				if n, ok := toInt(v); ok {
					out["pon_count"] = n
				}
			case "onu_total", "total_onu", "onus_total":
				if n, ok := toInt(v); ok {
					out["onu_total_sum"] = n
				}
			case "onu_online", "online":
				if n, ok := toInt(v); ok {
					out["onu_online_sum"] = n
				}
			case "onu_offline", "offline":
				if n, ok := toInt(v); ok {
					out["onu_offline_sum"] = n
				}
			}
		}
	}

	var ponsArr []any
	if len(ponsJSON) == 0 || json.Unmarshal(ponsJSON, &ponsArr) != nil {
		return out
	}
	var ponsMaps []map[string]any
	for _, it := range ponsArr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ponsMaps = append(ponsMaps, m)
	}
	ponsMaps = oltifderive.DedupePonMaps(ponsMaps)
	nPon := len(ponsMaps)
	if nPon > 0 {
		out["pon_count"] = nPon
	}
	tot, on, off := 0, 0, 0
	for _, m := range ponsMaps {
		oltifderive.NormalizePonONUCounts(m)
		tot += pickInt(m, "onu_total", "total_onu", "onus", "onus_total", "onu_count")
		on += pickInt(m, "onu_online", "online", "onu_ok")
		off += pickInt(m, "onu_offline", "offline", "onu_down")
	}
	if on+off > tot && tot > 0 {
		off = tot - on
		if off < 0 {
			off = 0
		}
	}
	if tot > 0 || on > 0 || off > 0 {
		out["onu_total_sum"] = tot
		out["onu_online_sum"] = on
		out["onu_offline_sum"] = off
	}
	return out
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func pickInt(m map[string]any, keys ...string) int {
	for _, want := range keys {
		for k, v := range m {
			if keyNorm(k) == keyNorm(want) {
				if n, ok := toInt(v); ok {
					return n
				}
			}
		}
	}
	return 0
}

// PonRows normaliza cada elemento do array pons para o frontend (chaves canónicas).
func PonRows(ponsJSON []byte) []map[string]any {
	var ponsArr []any
	if json.Unmarshal(ponsJSON, &ponsArr) != nil {
		return nil
	}
	var ponsMaps []map[string]any
	for _, it := range ponsArr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ponsMaps = append(ponsMaps, m)
	}
	ponsMaps = oltifderive.DedupePonMaps(ponsMaps)
	var out []map[string]any
	for i, m := range ponsMaps {
		oltifderive.NormalizePonONUCounts(m)
		id := firstStr(m, "pon", "pon_id", "id", "name", "if_index")
		name := firstStr(m, "name", "description", "descr", "pon")
		if name == "" {
			name = id
		}
		row := map[string]any{
			"_index":      i,
			"id":          id,
			"name":        name,
			"rx_dbm":      nil,
			"tx_dbm":      nil,
			"onu_total":   pickInt(m, "onu_total", "total_onu", "onus", "onus_total", "onu_count"),
			"onu_online":  pickInt(m, "onu_online", "online", "onu_ok"),
			"onu_offline": pickInt(m, "onu_offline", "offline", "onu_down"),
			"status":      firstStr(m, "status", "state", "oper_status"),
		}
		oltifderive.NormalizePonONUCounts(row)
		if f, ok := firstFloat(m, "rx_dbm", "rx", "pon_rx", "optical_rx"); ok {
			row["rx_dbm"] = f
		}
		if f, ok := firstFloat(m, "tx_dbm", "tx", "pon_tx", "optical_tx"); ok {
			row["tx_dbm"] = f
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		ai := strings.TrimSpace(toString(out[i]["id"]))
		aj := strings.TrimSpace(toString(out[j]["id"]))
		if ai != "" && aj != "" {
			return lessPonNatural(ai, aj)
		}
		return lessPonNatural(strings.TrimSpace(toString(out[i]["name"])), strings.TrimSpace(toString(out[j]["name"])))
	})
	for i := range out {
		out[i]["_index"] = i
	}
	return out
}

func ponParts(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := make([]int, 0, 4)
	n := 0
	in := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			in = true
			continue
		}
		if in {
			out = append(out, n)
			n = 0
			in = false
		}
	}
	if in {
		out = append(out, n)
	}
	return out
}

func lessPonNatural(a, b string) bool {
	pa, pb := ponParts(a), ponParts(b)
	if len(pa) > 0 && len(pb) > 0 {
		n := len(pa)
		if len(pb) < n {
			n = len(pb)
		}
		for i := 0; i < n; i++ {
			if pa[i] == pb[i] {
				continue
			}
			return pa[i] < pb[i]
		}
		if len(pa) != len(pb) {
			return len(pa) < len(pb)
		}
	}
	return strings.ToLower(strings.TrimSpace(a)) < strings.ToLower(strings.TrimSpace(b))
}

func firstStr(m map[string]any, keys ...string) string {
	for _, want := range keys {
		for k, v := range m {
			if keyNorm(k) == keyNorm(want) {
				s := strings.TrimSpace(toString(v))
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func firstFloat(m map[string]any, keys ...string) (float64, bool) {
	for _, want := range keys {
		for k, v := range m {
			if keyNorm(k) == keyNorm(want) {
				return toFloat(v)
			}
		}
	}
	return 0, false
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return ""
	}
}
