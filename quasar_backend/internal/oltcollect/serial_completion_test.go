package oltcollect

import (
	"context"
	"testing"
	"time"
)

func rowPtr(m map[string]any) *map[string]any { return &m }

// Reproduz o cenário relatado: 8 PONs, as 3 primeiras já com serial, as 5 últimas sem — a fase de
// completar serial tem de mirar justamente nas PONs que faltam, as mais "furadas" primeiro.
func TestMissingSerialSegments_targetsLastPons(t *testing.T) {
	byKey := map[string]*map[string]any{}
	add := func(pon, onu int, serial string) {
		row := map[string]any{"pon": pon, "onu": onu}
		if serial != "" {
			row["serial"] = serial
		}
		byKey[keyPonOnu(pon, onu)] = rowPtr(row)
	}
	for pon := 1; pon <= 3; pon++ {
		for onu := 1; onu <= 5; onu++ {
			add(pon, onu, "VSOL12345678")
		}
	}
	for pon := 4; pon <= 8; pon++ {
		n := 5
		if pon == 6 {
			n = 9 // PON 6 é a mais incompleta
		}
		for onu := 1; onu <= n; onu++ {
			add(pon, onu, "")
		}
	}
	segs := missingSerialSegments(byKey)
	if len(segs) != 5 {
		t.Fatalf("PONs com falta = %d, want 5 (4..8): %+v", len(segs), segs)
	}
	if segs[0].seg != 6 || segs[0].missing != 9 {
		t.Fatalf("a PON mais incompleta devia vir primeiro: %+v", segs[0])
	}
	if countRowsMissingSerial(byKey) != 4*5+9 {
		t.Fatalf("countRowsMissingSerial = %d", countRowsMissingSerial(byKey))
	}
}

func TestMissingSerialSegments_usesIfIndexWhenPresent(t *testing.T) {
	byKey := map[string]*map[string]any{
		"1.1": rowPtr(map[string]any{"pon": 1, "onu": 1, "if_index": 268500992}),
	}
	segs := missingSerialSegments(byKey)
	if len(segs) != 1 || segs[0].seg != 268500992 {
		t.Fatalf("perfil estilo ZTE devia usar o ifIndex como 1º segmento: %+v", segs)
	}
}

// Tabelas de sufixo único (.ONU, mapeadas via onuPonByOnu) não têm PON no 1º segmento — a
// sub-árvore seria outra coisa, então a fase nem deve tentar.
func TestCompleteMissingSerials_skipsSingleSuffixTables(t *testing.T) {
	byKey := map[string]*map[string]any{"1.1": rowPtr(map[string]any{"pon": 1, "onu": 1})}
	def := OnuMetricDef{OID: "1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5"}
	filled, walks := completeMissingSerials(context.Background(), "127.0.0.1", "public", def, byKey,
		nil, map[int]int{1: 1}, 30*time.Second)
	if filled != 0 || len(walks) != 0 {
		t.Fatalf("devia pular tabela de sufixo único: filled=%d walks=%d", filled, len(walks))
	}
}

func TestSerialCompletionTotalBudget_bounds(t *testing.T) {
	if got := serialCompletionTotalBudget(60 * time.Second); got != 30*time.Second {
		t.Errorf("piso: %v", got)
	}
	if got := serialCompletionTotalBudget(180 * time.Second); got != 60*time.Second {
		t.Errorf("1/3 de 180s: %v", got)
	}
	if got := serialCompletionTotalBudget(900 * time.Second); got != 90*time.Second {
		t.Errorf("teto: %v", got)
	}
}

func keyPonOnu(pon, onu int) string {
	return intToStr(pon) + "." + intToStr(onu)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
