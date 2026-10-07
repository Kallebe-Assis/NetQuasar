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
			t.Fatalf("lote devia ter só prioritárias (sem serial): %v", r)
		}
	}
	// offset 7 sobre 5 prioritárias = posição 2 → ONUs 3,4,5; próximo offset = (2+3)%5 = 0.
	if intFromRow(batch[0], "onu") != 3 || nextOffset != 0 {
		t.Fatalf("rodízio sobre priority: primeiro=%d nextOffset=%d, want 3 e 0", intFromRow(batch[0], "onu"), nextOffset)
	}
}

// Reproduz o sintoma "serial só nas primeiras PONs": com muitas ONUs sem serial e lote pequeno, a
// fila prioritária fixa nunca chegava às PONs finais. Com rodízio, todas entram em algum ciclo.
func TestSelectOnuTelnetBatch_priorityRotationReachesLastPon(t *testing.T) {
	var priority []map[string]any
	for pon := 1; pon <= 8; pon++ {
		for onu := 1; onu <= 10; onu++ {
			priority = append(priority, rowWithSerial(pon, onu, "", false))
		}
	}
	seen := map[string]bool{}
	offset := 0
	for cycle := 0; cycle < 4; cycle++ { // 80 ONUs, lote de 25 → 4 ciclos cobrem tudo
		var batch []map[string]any
		batch, offset = selectOnuTelnetBatch(priority, nil, 25, offset)
		for _, r := range batch {
			seen[onuRowKey(r)] = true
		}
	}
	if len(seen) != 80 {
		t.Fatalf("ONUs atendidas em 4 ciclos = %d, want 80 (inclui PON 8)", len(seen))
	}
	if !seen["8.10"] {
		t.Fatal("última ONU da última PON nunca foi atendida")
	}
}
