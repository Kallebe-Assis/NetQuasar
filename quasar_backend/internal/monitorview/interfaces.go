package monitorview

import (
	"encoding/json"
	"strings"
)

// NormalizeInterfaceRows converte payload de interface_table (API) para linhas tipadas.
func NormalizeInterfaceRows(raw any) []InterfaceRow {
	tab, ok := raw.([]map[string]any)
	if !ok {
		if arr, ok2 := raw.([]any); ok2 {
			for _, item := range arr {
				m, _ := item.(map[string]any)
				if m != nil {
					tab = append(tab, m)
				}
			}
		}
	}
	if len(tab) == 0 {
		return nil
	}
	out := make([]InterfaceRow, 0, len(tab))
	for _, m := range tab {
		row := InterfaceRow{
			Index:       intFromAny(m["if_index"]),
			Name:        firstNonEmpty(m, "if_name", "descr", "display_name"),
			DisplayName: strVal(m["display_name"]),
			Type:        strVal(m["type"]),
			AdminStatus: strVal(m["admin_status"]),
			OperStatus:  strVal(m["oper_status"]),
			InBps:       floatFromAny(m["in_bps"]),
			OutBps:      floatFromAny(m["out_bps"]),
		}
		if v := floatPtrFromAny(m["rx_dbm"]); v != nil {
			row.RxDbm = v
		}
		if v := floatPtrFromAny(m["tx_dbm"]); v != nil {
			row.TxDbm = v
		}
		if v := floatPtrFromAny(m["speed_bps"]); v != nil {
			row.SpeedBps = v
		}
		if row.DisplayName == "" {
			row.DisplayName = row.Name
		}
		out = append(out, row)
	}
	return out
}

// TrafficPointsFromHistory converte histórico de tráfego para pontos de gráfico.
func TrafficPointsFromHistory(history map[int][]struct {
	Ts  int64
	Rx  float64
	Tx  float64
}) map[int][]TrafficPoint {
	if len(history) == 0 {
		return nil
	}
	out := make(map[int][]TrafficPoint, len(history))
	for idx, pts := range history {
		rows := make([]TrafficPoint, 0, len(pts))
		for _, p := range pts {
			rows = append(rows, TrafficPoint{Ts: p.Ts, RxBps: p.Rx, TxBps: p.Tx})
		}
		out[idx] = rows
	}
	return out
}

func strVal(v any) string {
	return strings.TrimSpace(strings.Trim(fmtAny(v), " "))
}

func fmtAny(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func firstNonEmpty(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := strVal(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func floatFromAny(v any) float64 {
	f := floatPtrFromAny(v)
	if f == nil {
		return 0
	}
	return *f
}

func floatPtrFromAny(v any) *float64 {
	switch x := v.(type) {
	case float64:
		if x != x || x > 1e18 {
			return nil
		}
		return &x
	case int:
		f := float64(x)
		return &f
	case int64:
		f := float64(x)
		return &f
	case json.Number:
		f, err := x.Float64()
		if err != nil || f != f || f > 1e18 {
			return nil
		}
		return &f
	case string:
		// ignore parse errors
		return nil
	default:
		return nil
	}
}
