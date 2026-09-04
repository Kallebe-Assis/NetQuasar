package oltcollect

import (
	"encoding/json"
	"testing"
)

func TestIsPlausibleOnuSerial(t *testing.T) {
	cases := map[string]bool{
		"":             false,
		"3":            false,
		"6":            false,
		"  4  ":        false,
		"ITBSCF8F197A": true,
		"000062F89D3A": true,
	}
	for in, want := range cases {
		if got := IsPlausibleOnuSerial(in); got != want {
			t.Fatalf("IsPlausibleOnuSerial(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestSanitizeOnuSerialsMap cobre o bug real em produção: um perfil de OLT mal configurado
// (OID de "serial" a apontar para a tabela de status) faz qualquer coletor devolver um código
// de estado curto ("3") como se fosse o serial. SanitizeOnuSerialsMap é a última linha de
// defesa antes da gravação em olt_snapshots — deve remover só os seriais implausíveis, sem
// mexer nos outros campos da linha nem nas linhas com serial válido.
func TestSanitizeOnuSerialsMap(t *testing.T) {
	summary := map[string]any{
		"vsol_onu_rows": []any{
			map[string]any{"pon": 1, "onu": 1, "serial": "3", "model": "D401", "onu_online_sta": 3},
			map[string]any{"pon": 1, "onu": 2, "serial": "ITBSCF8F197A", "model": "HG8010H"},
			map[string]any{"pon": 2, "onu": 1, "model": "sem-serial-nenhum"},
		},
	}
	cleaned := SanitizeOnuSerialsMap(summary)
	if cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", cleaned)
	}
	rows := OnuRowsFromSummary(summary)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (não deve apagar a linha, só o campo serial)", len(rows))
	}
	if _, ok := rows[0]["serial"]; ok {
		t.Fatalf("linha 0 devia ter perdido o serial implausível: %+v", rows[0])
	}
	if rows[0]["model"] != "D401" {
		t.Fatalf("linha 0 não devia perder outros campos: %+v", rows[0])
	}
	if rows[1]["serial"] != "ITBSCF8F197A" {
		t.Fatalf("linha 1 (serial válido) não devia ser tocada: %+v", rows[1])
	}
}

// TestMergeSerialSearchMatch cobre o bug real reportado: uma ONU encontrada por pesquisa telnet
// ao vivo (searchOLTOnuBySerial) mas ausente do snapshot SNMP não entrava no conjunto de
// "seriais conhecidos" que valida o vínculo de cliente — "encontrei pela pesquisa, mas não
// consigo vincular". MergeSerialSearchMatch deve fazer essa ONU aparecer em vsol_onu_rows.
func TestMergeSerialSearchMatch_NewRow(t *testing.T) {
	summary := map[string]any{
		"vsol_onu_rows": []any{
			map[string]any{"pon": 1, "onu": 1, "serial": "ITBSCF8F197A"},
		},
	}
	changed := MergeSerialSearchMatch(summary, 3, 12, "ZTEGD2557B46", "F670L")
	if !changed {
		t.Fatal("esperava changed=true ao acrescentar linha nova")
	}
	rows := OnuRowsFromSummary(summary)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	found := false
	for _, r := range rows {
		if r["serial"] == "ZTEGD2557B46" {
			found = true
			if r["pon"] != 3 || r["onu"] != 12 {
				t.Fatalf("linha nova com pon/onu errados: %+v", r)
			}
			if r["model"] != "F670L" {
				t.Fatalf("linha nova devia ter o modelo: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("serial da pesquisa telnet não apareceu em vsol_onu_rows: %+v", rows)
	}
}

func TestMergeSerialSearchMatch_UpdatesExistingRow(t *testing.T) {
	summary := map[string]any{
		"vsol_onu_rows": []any{
			map[string]any{"pon": 1, "onu": 1, "model": "HG8010H"}, // sem serial (walk incompleto)
		},
	}
	if !MergeSerialSearchMatch(summary, 1, 1, "itbscf8f197a", "") {
		t.Fatal("esperava changed=true ao preencher serial em falta")
	}
	rows := OnuRowsFromSummary(summary)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (deve actualizar, não duplicar)", len(rows))
	}
	if rows[0]["serial"] != "ITBSCF8F197A" {
		t.Fatalf("serial devia ter sido preenchido em maiúsculas: %+v", rows[0])
	}
	if rows[0]["model"] != "HG8010H" {
		t.Fatalf("não devia perder o modelo já existente: %+v", rows[0])
	}
}

func TestMergeSerialSearchMatch_RejectsImplausibleOrIncomplete(t *testing.T) {
	summary := map[string]any{"vsol_onu_rows": []any{}}
	cases := []struct {
		pon, onu int
		serial   string
		name     string
	}{
		{0, 1, "ITBSCF8F197A", "pon zero"},
		{1, 0, "ITBSCF8F197A", "onu zero"},
		{1, 1, "3", "serial implausível"},
		{1, 1, "", "serial vazio"},
	}
	for _, c := range cases {
		if MergeSerialSearchMatch(summary, c.pon, c.onu, c.serial, "") {
			t.Fatalf("%s: esperava changed=false, mas mudou algo", c.name)
		}
	}
	if len(OnuRowsFromSummary(summary)) != 0 {
		t.Fatal("nenhum dos casos inválidos devia ter acrescentado linha")
	}
}

func TestSanitizeOnuSerialsMap_NoOnuRows(t *testing.T) {
	summary := map[string]any{"olt_profile": map[string]any{"brand": "VSOL"}}
	if n := SanitizeOnuSerialsMap(summary); n != 0 {
		t.Fatalf("cleaned = %d, want 0 (sem vsol_onu_rows, nada a limpar)", n)
	}
	if n := SanitizeOnuSerialsMap(nil); n != 0 {
		t.Fatalf("cleaned(nil) = %d, want 0", n)
	}
}

func TestSanitizeOnuSerialsJSON(t *testing.T) {
	raw := []byte(`{"vsol_onu_rows":[{"pon":1,"onu":1,"serial":"3"},{"pon":1,"onu":2,"serial":"ZTEGD2557B46"}]}`)
	out := SanitizeOnuSerialsJSON(raw)
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	rows := OnuRowsFromSummary(decoded)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if _, ok := rows[0]["serial"]; ok {
		t.Fatalf("serial implausível devia ter sido removido: %+v", rows[0])
	}
	if rows[1]["serial"] != "ZTEGD2557B46" {
		t.Fatalf("serial válido não devia ser tocado: %+v", rows[1])
	}

	// Sem nada para limpar, devolve exactamente os mesmos bytes (sem round-trip desnecessário).
	cleanRaw := []byte(`{"vsol_onu_rows":[{"pon":1,"onu":1,"serial":"ZTEGD2557B46"}]}`)
	if out := SanitizeOnuSerialsJSON(cleanRaw); string(out) != string(cleanRaw) {
		t.Fatalf("esperava bytes inalterados quando nada precisa de limpeza, veio: %s", out)
	}

	// JSON vazio/inválido: devolve o input tal como veio, nunca perde dados silenciosamente.
	if out := SanitizeOnuSerialsJSON(nil); out != nil {
		t.Fatalf("SanitizeOnuSerialsJSON(nil) = %v, want nil", out)
	}
	invalid := []byte(`not json`)
	if out := SanitizeOnuSerialsJSON(invalid); string(out) != string(invalid) {
		t.Fatalf("JSON inválido devia voltar inalterado, veio: %s", out)
	}
}
