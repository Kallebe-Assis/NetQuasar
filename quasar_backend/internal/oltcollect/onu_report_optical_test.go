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

func TestParseVsolOpticalInfoFields_onuParenLabel(t *testing.T) {
	out := `Alarm                      : enable
Piggyback DBA rpt mode     : not support
Rx optical level(ONU)      : -22.93
Lower rx optical threshold : -36.00
Upper rx optical threshold : -0.00
Tx optical level           : 2.38
Lower tx optical threshold : 70.00
Upper tx optical threshold : 90.00
ONU response time          : 88
Power feed voltage         : 3.30(V)
Laser bias current         : 8.976(mA)
Temperature                : 52.199(C)

gpon-olt(config-pon-0/1)#`
	fields := ParseTelnetReportSteps([]struct {
		Command string
		Output  string
	}{{Command: "show onu 3 optical_info", Output: out}})
	if fields["RX"] != "-22.93" {
		t.Fatalf("RX=%q fields=%v", fields["RX"], fields)
	}
	if fields["TX"] != "2.38" {
		t.Fatalf("TX=%q", fields["TX"])
	}
	if fields["Voltagem"] != "3.30" {
		t.Fatalf("Voltagem=%q", fields["Voltagem"])
	}
	if fields["Temperatura"] != "52.199" {
		t.Fatalf("Temperatura=%q", fields["Temperatura"])
	}
	if fields["Bias"] != "8.976" {
		t.Fatalf("Bias=%q", fields["Bias"])
	}
}

func TestParseVsolPonOnuRxPowerTable(t *testing.T) {
	out := `Onu         ONU_Rx
------------------------------------
2
-18.12`
	fields := ParseTelnetReportSteps([]struct {
		Command string
		Output  string
	}{{Command: "show pon onu 2 rx-power", Output: out}})
	if fields["RX"] != "-18.12" {
		t.Fatalf("RX=%q fields=%v", fields["RX"], fields)
	}
	row := map[string]any{}
	mergeTelnetFieldsIntoOnuRow(row, fields, "2026-01-01T00:00:00Z")
	if row["rx_dbm"].(float64) != -18.12 {
		t.Fatalf("rx_dbm=%v", row["rx_dbm"])
	}
}
