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
