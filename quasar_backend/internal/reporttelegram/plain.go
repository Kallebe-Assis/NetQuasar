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

	cols, _ := payload["columns"].([]string)
	rows, _ := payload["rows"].([][]string)
	if len(cols) > 0 && len(rows) > 0 {
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

	if chart, ok := payload["chart"].(map[string]any); ok {
		if pts, ok := chart["points"].([]map[string]any); ok && len(pts) > 0 {
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
				sb.WriteString(fmt.Sprintf("• %v — total %v | online %v | offline %v\n",
					p["t"], p["total"], p["online"], p["offline"]))
			}
		}
	}

	sb.WriteString("\n—\nNetQuasar · relatório")
	return strings.TrimSpace(sb.String())
}
