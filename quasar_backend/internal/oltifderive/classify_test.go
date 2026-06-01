package oltifderive

import "testing"

func TestClassifyKind(t *testing.T) {
	cases := []struct{ disp, descr string; want Kind }{
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
