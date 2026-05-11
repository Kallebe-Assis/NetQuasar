package zteparse

import "testing"

func TestParseShowGponOnuState(t *testing.T) {
	raw := `
gpon-onu_1/1/1:1   enable   omcc   working
gpon-onu_1/1/1:2   enable   omcc   LOS
gpon-onu_1/1/10:1  enable   omcc   working
ONU-1/1/2:1        up       online
`
	rows := ParseShowGponOnuState(raw)
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0].Pon != "1/1/1" || rows[0].OnuTotal != 2 || rows[0].OnuOnline != 1 || rows[0].OnuOffline != 1 {
		t.Fatalf("pon1 invalid: %+v", rows[0])
	}
	if rows[1].Pon != "1/1/2" || rows[1].OnuOnline != 1 {
		t.Fatalf("pon2 invalid: %+v", rows[1])
	}
	if rows[2].Pon != "1/1/10" {
		t.Fatalf("ordem inválida: %+v", rows)
	}
}

func TestParseShowGponOnuState_ZTERealOutput(t *testing.T) {
	raw := `
olt-zte-miracema-01#show gpon onu state 
OnuIndex     Admin state  OMCC state  Phase state  Speed mode 
---------------------------------------------------------------
1/1/1:1       enable       enable      working      GPON
1/1/1:2       enable       enable      working      GPON
1/1/1:12      enable       disable     DyingGasp    GPON
1/1/1:26      enable       disable     DyingGasp    GPON
1/1/1:32      enable       disable     LOS          GPON
1/1/1:37      enable       enable      working      GPON
`
	rows := ParseShowGponOnuState(raw)
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	r := rows[0]
	if r.Pon != "1/1/1" {
		t.Fatalf("pon inválida: %+v", r)
	}
	if r.OnuTotal != 6 || r.OnuOnline != 3 || r.OnuOffline != 3 {
		t.Fatalf("contagem inválida: %+v", r)
	}
}

