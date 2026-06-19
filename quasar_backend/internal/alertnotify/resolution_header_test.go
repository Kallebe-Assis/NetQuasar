package alertnotify

import "testing"

func TestResolutionHeaderLatencyNotOnline(t *testing.T) {
	got := resolutionHeader("latency_high", "Latência voltou ao intervalo normal",
		"OLT (10.0.0.1): latência ICMP/TCP em 45 ms (acima do limiar).")
	if got != "🟢 ALERTA RESOLVIDO" {
		t.Fatalf("latency_high resolve header = %q, want ALERTA RESOLVIDO", got)
	}
}

func TestResolutionHeaderPingOnline(t *testing.T) {
	got := resolutionHeader("ping_unreachable", "Equipamento voltou a responder (ICMP/TCP)", "detalhe")
	if got != "🟢 EQUIPAMENTO ONLINE" {
		t.Fatalf("ping_unreachable header = %q", got)
	}
}
