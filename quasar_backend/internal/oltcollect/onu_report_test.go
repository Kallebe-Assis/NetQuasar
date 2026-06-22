package oltcollect

import "testing"

func TestResolveGponOnuFromPonOnu(t *testing.T) {
	got := ResolveGponOnu(OnuReportTarget{Pon: 9, Onu: 80})
	if got != "gpon_onu-1/1/9:80" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveGponOnuFromIfName(t *testing.T) {
	got := ResolveGponOnu(OnuReportTarget{IfName: "gpon_onu-1/1/9:80"})
	if got != "gpon_onu-1/1/9:80" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderSerialSearchCommand(t *testing.T) {
	cfg := OnuReportConfig{SerialSearchCommand: "show gpon onu by sn {serial}"}
	cmd := cfg.RenderSerialSearchCommand(OnuReportTarget{Serial: "ABC123"}, TelnetSecrets{})
	if cmd != "show gpon onu by sn ABC123" {
		t.Fatalf("got %q", cmd)
	}
}

func TestParsePonOnuFromGponOnu(t *testing.T) {
	pon, onu := ParsePonOnuFromGponOnu("gpon_onu-1/1/9:80")
	if pon != 9 || onu != 80 {
		t.Fatalf("pon=%d onu=%d", pon, onu)
	}
}

func TestParseGponOnuFromOutput(t *testing.T) {
	out := `SearchResult
-----------------
gpon_onu-1/1/9:80`
	got := ParseGponOnuFromOutput(out)
	if got != "gpon_onu-1/1/9:80" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderCommandsZTE(t *testing.T) {
	cfg := ParseOnuReportConfig([]byte(`{
		"commands": [
			"show gpon onu detail-info {gpon_onu}",
			"show pon power onu-rx {gpon_onu}"
		]
	}`))
	cmds := cfg.RenderCommands(OnuReportTarget{Pon: 9, Onu: 80}, TelnetSecrets{})
	if len(cmds) != 2 {
		t.Fatalf("len=%d", len(cmds))
	}
	if cmds[0] != "show gpon onu detail-info gpon_onu-1/1/9:80" {
		t.Fatalf("cmd0=%q", cmds[0])
	}
}

func TestRenderCommandsVSOL(t *testing.T) {
	cfg := ParseOnuReportConfig([]byte(`{
		"commands": ["show onu info {pon} {onu}", "show onu state {pon} {onu}"]
	}`))
	cmds := cfg.RenderCommands(OnuReportTarget{Pon: 1, Onu: 1}, TelnetSecrets{})
	if len(cmds) != 2 || cmds[0] != "show onu info 1 1" || cmds[1] != "show onu state 1 1" {
		t.Fatalf("cmds=%v", cmds)
	}
}

func TestRenderPreCommandsSecrets(t *testing.T) {
	cfg := ParseOnuReportConfig([]byte(`{
		"pre_commands": ["enable", "{enable}", "conf terminal"]
	}`))
	pre := cfg.RenderPreCommands(OnuReportTarget{}, TelnetSecrets{Enable: "secret-en"})
	if len(pre) != 3 || pre[1] != "secret-en" {
		t.Fatalf("pre=%v", pre)
	}
}

func TestOnuReportConfig_MonitorEnabled(t *testing.T) {
	cfg := ParseOnuReportConfig([]byte(`{"enabled":true,"commands":["show onu {pon} {onu}"]}`))
	if !cfg.MonitorEnabled() {
		t.Fatal("expected monitor enabled")
	}
	if cfg.EffectiveMaxOnus() != 25 {
		t.Fatalf("max=%d", cfg.EffectiveMaxOnus())
	}
}

func TestSelectRotatingOnuBatch_wraps(t *testing.T) {
	cands := []map[string]any{
		{"pon": 1, "onu": 1}, {"pon": 1, "onu": 2}, {"pon": 1, "onu": 3},
		{"pon": 2, "onu": 1}, {"pon": 2, "onu": 2},
	}
	batch, next := selectRotatingOnuBatch(cands, 3, 4)
	if len(batch) != 3 {
		t.Fatalf("batch len=%d", len(batch))
	}
	if intFromRow(batch[0], "onu") != 2 || intFromRow(batch[1], "pon") != 1 {
		t.Fatalf("batch0=%v batch1=%v", batch[0], batch[1])
	}
	if next != 2 {
		t.Fatalf("next=%d", next)
	}
}

func TestSelectRotatingOnuBatch_allWhenBelowMax(t *testing.T) {
	cands := []map[string]any{{"pon": 1, "onu": 1}, {"pon": 1, "onu": 2}}
	batch, next := selectRotatingOnuBatch(cands, 25, 10)
	if len(batch) != 2 || next != 0 {
		t.Fatalf("batch=%d next=%d", len(batch), next)
	}
}

func TestCarryForwardTelnetFromPrev(t *testing.T) {
	prev := []map[string]any{{
		"pon": 1, "onu": 2, "data_source_telnet": true,
		"rx_pwr": "-22.5", "telnet_report_at": "2026-01-01T00:00:00Z",
	}}
	out := []map[string]any{
		{"pon": 1, "onu": 1, "online": true},
		{"pon": 1, "onu": 2, "online": true},
	}
	carryForwardTelnetFromPrev(out, prev, map[string]bool{"1.1": true})
	if out[1]["rx_pwr"] != "-22.5" {
		t.Fatalf("expected carry forward, got %v", out[1]["rx_pwr"])
	}
	if out[0]["rx_pwr"] != nil {
		t.Fatal("should not carry to refreshed key")
	}
}
