package oltcollect

import "testing"

func rowWithSerial(pon, onu int, serial string, online bool) map[string]any {
	return map[string]any{"pon": pon, "onu": onu, "serial": serial, "online": online}
}

func TestBuildOnuTelnetCandidates_missingSerialAlwaysPriority(t *testing.T) {
	rows := []map[string]any{
		rowWithSerial(1, 1, "ITBSCF8F197A", true),
		rowWithSerial(1, 2, "", false),          // offline, sem serial
		rowWithSerial(1, 3, "3", true),          // "serial" implausível (código de fase)
		rowWithSerial(1, 4, "OK5678901", false), // offline mas já tem serial
	}
	cfg := OnuReportConfig{MonitorOnlineOnly: true}
	priority, rest := buildOnuTelnetCandidates(rows, cfg)

	if len(priority) != 2 {
		t.Fatalf("priority = %d rows, want 2 (onu 2 e onu 3)", len(priority))
	}
	for _, r := range priority {
		if rowHasPlausibleSerial(r) {
			t.Fatalf("linha com serial plausível entrou em priority: %v", r)
		}
	}
	// MonitorOnlineOnly=true: das que já têm serial, só a online (onu 1) deveria ficar em rest —
	// onu 4 está offline e tem serial, então fica de fora (comportamento antigo preservado).
	if len(rest) != 1 || intFromRow(rest[0], "onu") != 1 {
		t.Fatalf("rest = %v, want só a ONU 1 (online, com serial)", rest)
	}
}

func TestBuildOnuTelnetCandidates_missingSerialIgnoresOnlineOnlyGate(t *testing.T) {
	// Sem isto, uma ONU offline sem serial nunca seria candidata com MonitorOnlineOnly=true —
	// exactamente o bug relatado: "quando eu clico pra atualizar funciona, mas a coleta nunca
	// chega nela".
	rows := []map[string]any{rowWithSerial(2, 5, "", false)}
	cfg := OnuReportConfig{MonitorOnlineOnly: true}
	priority, rest := buildOnuTelnetCandidates(rows, cfg)
	if len(priority) != 1 || len(rest) != 0 {
		t.Fatalf("priority=%d rest=%d, want priority=1 rest=0 (offline sem serial ainda deve ser candidata)", len(priority), len(rest))
	}
}

func TestSelectOnuTelnetBatch_priorityAlwaysIncluded(t *testing.T) {
	priority := []map[string]any{rowWithSerial(1, 1, "", false), rowWithSerial(1, 2, "", false)}
	rest := []map[string]any{
		rowWithSerial(1, 3, "A1111111", true),
		rowWithSerial(1, 4, "A2222222", true),
		rowWithSerial(1, 5, "A3333333", true),
	}
	batch, nextOffset := selectOnuTelnetBatch(priority, rest, 3, 0)
	if len(batch) != 3 {
		t.Fatalf("batch = %d, want 3 (2 prioritárias + 1 do rodízio)", len(batch))
	}
	if intFromRow(batch[0], "onu") != 1 || intFromRow(batch[1], "onu") != 2 {
		t.Fatalf("prioritárias não vieram primeiro: %v", batch)
	}
	if intFromRow(batch[2], "onu") != 3 {
		t.Fatalf("terceiro item devia ser o início do rodízio sobre rest: %v", batch[2])
	}
	if nextOffset != 1 {
		t.Fatalf("nextOffset = %d, want 1", nextOffset)
	}
}

func TestSelectOnuTelnetBatch_priorityExceedsBudget(t *testing.T) {
	priority := make([]map[string]any, 0, 5)
	for i := 1; i <= 5; i++ {
		priority = append(priority, rowWithSerial(1, i, "", false))
	}
	rest := []map[string]any{rowWithSerial(1, 99, "AAAA1111", true)}
	batch, nextOffset := selectOnuTelnetBatch(priority, rest, 3, 7)
	if len(batch) != 3 {
		t.Fatalf("batch = %d, want 3 (só prioritárias, orçamento esgotado)", len(batch))
	}
	for _, r := range batch {
		if rowHasPlausibleSerial(r) {
			t.Fatalf("rotação sobre `rest` não devia ter avançado (sem orçamento sobrando): %v", r)
		}
	}
	if nextOffset != 7 {
		t.Fatalf("nextOffset = %d, want inalterado (7) — rodízio sobre rest não avançou", nextOffset)
	}
}
