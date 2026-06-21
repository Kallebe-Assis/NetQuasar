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
