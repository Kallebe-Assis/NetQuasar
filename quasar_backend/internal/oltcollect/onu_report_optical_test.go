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

// TestParseZteRxPowerIgnoresOtherStepsPhantomReadings reproduz um relatório real de ONU
// ZTE (4 passos): o RX/TX correto só está no passo "show pon power onu-rx/tx", mas os
// passos "show gpon onu detail-info" e "show pon onu information" — que nada têm a ver com
// potência óptica — antes eram roteados por engano para o parser de tabela VSOL (por causa
// do "show pon onu" bater na condição do primeiro, e do fallback genérico do segundo), que
// lia o primeiro número negativo/decimal do texto solto como se fosse dBm. Isso incluía o
// "-1" do próprio identificador "gpon_onu-1/1/6:78" e o "-07" do mês de uma data
// "2026-07-29" na tabela de histórico Authpass Time — produzindo RX="-07" sempre, em vez do
// valor real "-19.032" (bug relatado em produção).
func TestParseZteRxPowerIgnoresOtherStepsPhantomReadings(t *testing.T) {
	detailInfo := "ONU interface:          gpon_onu-1/1/6:78\n" +
		"  Name:                 simone\n" +
		"  Type:                 GU201-G\n" +
		"  Admin state:          enable\n" +
		"------------------------------------------\n" +
		"       Authpass Time          OfflineTime             Cause\n" +
		"   1   2026-07-29 04:38:42    2026-08-06 19:50:01     DyingGasp"
	information := "ONU interface:                  gpon_onu-1/1/6:78\n" +
		"SN reported:                    ITBSCF6B9163\n" +
		"ONU ID:                         34\n" +
		"Hardware version:               ONUR1_v2.0\n" +
		"------------------------------------------\n" +
		"       Authpass Time          OfflineTime             Cause\n" +
		"   1   2026-07-29 04:38:42    2026-08-06 19:50:01     DyingGasp"
	rxPower := "Onu                  Rx power\n" +
		"------------------------------------\n" +
		"gpon_onu-1/1/6:78    -19.032(dbm)"
	txPower := "Onu                  Tx power\n" +
		"------------------------------------\n" +
		"gpon_onu-1/1/6:78    2.006(dbm)"

	fields := ParseTelnetReportSteps([]struct {
		Command string
		Output  string
	}{
		{Command: "show gpon onu detail-info gpon_onu-1/1/6:78", Output: detailInfo},
		{Command: "show pon onu information gpon_onu-1/1/6:78", Output: information},
		{Command: "show pon power onu-rx gpon_onu-1/1/6:78", Output: rxPower},
		{Command: "show pon power onu-tx gpon_onu-1/1/6:78", Output: txPower},
	})
	if fields["RX"] != "-19.032" {
		t.Fatalf("RX=%q want -19.032 (fields=%v)", fields["RX"], fields)
	}
	if fields["TX"] != "2.006" {
		t.Fatalf("TX=%q want 2.006 (fields=%v)", fields["TX"], fields)
	}
	// Ainda deve extrair os campos genuínos desses dois passos (isso ficava escondido
	// antes, porque o retorno adiantado do parser VSOL descartava tudo o resto).
	if fields["ONU ID"] != "34" {
		t.Fatalf("ONU ID=%q fields=%v", fields["ONU ID"], fields)
	}
	if fields["Nome"] != "simone" {
		t.Fatalf("Nome=%q fields=%v", fields["Nome"], fields)
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
