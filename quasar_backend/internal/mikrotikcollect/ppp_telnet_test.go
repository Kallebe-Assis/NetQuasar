package mikrotikcollect

import "testing"

func TestParseRouterOSUptime(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"5h3m22s", 5*3600 + 3*60 + 22},
		{"9m52s", 9*60 + 52},
		{"3d5h2m", 3*24*3600 + 5*3600 + 2*60},
		{"1w2d", 7*24*3600 + 2*24*3600},
		{"", 0},
		{"garbage", 0},
	}
	for _, c := range cases {
		got, ok := ParseRouterOSUptime(c.raw)
		if c.raw == "" || c.raw == "garbage" {
			if ok {
				t.Errorf("ParseRouterOSUptime(%q) ok=true, want false", c.raw)
			}
			continue
		}
		if !ok {
			t.Fatalf("ParseRouterOSUptime(%q) ok=false, want true", c.raw)
		}
		if got != c.want {
			t.Errorf("ParseRouterOSUptime(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func TestParsePPPActiveDetail(t *testing.T) {
	// Formato real do RouterOS para "/ppp active print detail without-paging" — cada sessão
	// pode ter uma linha ";;; comentário" acima (herdada do /ppp secret correspondente).
	out := `Flags: R - radius
 0   R name="luciafranca" service=pppoe caller-id="18:0D:2C:5E:0A:64" address=192.168.50.200
       uptime=5h3m22s encoding="" session-id=0x8A000000

 1   R name="postodesaude" service=pppoe caller-id="50:D4:F7:15:B4:D9" address=192.168.50.199
       uptime=5h3m21s encoding="" session-id=0x8B000000

 3   ;;; ROSA PARACAMBI
     R name="rosaparacambi" service=pppoe caller-id="48:51:CF:48:B6:31" address=192.168.50.197
       uptime=5h3m10s encoding="" session-id=0x8C000000
`
	entries := ParsePPPActiveDetail(out)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(entries), entries)
	}

	if entries[0].Name != "luciafranca" || entries[0].CallerID != "18:0D:2C:5E:0A:64" ||
		entries[0].Address != "192.168.50.200" || !entries[0].Radius {
		t.Errorf("entry 0 unexpected: %+v", entries[0])
	}
	if entries[0].UptimeSec != 5*3600+3*60+22 {
		t.Errorf("entry 0 uptime sec = %d, want %d", entries[0].UptimeSec, 5*3600+3*60+22)
	}
	if entries[0].Comment != "" {
		t.Errorf("entry 0 comment = %q, want empty", entries[0].Comment)
	}

	if entries[2].Name != "rosaparacambi" {
		t.Errorf("entry 2 name = %q, want rosaparacambi", entries[2].Name)
	}
	if entries[2].Comment != "ROSA PARACAMBI" {
		t.Errorf("entry 2 comment = %q, want %q", entries[2].Comment, "ROSA PARACAMBI")
	}
}

func TestParsePPPSecretDetail(t *testing.T) {
	out := `Flags: X - disabled
 0   name="luciafranca" service=pppoe caller-id="" profile=default-encryption
       local-address="" remote-address="" disabled=no

 3   ;;; ROSA PARACAMBI
     name="rosaparacambi" service=pppoe caller-id="" profile=default-encryption
       local-address="" remote-address="" disabled=no

 9   ;;; cliente cancelado
     X name="exclientex" service=pppoe caller-id="" profile=default-encryption
       local-address="" remote-address="" disabled=yes
`
	entries := ParsePPPSecretDetail(out)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Name != "luciafranca" || entries[0].Comment != "" {
		t.Errorf("entry 0 unexpected: %+v", entries[0])
	}
	if entries[1].Name != "rosaparacambi" || entries[1].Comment != "ROSA PARACAMBI" {
		t.Errorf("entry 1 unexpected: %+v", entries[1])
	}
	if entries[2].Name != "exclientex" || entries[2].Comment != "cliente cancelado" || !entries[2].Disabled {
		t.Errorf("entry 2 unexpected: %+v", entries[2])
	}
}

func TestParseRouterOSRecordsIgnoresLeadingCommentWithoutRecord(t *testing.T) {
	// Um ";;; comentário" no fim da saída sem nenhum registo a seguir não deve gerar entradas
	// nem rebentar o parser.
	out := `Flags: R - radius
;;; comentário órfão
`
	recs := ParseRouterOSRecords(out)
	if len(recs) != 0 {
		t.Fatalf("expected 0 records, got %d: %+v", len(recs), recs)
	}
}
