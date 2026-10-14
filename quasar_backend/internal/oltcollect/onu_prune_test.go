package oltcollect

import "testing"

func TestPruneOnuRowsWithoutSerial(t *testing.T) {
	rows := []map[string]any{
		{"pon": 1, "onu": 1, "serial": "ZTEG12345678"},
		{"pon": 1, "onu": 2},                  // herda do anterior
		{"pon": 1, "onu": 3},                  // fantasma
		{"pon": 1, "onu": 4, "serial": "abc"}, // serial implausível
	}
	prev := []map[string]any{{"pon": 1, "onu": 2, "serial": "hwtcb4ddf7e3", "model": "UN1800X"}}
	kept, dropped := pruneOnuRowsWithoutSerial(rows, prev, true)
	if len(kept) != 2 || len(dropped) != 2 {
		t.Fatalf("kept=%d dropped=%v", len(kept), dropped)
	}
	if kept[1]["serial"] != "HWTCB4DDF7E3" || kept[1]["model"] != "UN1800X" {
		t.Errorf("serial deveria vir do snapshot anterior: %+v", kept[1])
	}
	// leitura do serial não confiável: nada é descartado
	kept2, dropped2 := pruneOnuRowsWithoutSerial([]map[string]any{{"pon": 1, "onu": 9}}, nil, false)
	if len(kept2) != 1 || len(dropped2) != 0 {
		t.Errorf("walk não confiável não pode apagar ONU: %v %v", kept2, dropped2)
	}
}

func TestSerialWalkReliable(t *testing.T) {
	if !serialWalkReliable([]map[string]any{{"metric": MetricSerial, "matched_rows": 120}}) {
		t.Error("walk com linhas e sem corte é confiável")
	}
	if serialWalkReliable([]map[string]any{{"metric": MetricSerial, "matched_rows": 120, "truncated": true}}) {
		t.Error("walk cortado não é confiável")
	}
	if serialWalkReliable([]map[string]any{{"metric": MetricSerial, "matched_rows": 0}}) {
		t.Error("walk vazio não é confiável")
	}
}

func TestOfflineOnuPerdeLeiturasInclusiveTelnet(t *testing.T) {
	rows := []map[string]any{{
		"pon": 1, "onu": 1, "online": false, "serial": "ZTEG12345678", "rx_dbm": -20.1, "rx_pwr": "-20.10", "tx_pwr": "2.4", "temp": "40",
		"telnet_fields": map[string]any{"RX": "-17.7 dBm", "TX": "2.2 dBm", "Distância": "1969m", "Tempo online": "16h", "Temperatura": "41", "SN": "ZTEG12345678", "Nome": "C9-03", "Modelo": "GU201-G"},
	}, {
		"pon": 1, "onu": 2, "online": true, "rx_dbm": -22.0, "telnet_fields": map[string]any{"RX": "-22 dBm", "SN": "X"},
	}}
	stripOfflineOnuTelemetry(rows)
	off := rows[0]
	for _, k := range []string{"rx_dbm", "rx_pwr", "tx_pwr", "temp"} {
		if _, ok := off[k]; ok {
			t.Errorf("%s deveria sumir da ONU offline", k)
		}
	}
	tf := off["telnet_fields"].(map[string]any)
	if len(tf) != 3 || tf["SN"] == nil || tf["Nome"] == nil || tf["Modelo"] == nil {
		t.Errorf("só a identidade deve ficar: %+v", tf)
	}
	if _, ok := rows[1]["rx_dbm"]; !ok || rows[1]["telnet_fields"].(map[string]any)["RX"] == nil {
		t.Error("ONU online não pode perder leitura")
	}
}
