package oltifderive

import "testing"

// TestDedupeOltInterfaceTablePonRows reproduz o bug real relatado pelo usuário: uma OLT VSOL
// V1600G1 com 8 portas PON expunha 16 linhas na tabela genérica de interfaces — cada porta física
// aparecia duas vezes no ifTable SNMP, uma como "GPON0/N" e outra com o nome sintético "PON N".
func TestDedupeOltInterfaceTablePonRows(t *testing.T) {
	tab := []map[string]any{
		{"if_index": 1, "display_name": "GPON0/1", "descr": "", "in_octets": 1000, "out_octets": 2000, "oper_status": "up", "oper_status_n": 1},
		{"if_index": 101, "display_name": "PON 01", "descr": "", "in_octets": 0, "out_octets": 0, "oper_status": "down", "oper_status_n": 0},
		{"if_index": 2, "display_name": "GPON0/2", "descr": "", "in_octets": 0, "out_octets": 0, "oper_status": "down", "oper_status_n": 0},
		{"if_index": 102, "display_name": "PON 02", "descr": "", "in_octets": 500, "out_octets": 700, "oper_status": "up", "oper_status_n": 1},
		{"if_index": 50, "display_name": "GE0/1", "descr": "", "in_octets": 9999, "oper_status": "up", "oper_status_n": 1},
	}
	out := DedupeOltInterfaceTablePonRows(tab)

	if len(out) != 3 {
		t.Fatalf("esperava 3 linhas (2 PON deduplicadas + 1 GE), veio %d: %+v", len(out), out)
	}

	byName := map[string]map[string]any{}
	for _, row := range out {
		byName[row["display_name"].(string)] = row
	}

	g1, ok := byName["GPON0/1"]
	if !ok {
		t.Fatalf("GPON0/1 deveria sobreviver como nome de exibição (preferência GPON): %+v", out)
	}
	if g1["in_octets"] != 1000 || g1["out_octets"] != 2000 {
		t.Errorf("GPON0/1 deveria manter seus próprios contadores: %+v", g1)
	}

	g2, ok := byName["GPON0/2"]
	if !ok {
		t.Fatalf("GPON0/2 deveria sobreviver como nome de exibição, puxando dados da linha PON 02: %+v", out)
	}
	if g2["in_octets"] != 500 || g2["out_octets"] != 700 {
		t.Errorf("GPON0/2 deveria herdar os contadores reais da linha duplicada PON 02 (a sua própria estava zerada): %+v", g2)
	}
	if g2["oper_status_n"] != 1 {
		t.Errorf("GPON0/2 deveria herdar oper_status da linha PON 02 (a sua própria estava down/0): %+v", g2)
	}

	if _, stillThere := byName["PON 01"]; stillThere {
		t.Errorf("PON 01 não devia sobreviver como linha separada: %+v", out)
	}
	if _, stillThere := byName["PON 02"]; stillThere {
		t.Errorf("PON 02 não devia sobreviver como linha separada: %+v", out)
	}
	if _, ok := byName["GE0/1"]; !ok {
		t.Errorf("GE0/1 (não-PON) deveria passar intacta: %+v", out)
	}
}

// TestDedupeOltInterfaceTablePonRows_noDuplicatesUnaffected confere que uma tabela sem duplicação
// (o caso normal, maioria das OLTs) não perde nem funde nada.
func TestDedupeOltInterfaceTablePonRows_noDuplicatesUnaffected(t *testing.T) {
	tab := []map[string]any{
		{"if_index": 1, "display_name": "GPON0/1", "descr": ""},
		{"if_index": 2, "display_name": "GPON0/2", "descr": ""},
		{"if_index": 50, "display_name": "GE0/1", "descr": ""},
		{"if_index": 60, "display_name": "VLAN100", "descr": ""},
	}
	out := DedupeOltInterfaceTablePonRows(tab)
	if len(out) != 4 {
		t.Fatalf("esperava 4 linhas (nenhuma duplicada), veio %d: %+v", len(out), out)
	}
}

// TestDedupeOltInterfaceTablePonRows_zteUnaffected confere que o formato ZTE (com barras, já
// reconhecido por PonCompactFromPhy) não colide por engano com o padrão sintético VSOL "PON N".
func TestDedupeOltInterfaceTablePonRows_zteUnaffected(t *testing.T) {
	tab := []map[string]any{
		{"if_index": 1, "display_name": "PON-1/1/1", "descr": ""},
		{"if_index": 2, "display_name": "PON-1/1/2", "descr": ""},
	}
	out := DedupeOltInterfaceTablePonRows(tab)
	if len(out) != 2 {
		t.Fatalf("portas ZTE distintas não deviam ser fundidas entre si: %d: %+v", len(out), out)
	}
}

// TestDedupeOltInterfaceTablePonRows_noSlashVariant reproduz o caso real relatado: a OLT nomeia
// cada porta PON física duas vezes, uma vez "GPON0/N" e outra "GPONNNN" (sem barra, 3 dígitos) —
// antes da correção de PonCompactFromPhy, essas duas linhas tinham chaves diferentes e sobreviviam
// como linhas separadas (8 PONs físicos viravam 16 linhas na tabela de interfaces).
func TestDedupeOltInterfaceTablePonRows_noSlashVariant(t *testing.T) {
	tab := []map[string]any{
		{"if_index": 1, "display_name": "GPON0/1", "descr": "", "in_octets": 1000},
		{"if_index": 101, "display_name": "GPON001", "descr": "", "in_octets": 0},
		{"if_index": 2, "display_name": "GPON0/2", "descr": "", "in_octets": 0},
		{"if_index": 102, "display_name": "GPON002", "descr": "", "in_octets": 2000},
	}
	out := DedupeOltInterfaceTablePonRows(tab)
	if len(out) != 2 {
		t.Fatalf("esperava 2 PONs deduplicados (GPON0/1+GPON001, GPON0/2+GPON002), veio %d: %+v", len(out), out)
	}
	for _, row := range out {
		name := row["display_name"].(string)
		if name != "GPON0/1" && name != "GPON0/2" {
			t.Errorf("nome de exibição devia preferir a forma com barra: %q", name)
		}
	}
}
