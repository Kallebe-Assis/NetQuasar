package alertnotify

import (
	"strings"
	"testing"
)

func TestTelegramMostraNomeCadastradoDaInterface(t *testing.T) {
	meta := map[string]any{"pon": "1/1/3", "interface_custom_name": "PON Centro — Bairro Alto"}
	got := telegramMonitoringBlocksWithContext("CRITICAL", "PON inactiva (DOWN)", "PON 1/1/3 — status DOWN", "OLT Matriz", "10.0.0.2", "pon_down", meta)
	if !strings.Contains(got, "• Nome da interface: PON Centro — Bairro Alto") {
		t.Fatalf("faltou o nome da interface:\n%s", got)
	}
	// não repete quando o texto já traz o nome
	meta2 := map[string]any{"display_name": "sfp9 (REDE-X)", "interface_custom_name": "REDE-X"}
	got2 := telegramMonitoringBlocksWithContext("WARNING", "Potência óptica SFP", "interface sfp9 (REDE-X) — RX -19", "CCR", "10.0.0.1", "mikrotik_sfp_rx", meta2)
	if strings.Count(got2, "REDE-X") != 1 {
		t.Fatalf("nome repetido:\n%s", got2)
	}
}
