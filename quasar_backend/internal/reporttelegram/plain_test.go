package reporttelegram

import (
	"fmt"
	"testing"
)

func TestComposeSystemReportEquipmentByPop(t *testing.T) {
	text := ComposeSystemReport("Equipamentos por POP", map[string]any{
		"generated_at": "2026-06-05T12:30:00Z",
		"summary": map[string]any{
			"POPs":                 int64(2),
			"Equipamentos (total)": int64(3),
		},
		"groups": []map[string]any{
			{
				"pop": "Central",
				"coordinates": "-23.550520, -46.633308",
				"devices": []map[string]any{
					{"name": "OLT-01", "category": "OLT", "label": "OLT-01 [OLT]"},
				},
			},
			{
				"pop": "Norte",
				"devices": []map[string]any{
					{"name": "MK-01", "category": "Mikrotik", "label": "MK-01 [Mikrotik]"},
				},
			},
		},
		"columns": []string{"POP", "Equipamento"},
		"rows": [][]string{
			{"Central", "OLT-01 [OLT]"},
			{"Central", "BNG-01 [BNG]"},
			{"Norte", "MK-01 [Mikrotik]"},
		},
	})
	if indexOf(text, "Central") < 0 || indexOf(text, "OLT-01 [OLT]") < 0 {
		t.Fatalf("missing grouped content: %q", text)
	}
	if indexOf(text, "-23.550520") < 0 {
		t.Fatalf("missing coordinates: %q", text)
	}
	if indexOf(text, "Detalhes (") >= 0 {
		t.Fatalf("should not use flat details table when groups present: %q", text)
	}
	if indexOf(text, "Quantidade") >= 0 {
		t.Fatalf("should not mention quantity: %q", text)
	}
}

func TestComposeSystemReportOnuPerPonCompact(t *testing.T) {
	rows := make([][]string, 0, 60)
	for i := 1; i <= 30; i++ {
		rows = append(rows, []string{"OLT-A", fmt.Sprintf("%d", i), fmt.Sprintf("PON%d", i), "64", "60", "4", "2026-06-05T12:30:00Z"})
	}
	for i := 1; i <= 30; i++ {
		rows = append(rows, []string{"OLT-B", fmt.Sprintf("%d", i), "", "32", "30", "2", "2026-06-05T12:30:00Z"})
	}
	text := ComposeSystemReport("ONUs por PON", map[string]any{
		"generated_at": "2026-06-05T12:30:00Z",
		"columns":      []string{"OLT", "PON", "Nome PON", "Total", "Online", "Offline", "Snapshot"},
		"rows":         rows,
		"summary": map[string]any{
			"Portas PON": 60,
		},
	})
	if indexOf(text, "• PON 1 (PON1): 64 total · 60 on · 4 off") < 0 {
		t.Fatalf("missing compact line: %q", text)
	}
	if indexOf(text, "OLT:") >= 0 {
		t.Fatalf("should not use verbose per-field layout: %q", text)
	}
	if len(text) > 3900 {
		t.Fatalf("compact report still too long: %d", len(text))
	}
}

func TestComposeSystemReportPlainText(t *testing.T) {
	text := ComposeSystemReport("Alertas ativos", map[string]any{
		"generated_at": "2026-06-05T12:30:00Z",
		"description":  "Todos os alertas em aberto.",
		"summary": map[string]any{
			"Total": 2,
			"Por tipo": map[string]int{
				"dhcp":  10,
				"pppoe": 5,
			},
		},
		"columns": []string{"Equipamento", "IP"},
		"rows":    [][]string{{"OLT-A", "10.0.0.1"}},
	})
	if text == "" {
		t.Fatal("empty text")
	}
	if indexOf(text, "Alertas ativos") < 0 {
		t.Fatalf("missing title: %q", text)
	}
	if indexOf(text, "<pre>") >= 0 || indexOf(text, "map[") >= 0 {
		t.Fatalf("should be plain readable text: %q", text)
	}
}


func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
