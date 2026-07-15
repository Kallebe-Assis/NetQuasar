package oltcollect

import "testing"

func TestParseOnuListFromTelnetOutput_vsol(t *testing.T) {
	out := `Onuindex         Model                Profile                Mode    AuthInfo
--------------------------------------------------------------------------------
GPON0/1:27 R1v2                 PROFILE-1              sn      ITBSCF8F197A
GPON0/1:28 GU201-G              PROFILE-1              sn      XGTC07101752
GPON0/2:5  GU201-G              PROFILE-1              sn      XGTC07101349`
	entries := ParseOnuListFromTelnetOutput(out)
	if len(entries) != 3 {
		t.Fatalf("entries=%d", len(entries))
	}
	if entries[1].Serial != "XGTC07101752" || entries[1].Pon != 1 || entries[1].Onu != 28 {
		t.Fatalf("entry1=%+v", entries[1])
	}
	if entries[2].Pon != 2 || entries[2].Onu != 5 {
		t.Fatalf("entry2=%+v", entries[2])
	}
}

func TestParseOnuListFromTelnetOutput_vsolAutoFind(t *testing.T) {
	out := `OLT-PADUA(config-pon-0/4)# show onu auto-find 

OnuIndex                 Sn                       State
---------------------------------------------------------
GPON0/4:1                ZTEGCFAA2AB1             unknow
GPON0/4:2                ITBSCF8F197A             unknow`
	entries := ParseOnuListFromTelnetOutput(out)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2, got %+v", len(entries), entries)
	}
	if entries[0].Serial != "ZTEGCFAA2AB1" || entries[0].Pon != 4 || entries[0].Onu != 1 {
		t.Fatalf("entry0=%+v", entries[0])
	}
	if entries[0].Mode != "unknow" || entries[0].GponOnu != "GPON0/4:1" {
		t.Fatalf("entry0 mode/gpon=%+v", entries[0])
	}
	if entries[1].Serial != "ITBSCF8F197A" || entries[1].Pon != 4 || entries[1].Onu != 2 {
		t.Fatalf("entry1=%+v", entries[1])
	}
}

func TestParseOnuListFromTelnetOutput_ztePonOnuUncfg(t *testing.T) {
	out := `show pon onu uncfg
OltIndex            Model                SN                 PW
-----------------------------------------------------------------------------
gpon_olt-1/1/9      R1v2                 ITBSCF8F197E       123456789
gpon_olt-1/1/9      HG8010H              HWTC36D05643       N/A
olt-zte-miracema-01#`
	entries := ParseOnuListFromTelnetOutput(out)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2, got %+v", len(entries), entries)
	}
	if entries[0].Pon != 9 || entries[0].Model != "R1v2" || entries[0].Serial != "ITBSCF8F197E" {
		t.Fatalf("entry0=%+v", entries[0])
	}
	if entries[1].Pon != 9 || entries[1].Model != "HG8010H" || entries[1].Serial != "HWTC36D05643" {
		t.Fatalf("entry1=%+v", entries[1])
	}
	if entries[0].GponOnu != "" || entries[1].GponOnu != "" {
		t.Fatalf("uncfg must not set gpon_olt as gpon_onu: %+v / %+v", entries[0], entries[1])
	}
}

func TestFilterSerialSearchEntries(t *testing.T) {
	entries := []SerialSearchOnuEntry{
		{Pon: 1, Onu: 28, Serial: "XGTC07101752"},
		{Pon: 1, Onu: 29, Serial: "XGTC07101349"},
		{Pon: 2, Onu: 5, Serial: "XGTC07101349"},
	}
	all := FilterSerialSearchEntries(entries, "XGTC07101349", 0)
	if len(all) != 2 {
		t.Fatalf("all=%d", len(all))
	}
	pon1 := FilterSerialSearchEntries(entries, "XGTC07101349", 1)
	if len(pon1) != 1 || pon1[0].Onu != 29 {
		t.Fatalf("pon1=%+v", pon1)
	}
	partial := FilterSerialSearchEntries(entries, "101349", 0)
	if len(partial) != 2 {
		t.Fatalf("partial=%d", len(partial))
	}
	colon := FilterSerialSearchEntries([]SerialSearchOnuEntry{
		{Pon: 1, Onu: 1, Serial: "ITBS:CF8F:197A"},
	}, "cf8f197a", 0)
	if len(colon) != 1 {
		t.Fatalf("colon=%+v", colon)
	}
}

func TestSerialSearchModeDetection(t *testing.T) {
	direct := OnuReportConfig{SerialSearchCommand: "show onu sn {serial}"}
	if !direct.SerialSearchUsesSerialPlaceholder() {
		t.Fatal("expected direct mode")
	}
	list := OnuReportConfig{SerialSearchCommand: "show onu info {pon}"}
	if list.SerialSearchUsesSerialPlaceholder() {
		t.Fatal("expected list mode")
	}
	if !list.SerialSearchUsesPonPlaceholder() {
		t.Fatal("expected pon placeholder")
	}
}

func TestParsePonOnuFromVsolGponIndex(t *testing.T) {
	pon, onu := ParsePonOnuFromVsolGponIndex("GPON0/1:27")
	if pon != 1 || onu != 27 {
		t.Fatalf("pon=%d onu=%d", pon, onu)
	}
}
