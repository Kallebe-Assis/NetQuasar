package reporttelegram

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatGeneratedAt formata RFC3339 para leitura em português.
func FormatGeneratedAt(iso string) string {
	iso = strings.TrimSpace(iso)
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.Local().Format("02/01/2006 15:04")
}

// FormatValue formata valores de resumo (inclui mapas) em texto legível.
func FormatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case string:
		if strings.TrimSpace(x) == "" {
			return "—"
		}
		return x
	case bool:
		if x {
			return "Sim"
		}
		return "Não"
	case int:
		return fmt.Sprintf("%d", x)
	case int32:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float32:
		return fmt.Sprintf("%.2f", x)
	case float64:
		return fmt.Sprintf("%.2f", x)
	case map[string]any:
		return formatStringMap(x)
	case map[string]int:
		m := make(map[string]any, len(x))
		for k, n := range x {
			m[k] = n
		}
		return formatStringMap(m)
	case map[string]int64:
		m := make(map[string]any, len(x))
		for k, n := range x {
			m[k] = n
		}
		return formatStringMap(m)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func formatStringMap(m map[string]any) string {
	if len(m) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("  • %s: %s", k, FormatValue(m[k])))
	}
	return "\n" + strings.Join(lines, "\n")
}

// ComposeSystemReport monta mensagem Telegram em texto simples (sem HTML/Markdown).
func ComposeSystemReport(title string, payload map[string]any) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(title))
	sb.WriteString("\n")
	if gen, ok := payload["generated_at"].(string); ok && gen != "" {
		sb.WriteString("Gerado: ")
		sb.WriteString(FormatGeneratedAt(gen))
		sb.WriteString("\n")
	}
	if desc, ok := payload["description"].(string); ok && strings.TrimSpace(desc) != "" {
		sb.WriteString(strings.TrimSpace(desc))
		sb.WriteString("\n")
	}

	if summary, ok := payload["summary"].(map[string]any); ok && len(summary) > 0 {
		sb.WriteString("\nResumo\n")
		keys := make([]string, 0, len(summary))
		for k := range summary {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := FormatValue(summary[k])
			if strings.HasPrefix(val, "\n") {
				sb.WriteString(k)
				sb.WriteString(":")
				sb.WriteString(val)
				sb.WriteString("\n")
			} else {
				sb.WriteString(fmt.Sprintf("• %s: %s\n", k, val))
			}
		}
	}

	wroteGroups := writeEquipmentByPopGroups(&sb, payload)

	cols, _ := payload["columns"].([]string)
	rows, _ := payload["rows"].([][]string)
	if !wroteGroups && len(cols) > 0 && len(rows) > 0 {
		if isOnuPerPonColumns(cols) {
			writeOnuPerPonCompact(&sb, cols, rows)
		} else {
			sb.WriteString(fmt.Sprintf("\nDetalhes (%d linha(s))\n", len(rows)))
			limit := 40
			for i, row := range rows {
				if i >= limit {
					sb.WriteString(fmt.Sprintf("… e mais %d linha(s)\n", len(rows)-limit))
					break
				}
				sb.WriteString(fmt.Sprintf("\n%d) ", i+1))
				parts := make([]string, 0, len(row))
				for j, cell := range row {
					col := ""
					if j < len(cols) {
						col = cols[j]
					}
					cell = strings.TrimSpace(cell)
					if cell == "" {
						continue
					}
					if col == "Última coleta" && strings.Contains(cell, "T") {
						cell = FormatGeneratedAt(cell)
					}
					if col != "" {
						parts = append(parts, fmt.Sprintf("%s: %s", col, cell))
					} else {
						parts = append(parts, cell)
					}
				}
				sb.WriteString(strings.Join(parts, "\n   "))
				sb.WriteString("\n")
			}
		}
	}

	if chart, ok := payload["chart"].(map[string]any); ok {
		chartKind, _ := chart["kind"].(string)
		if chartKind == "bng-subscribers" {
			if av, ok := payload["averages"].(map[string]any); ok {
				writeBngLoginAverages(&sb, av)
			}
		} else if pts, ok := chart["points"].([]map[string]any); ok && len(pts) > 0 {
			label := "Evolução"
			if l, ok := chart["label"].(string); ok && l != "" {
				label = l
			}
			sb.WriteString("\n")
			sb.WriteString(label)
			sb.WriteString("\n")
			for i, p := range pts {
				if i >= 14 {
					sb.WriteString("… (mais pontos no relatório completo)\n")
					break
				}
				if _, hasOnline := p["online"]; hasOnline {
					sb.WriteString(fmt.Sprintf("• %v — total %v | online %v | offline %v\n",
						p["t"], p["total"], p["online"], p["offline"]))
					continue
				}
				ts := p["t"]
				if ca, ok := p["collected_at"]; ok && ca != nil {
					ts = ca
				}
				tsLabel := fmt.Sprint(ts)
				if dev, ok := p["device"].(string); ok && strings.TrimSpace(dev) != "" {
					tsLabel = fmt.Sprintf("%s (%s)", FormatGeneratedAt(fmt.Sprint(ts)), dev)
				} else if strings.Contains(tsLabel, "T") {
					tsLabel = FormatGeneratedAt(tsLabel)
				}
				parts := []string{tsLabel}
				for _, k := range []string{"total", "pppoe", "ipv4", "ipv6", "dual_stack"} {
					if v, ok := p[k]; ok && v != nil && fmt.Sprint(v) != "" {
						parts = append(parts, fmt.Sprintf("%s %v", k, v))
					}
				}
				sb.WriteString("• " + strings.Join(parts, " | ") + "\n")
			}
		}
	}

	sb.WriteString("\n—\nNetQuasar · relatório")
	return strings.TrimSpace(sb.String())
}

func writeEquipmentByPopGroups(sb *strings.Builder, payload map[string]any) bool {
	groups := coerceAnySlice(payload["groups"])
	if len(groups) == 0 {
		return false
	}
	sb.WriteString("\n")
	for gi, item := range groups {
		g := coerceStringMap(item)
		if g == nil {
			continue
		}
		pop := strings.TrimSpace(fmt.Sprint(g["pop"]))
		if pop == "" || pop == "<nil>" {
			continue
		}
		if gi > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pop)
		sb.WriteString("\n")
		if coords := strings.TrimSpace(fmt.Sprint(g["coordinates"])); coords != "" && coords != "<nil>" {
			sb.WriteString("  ")
			sb.WriteString(coords)
			sb.WriteString("\n")
		}
		for _, dv := range coerceAnySlice(g["devices"]) {
			dm := coerceStringMap(dv)
			if dm == nil {
				continue
			}
			label := deviceLabelFromMap(dm)
			if label == "" {
				continue
			}
			sb.WriteString("  • ")
			sb.WriteString(label)
			sb.WriteString("\n")
		}
	}
	return true
}

func deviceLabelFromMap(dm map[string]any) string {
	label := strings.TrimSpace(fmt.Sprint(dm["label"]))
	if label != "" && label != "<nil>" {
		return label
	}
	name := strings.TrimSpace(fmt.Sprint(dm["name"]))
	cat := strings.TrimSpace(fmt.Sprint(dm["category"]))
	if name == "" || name == "<nil>" {
		return ""
	}
	if cat != "" && cat != "<nil>" {
		return fmt.Sprintf("%s [%s]", name, cat)
	}
	return name
}

func coerceAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, len(x))
		for i, m := range x {
			out[i] = m
		}
		return out
	case [][]string:
		out := make([]any, len(x))
		for i, row := range x {
			out[i] = row
		}
		return out
	default:
		return nil
	}
}

func coerceStringMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func writeBngLoginAverages(sb *strings.Builder, av map[string]any) {
	wins, ok := av["windows"].([]map[string]any)
	if !ok || len(wins) == 0 {
		return
	}
	metricLabels := map[string]string{
		"pppoe":      "PPPoE",
		"ipv4":       "IPv4",
		"ipv6":       "IPv6",
		"dual_stack": "Dual-stack",
		"total":      "Total",
	}
	sb.WriteString("\nMédias de logins\n")
	for i, w := range wins {
		if i > 0 {
			sb.WriteString("\n")
		}
		label := fmt.Sprint(w["label"])
		if label == "" || label == "<nil>" {
			label = fmt.Sprintf("%v dias", w["days"])
		}
		sb.WriteString(label)
		sb.WriteString("\n")
		if n, ok := w["samples"]; ok {
			sb.WriteString(fmt.Sprintf("  %v coletas\n", n))
		}
		for _, k := range []string{"pppoe", "ipv4", "ipv6", "dual_stack", "total"} {
			if v, ok := w[k]; ok && v != nil {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", metricLabels[k], v))
			}
		}
	}
}

func isOnuPerPonColumns(cols []string) bool {
	if len(cols) < 6 {
		return false
	}
	return cols[0] == "OLT" && cols[1] == "PON" && cols[3] == "Total" && cols[4] == "Online" && cols[5] == "Offline"
}

// writeOnuPerPonCompact — uma linha por PON (evita mensagem Telegram > 4096).
func writeOnuPerPonCompact(sb *strings.Builder, cols []string, rows [][]string) {
	_ = cols
	sb.WriteString(fmt.Sprintf("\nDetalhes (%d porta(s) PON)\n", len(rows)))
	curOLT := ""
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		olt := strings.TrimSpace(row[0])
		pon := strings.TrimSpace(row[1])
		name := strings.TrimSpace(row[2])
		total, on, off := strings.TrimSpace(row[3]), strings.TrimSpace(row[4]), strings.TrimSpace(row[5])
		if olt != curOLT {
			curOLT = olt
			sb.WriteString("\n")
			sb.WriteString(olt)
			sb.WriteString("\n")
		}
		label := pon
		if name != "" && name != pon {
			label = fmt.Sprintf("%s (%s)", pon, name)
		}
		sb.WriteString(fmt.Sprintf("• PON %s: %s total · %s on · %s off\n", label, total, on, off))
	}
}
