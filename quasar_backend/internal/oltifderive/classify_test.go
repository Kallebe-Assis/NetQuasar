package oltifderive

import "testing"

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		disp, descr string
		want        Kind
	}{
		{"GE0/1", "", KindManagement},
		{"VLAN500", "", KindVLAN},
		{"GPON0/1", "", KindPON},
		{"PON-1/1/1", "", KindPON},
		{"gpon_olt-1/1/1", "", KindPON},
		{"gpon-1/1/1", "", KindPON},
		{"GPON01ONU2 ROGERIO", "", KindONU},
		{"ONU-1/1/1:2", "", KindONU},
		{"gpon-onu_1/1/1:3", "", KindONU},
		{"GPON-ONU-1/1/1:4", "", KindONU},
		{"gpON12ONU3", "", KindONU},
		{"ether1", "", KindOther},
		{"GPON001", "", KindPON},
		{"GPON01", "", KindPON},
	}
	for _, tc := range cases {
		if g := ClassifyKind(tc.disp, tc.descr); g != tc.want {
			t.Fatalf("%q: got %s want %s", tc.disp, g, tc.want)
		}
	}
}

func TestPonCompact(t *testing.T) {
	if PonCompactFromPhy("GPON0/1", "") != "01" {
		t.Fatal(PonCompactFromPhy("GPON0/1", ""))
	}
	if PonCompactFromPhy("PON-1/1/16", "") != "1/1/16" {
		t.Fatal(PonCompactFromPhy("PON-1/1/16", ""))
	}
	if PonCompactFromPhy("gpon_olt-1/1/16", "") != "1/1/16" {
		t.Fatal(PonCompactFromPhy("gpon_olt-1/1/16", ""))
	}
	pc, onu, ok := PonCompactFromOnuIface("GPON01ONU2 x", "")
	if !ok || pc != "01" || onu != 2 {
		t.Fatalf("onu %v %v %v", pc, onu, ok)
	}
	pc, onu, ok = PonCompactFromOnuIface("ONU-1/1/16:5", "")
	if !ok || pc != "1/1/16" || onu != 5 {
		t.Fatalf("zte onu %v %v %v", pc, onu, ok)
	}
	pc, onu, ok = PonCompactFromOnuIface("GPON-ONU-1/1/1:2", "")
	if !ok || pc != "1/1/1" || onu != 2 {
		t.Fatalf("zte onu dash %v %v %v", pc, onu, ok)
	}
	if PonPortFromCompact("1/1/16") != 16 || PonPortFromCompact("01") != 1 {
		t.Fatal("PonPortFromCompact")
	}
	pp, onuN, c, ok := ParseOnuIfLabels("GPON-ONU_1/1/3:7", "")
	if !ok || pp != 3 || onuN != 7 || c != "1/1/3" {
		t.Fatalf("ParseOnuIfLabels %d %d %q ok=%v", pp, onuN, c, ok)
	}
}

// TestPonCompactFromPhy_noSlashVsolVariant reproduz o bug real relatado pelo usuário: uma OLT VSOL
// V1600G1 nomeia a mesma porta física PON 1 como "GPON001" (sem barra, 3 dígitos) em vez de
// "GPON0/1" — antes desta correção, PonCompactFromPhy devolvia "" para esse formato, então a chave
// canónica caía no fallback (ex.: o nome/id cru), nunca convergindo com "GPON0/1"/"PON 01" e
// duplicando cada porta física (8 PONs apareciam como 16 interfaces).
func TestPonCompactFromPhy_noSlashVsolVariant(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GPON001", "01"},
		{"GPON01", "01"},
		{"gpon008", "08"},
		{"GPON010", "010"}, // porta 10 -> mesmo esquema "0"+N que VsolMibPonCompactID já usa
	}
	for _, tc := range cases {
		if got := PonCompactFromPhy(tc.in, ""); got != tc.want {
			t.Errorf("PonCompactFromPhy(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// as 3 grafias da MESMA porta física (GPON0/1, PON 01, GPON001) têm de convergir pra mesma chave
	a := PonCompactFromPhy("GPON0/1", "")
	b := VsolMibPonCompactID(1) // forma que CanonicalPonRowKey usa para o nome sintético "PON 1"
	c := PonCompactFromPhy("GPON001", "")
	if a != b || b != c {
		t.Fatalf("GPON0/1=%q, PON-1(vsol)=%q, GPON001=%q — deviam ser todas iguais", a, b, c)
	}
	// não pode colidir com o formato ONU (dígitos seguidos de "ONU", nunca deve casar aqui)
	if PonCompactFromPhy("GPON01ONU2", "") != "" {
		t.Errorf("GPON01ONU2 não é porta física, não devia casar com rePonPhyNoSlash: %q", PonCompactFromPhy("GPON01ONU2", ""))
	}
}
