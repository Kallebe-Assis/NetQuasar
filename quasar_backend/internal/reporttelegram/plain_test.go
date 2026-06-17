package reporttelegram

import "testing"

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
	if contains := func(s, sub string) bool { return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0) }; !contains(text, "Alertas ativos") {
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
