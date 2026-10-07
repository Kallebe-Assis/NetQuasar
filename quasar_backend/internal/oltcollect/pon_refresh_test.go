package oltcollect

import (
	"testing"
	"time"
)

func TestPonRefreshCommandList(t *testing.T) {
	c := OnuReportConfig{PonRefreshCommand: "show gpon onu baseinfo gpon-olt_1/1/{pon}; show gpon onu state gpon-olt_1/1/{pon}\n\n  "}
	got := c.RenderPonRefreshCommands(9, TelnetSecrets{})
	if len(got) != 2 || got[0] != "show gpon onu baseinfo gpon-olt_1/1/9" || got[1] != "show gpon onu state gpon-olt_1/1/9" {
		t.Fatalf("comandos renderizados: %v", got)
	}
	if !c.HasPonRefresh() || (OnuReportConfig{}).HasPonRefresh() {
		t.Error("HasPonRefresh")
	}
	if cfg := ParseOnuReportConfig([]byte(`{"pon_refresh_command":"  show onu info {pon}  "}`)); cfg.PonRefreshCommand != "show onu info {pon}" {
		t.Errorf("ParseOnuReportConfig não preservou o campo: %q", cfg.PonRefreshCommand)
	}
}

func TestParsePonRefreshOutputZTE(t *testing.T) {
	base := `OnuIndex                 Type                Mode    AuthInfo
----------------------------------------------------------------
gpon-onu_1/1/9:1         ZTE-F670            sn      SN:ZTEGC1A2B3C4
gpon-onu_1/1/9:2         F601                sn      SN:ITBSCF8F197E
gpon-onu_1/1/9:3         ZTE-F660            sn      SN:ZTEGC9999999
`
	state := `OnuIndex     Admin State  OMCC State  Phase State  Channel
1/1/9:1      enable       enable      working      1(GPON)
1/1/9:2      enable       enable      LOS          1(GPON)
1/1/9:3      enable       enable      DyingGasp    1(GPON)
1/1/9:4      enable       enable      OffLine      1(GPON)
`
	entries := mergePonRefreshEntries(append(ParsePonRefreshOutput(base), ParsePonRefreshOutput(state)...), 9)
	if len(entries) != 4 {
		t.Fatalf("esperava 4 ONUs, veio %d: %+v", len(entries), entries)
	}
	e1 := entries[0]
	if e1.Onu != 1 || e1.Serial != "ZTEGC1A2B3C4" || e1.Model != "ZTE-F670" || e1.Online == nil || !*e1.Online {
		t.Errorf("ONU 1: %+v", e1)
	}
	if e2 := entries[1]; e2.Serial != "ITBSCF8F197E" || e2.Online == nil || *e2.Online || e2.State != "LOS" {
		t.Errorf("ONU 2: %+v", e2)
	}
	if e4 := entries[3]; e4.Serial != "" || e4.Online == nil || *e4.Online {
		t.Errorf("ONU 4 (só estado): %+v", e4)
	}
	// ONU de outra PON é descartada.
	if got := mergePonRefreshEntries(ParsePonRefreshOutput(base), 3); len(got) != 0 {
		t.Errorf("PON errada devia filtrar tudo: %+v", got)
	}
}

func TestParsePonRefreshOutputVSOL(t *testing.T) {
	out := `OnuIndex   Model      Profile    Mode  SN
---------------------------------------------
GPON0/3:1  OT-2200-GP PROFILE-1  sn    ZTEGDA1CCA51
GPON0/3:2  F601       PROFILE-1  sn    ITBSCF8F197A
`
	entries := mergePonRefreshEntries(ParsePonRefreshOutput(out), 3)
	if len(entries) != 2 || entries[0].Serial != "ZTEGDA1CCA51" || entries[1].Onu != 2 {
		t.Fatalf("VSOL: %+v", entries)
	}
}

func TestMergePonRefreshIntoSummary(t *testing.T) {
	summary := map[string]any{
		"vsol_onu_count": 4, "vsol_onu_online": 3, "vsol_onu_offline": 1,
		"vsol_onu_rows": []any{
			map[string]any{"pon": 9, "onu": 1, "online": true, "serial": "ZTEGCOLD0001"},
			map[string]any{"pon": 9, "onu": 2, "online": true},
			map[string]any{"pon": 9, "onu": 3, "online": false},
			map[string]any{"pon": 1, "onu": 1, "online": true, "serial": "OUTRAPON0001"},
		},
	}
	on, off := true, false
	st := MergePonRefreshIntoSummary(summary, 9, []PonRefreshEntry{
		{Pon: 9, Onu: 1, Serial: "ZTEGC1A2B3C4", Online: &on},   // serial trocado
		{Pon: 9, Onu: 2, Serial: "ITBSCF8F197E", Online: &off},  // serial novo + caiu
		{Pon: 9, Onu: 3, Online: &on},                           // só estado
		{Pon: 9, Onu: 4, Serial: "ZTEGC9999999", Online: &on},   // ONU nova
		{Pon: 9, Onu: 5, Serial: "curto", Model: "F601"},        // serial implausível não vale
	}, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))

	if st.Added != 2 || st.SerialsFilled != 3 || st.StateChanged != 3 {
		t.Errorf("estatísticas: %+v", st)
	}
	rows := OnuRowsFromSummary(summary)
	by := map[string]map[string]any{}
	for _, r := range rows {
		by[onuRowKey(r)] = r
	}
	if by["9.1"]["serial"] != "ZTEGC1A2B3C4" || by["9.2"]["online"] != false || by["9.3"]["online"] != true {
		t.Errorf("linhas da PON 9: %v %v %v", by["9.1"], by["9.2"], by["9.3"])
	}
	if _, has := by["9.5"]["serial"]; has {
		t.Errorf("serial implausível não pode ser gravado: %v", by["9.5"])
	}
	if by["1.1"]["serial"] != "OUTRAPON0001" || by["1.1"]["pon_refresh_at"] != nil {
		t.Errorf("outra PON não pode ser tocada: %v", by["1.1"])
	}
	// PON 9: 5 linhas; online = 1, 3 e 4. A ONU 5 não tem estado → conta como offline; a OLT inteira tem a PON 1 online.
	if st.Total != 5 || st.Online != 3 || st.Offline != 2 {
		t.Errorf("totais da PON: %+v", st)
	}
	if _, has := summary["pon_refresh_at"].(map[string]any)["9"]; !has {
		t.Error("faltou o carimbo pon_refresh_at")
	}
	// ONU 5 sem estado → o total da OLT não é recalculado (não dá para afirmar o estado de todas).
	if summary["vsol_onu_count"] != 4 {
		t.Errorf("total da OLT não devia mudar quando há ONU sem estado: %v", summary["vsol_onu_count"])
	}

	pons := []map[string]any{{"pon": 9, "onu_total": 1}, {"pon": 1, "onu_total": 7}}
	ApplyPonRefreshToPons(pons, 9, st)
	if pons[0]["onu_total"] != 5 || pons[0]["onu_online"] != 3 || pons[1]["onu_total"] != 7 {
		t.Errorf("contadores da PON: %v", pons)
	}
}

func TestPonRefreshOutputNoMatchErrorMessage(t *testing.T) {
	if got := ParsePonRefreshOutput("% Invalid input detected at '^' marker.\n"); len(got) != 0 {
		t.Errorf("saída de erro não pode virar ONU: %+v", got)
	}
}
