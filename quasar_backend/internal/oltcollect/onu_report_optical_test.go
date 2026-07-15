package oltcollect

import "testing"

func TestParseVsolOpticalInfoFields(t *testing.T) {
	out := `ONU ID: 5
ONU PON Interface:            pon_0/1
Rx optical level:             -23.280(dBm)
Tx optical level:             2.568(dBm)
Power feed voltage:           3.28(V)
Laser bias current:           10.400(mA)
Temperature:                  31.430(C)`
	fields := ParseTelnetReportSteps([]struct {
		Command string
		Output  string
	}{{Command: "show onu 5 optical_info", Output: out}})
	if fields["RX"] != "-23.280" {
		t.Fatalf("RX=%q fields=%v", fields["RX"], fields)
	}
	if fields["TX"] != "2.568" {
		t.Fatalf("TX=%q", fields["TX"])
	}
	if fields["Voltagem"] != "3.28" {
		t.Fatalf("Voltagem=%q", fields["Voltagem"])
	}
	if fields["Temperatura"] != "31.430" {
		t.Fatalf("Temperatura=%q", fields["Temperatura"])
	}
	row := map[string]any{}
	mergeTelnetFieldsIntoOnuRow(row, fields, "2026-01-01T00:00:00Z")
	if row["rx_dbm"].(float64) != -23.28 {
		t.Fatalf("rx_dbm=%v", row["rx_dbm"])
	}
	if row["tx_dbm"].(float64) != 2.568 {
		t.Fatalf("tx_dbm=%v", row["tx_dbm"])
	}
	if row["voltage"] != "3.28" {
		t.Fatalf("voltage=%v", row["voltage"])
	}
}

func TestParseVsolOpticalInfoFields_splitLines(t *testing.T) {
	out := `ONU ID: 9
ONU PON Interface:
pon_0/1
Rx optical level:
-24.202(dBm)
Tx optical level:
1.888(dBm)
Power feed voltage:
3.38(V)
Laser bias current:
15.100(mA)
Temperature:
29.250(C)`
	fields := ParseTelnetReportSteps([]struct {
		Command string
		Output  string
	}{{Command: "show onu 9 optical_info", Output: out}})
	if fields["RX"] != "-24.202" || fields["TX"] != "1.888" {
		t.Fatalf("optical=%v", fields)
	}
	if fields["Voltagem"] != "3.38" || fields["Temperatura"] != "29.250" {
		t.Fatalf("volt/temp=%v", fields)
	}
	if fields["Interface PON"] != "pon_0/1" {
		t.Fatalf("iface=%q", fields["Interface PON"])
	}
}
