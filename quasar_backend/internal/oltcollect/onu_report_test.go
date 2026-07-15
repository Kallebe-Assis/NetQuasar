package oltcollect

import (
	"strings"
	"testing"
)

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

func TestResolveGponOnuIgnoresGponOltUncfg(t *testing.T) {
	got := ResolveGponOnu(OnuReportTarget{
		Pon: 9, Onu: 127,
		IfName:  "gpon_olt-1/1/9",
		GponOnu: "gpon_olt-1/1/9",
	})
	if got != "gpon_onu-1/1/9:127" {
		t.Fatalf("got %q want gpon_onu-1/1/9:127", got)
	}
}

func TestSubstituteAuthorizeUsesGponOnuNotOlt(t *testing.T) {
	tpl := "interface {gpon_onu}\npon-onu-mng {gpon_onu}"
	got := SubstituteOnuReportTemplate(tpl, OnuReportTarget{
		Pon: 9, Onu: 127, IfName: "gpon_olt-1/1/9",
	})
	want := "interface gpon_onu-1/1/9:127\npon-onu-mng gpon_onu-1/1/9:127"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
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

func TestSplitTelnetCommands(t *testing.T) {
	in := "interface gpon_olt-1/1/9; no onu 80; exit; exit"
	out := splitTelnetCommands(in)
	if len(out) != 4 {
		t.Fatalf("len=%d out=%v", len(out), out)
	}
	if out[0] != "interface gpon_olt-1/1/9" || out[1] != "no onu 80" {
		t.Fatalf("out=%v", out)
	}
}

func TestSplitTelnetCommands_mashedZteAuthorize(t *testing.T) {
	in := "interface gpon_olt-1/1/9 onu 80 type GU201-G sn HWTC36D0543 exit " +
		"interface gpon_onu-1/1/9:80 name CLIENTE sn-bind enable sn tcont 1 profile 1G " +
		"gemport 1 name 1G tcont 1 exit interface vport-1/1/9.80:1 " +
		"service-port 1 user-vlan 74 vlan 74 exit " +
		"pon-onu-mng gpon_onu-1/1/9:80 " +
		"vlan port eth_0/1 mode tag vlan 74 vlan port eth_0/2 mode tag vlan 74 " +
		"vlan port eth_0/3 mode tag vlan 74 vlan port eth_0/4 mode tag vlan 74 " +
		"service 1 gemport 1 vlan 74 exit"
	out := splitTelnetCommands(in)
	want := []string{
		"interface gpon_olt-1/1/9",
		"onu 80 type GU201-G sn HWTC36D0543",
		"exit",
		"interface gpon_onu-1/1/9:80",
		"name CLIENTE",
		"sn-bind enable sn",
		"tcont 1 profile 1G",
		"gemport 1 name 1G tcont 1",
		"exit",
		"interface vport-1/1/9.80:1",
		"service-port 1 user-vlan 74 vlan 74",
		"exit",
		"pon-onu-mng gpon_onu-1/1/9:80",
		"vlan port eth_0/1 mode tag vlan 74",
		"vlan port eth_0/2 mode tag vlan 74",
		"vlan port eth_0/3 mode tag vlan 74",
		"vlan port eth_0/4 mode tag vlan 74",
		"service 1 gemport 1 vlan 74",
		"exit",
	}
	if len(out) != len(want) {
		t.Fatalf("len=%d want=%d\nout=%v", len(out), len(want), out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("i=%d got=%q want=%q\nfull=%v", i, out[i], want[i], out)
		}
	}
}

func TestSplitTelnetCommands_mashedWithPlaceholders(t *testing.T) {
	in := "interface gpon_olt-1/1/{pon} onu {onu} type GU201-G sn {serial} exit interface {gpon_onu} name CLIENTE exit"
	out := splitTelnetCommands(in)
	if len(out) < 5 {
		t.Fatalf("out=%v", out)
	}
	if out[0] != "interface gpon_olt-1/1/{pon}" || out[1] != "onu {onu} type GU201-G sn {serial}" {
		t.Fatalf("out=%v", out)
	}
	if out[3] != "interface {gpon_onu}" {
		t.Fatalf("out=%v", out)
	}
}

func TestEnsureConfigTerminalPrefix(t *testing.T) {
	got := ensureConfigTerminalPrefix(
		[]string{"terminal length 0"},
		[]string{"interface gpon_olt-1/1/9", "onu 1 type GU201-G sn ABC"},
	)
	if len(got) < 3 || got[0] != "configure terminal" {
		t.Fatalf("got=%v", got)
	}
	got2 := ensureConfigTerminalPrefix(
		[]string{"configure terminal"},
		[]string{"interface gpon_olt-1/1/9"},
	)
	if got2[0] != "interface gpon_olt-1/1/9" {
		t.Fatalf("should not duplicate: %v", got2)
	}
}

func TestSubstituteAuthorizePlaceholders(t *testing.T) {
	tpl := "onu {onu} type {onu_type} sn {serial}; vlan {vlan}; name {name}"
	got := SubstituteOnuReportTemplate(tpl, OnuReportTarget{
		Onu: 80, Serial: "HWTC36D05", Vlan: "74", OnuType: "GU201-G", Name: "9-80",
	})
	want := "onu 80 type GU201-G sn HWTC36D05; vlan 74; name 9-80"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestApplyAuthorizeTemplateDefaults_nameFromPonOnu(t *testing.T) {
	got := ApplyAuthorizeTemplateDefaults(OnuReportTarget{Pon: 9, Onu: 80, Vlan: "16", OnuType: "HG8010H"}, OnuReportConfig{})
	if got.Name != "9-80" {
		t.Fatalf("name=%q", got.Name)
	}
	if got.OnuType != "GU201-G" {
		t.Fatalf("onu_type always GU201-G, got %q", got.OnuType)
	}
	if got.Vlan != "16" {
		t.Fatalf("vlan should stay from caller, got %q", got.Vlan)
	}
}

func TestApplyAuthorizeTemplateDefaults_doesNotInventVlan(t *testing.T) {
	got := ApplyAuthorizeTemplateDefaults(OnuReportTarget{Pon: 1, Onu: 1}, OnuReportConfig{AuthorizeVlan: "74"})
	if got.Vlan != "" {
		t.Fatalf("expected empty vlan without catalog/SNMP, got %q", got.Vlan)
	}
}

func TestSplitTelnetCommands_zteAuthorizeScript(t *testing.T) {
	tpl := strings.Join([]string{
		"configure terminal",
		"interface gpon_olt-1/1/{pon}",
		"onu {onu} type GU201-G sn {serial}",
		"exit",
		"interface gpon_onu-1/1/{pon}:{onu}",
		"name {name}",
		"sn-bind enable sn",
		"tcont 1 profile 1G",
		"gemport 1 name 1G tcont 1",
		"exit",
		"interface vport-1/1/{pon}.{onu}:1",
		"service-port 1 user-vlan {vlan} vlan {vlan}",
		"exit",
		"pon-onu-mng gpon_onu-1/1/{pon}:{onu}",
		"vlan port eth_0/1 mode tag vlan {vlan}",
		"vlan port eth_0/2 mode tag vlan {vlan}",
		"vlan port eth_0/3 mode tag vlan {vlan}",
		"vlan port eth_0/4 mode tag vlan {vlan}",
		"service 1 gemport 1 vlan {vlan}",
		"exit",
	}, "\n")
	t.Helper()
	rendered := SubstituteOnuReportTemplate(tpl, OnuReportTarget{
		Pon: 9, Onu: 80, Serial: "HWTC36D05643", Vlan: "16", OnuType: "GU201-G", Name: "9-80",
	})
	got := splitTelnetCommands(rendered)
	want := []string{
		"configure terminal",
		"interface gpon_olt-1/1/9",
		"onu 80 type GU201-G sn HWTC36D05643",
		"exit",
		"interface gpon_onu-1/1/9:80",
		"name 9-80",
		"sn-bind enable sn",
		"tcont 1 profile 1G",
		"gemport 1 name 1G tcont 1",
		"exit",
		"interface vport-1/1/9.80:1",
		"service-port 1 user-vlan 16 vlan 16",
		"exit",
		"pon-onu-mng gpon_onu-1/1/9:80",
		"vlan port eth_0/1 mode tag vlan 16",
		"vlan port eth_0/2 mode tag vlan 16",
		"vlan port eth_0/3 mode tag vlan 16",
		"vlan port eth_0/4 mode tag vlan 16",
		"service 1 gemport 1 vlan 16",
		"exit",
	}
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d\ngot=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cmd[%d]=%q want %q", i, got[i], want[i])
		}
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
