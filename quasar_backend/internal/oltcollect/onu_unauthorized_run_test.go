package oltcollect

import (
	"testing"
)

func TestUnauthorizedUsesPonPlaceholder(t *testing.T) {
	cfg := OnuReportConfig{
		UnauthorizedOnuPreCommands: []string{"enable", "interface gpon 0/{pon}"},
		UnauthorizedOnuQueryCommand: "show onu auto-find",
	}
	if !cfg.UnauthorizedUsesPonPlaceholder() {
		t.Fatal("expected pon placeholder")
	}
	cfg2 := OnuReportConfig{
		UnauthorizedOnuPreCommands:    []string{"terminal length 0"},
		UnauthorizedOnuQueryCommand: "show gpon onu uncfg",
	}
	if cfg2.UnauthorizedUsesPonPlaceholder() {
		t.Fatal("unexpected pon placeholder")
	}
}

func TestSplitUnauthorizedPreCommands(t *testing.T) {
	global, perPon := splitUnauthorizedPreCommands([]string{
		"enable", "{enable}", "configure terminal", "interface gpon 0/{pon}",
	})
	if len(global) != 3 || len(perPon) != 1 {
		t.Fatalf("global=%v perPon=%v", global, perPon)
	}
	if perPon[0] != "interface gpon 0/{pon}" {
		t.Fatalf("perPon=%v", perPon)
	}
	global2, perPon2 := splitUnauthorizedPreCommands([]string{
		"enable", "configure terminal", "interface gpon 0/4",
	})
	if len(global2) != 2 || len(perPon2) != 1 {
		t.Fatalf("hardcoded global=%v perPon=%v", global2, perPon2)
	}
}

func TestRenderUnauthorizedPreCommandsVsol(t *testing.T) {
	cfg := OnuReportConfig{
		UnauthorizedOnuPreCommands: []string{
			"enable", "{enable}", "configure terminal", "interface gpon 0/{pon}",
		},
	}
	sec := TelnetSecrets{Enable: "secret"}
	global, perPon := splitUnauthorizedPreCommands(cfg.UnauthorizedOnuPreCommands)
	globalRendered := renderTelnetPreTemplates(global, OnuReportTarget{}, sec)
	perRendered := renderTelnetPreTemplates(perPon, OnuReportTarget{Pon: 4}, sec)
	if len(globalRendered) != 3 || globalRendered[1] != "secret" {
		t.Fatalf("global=%v", globalRendered)
	}
	if len(perRendered) != 1 || perRendered[0] != "interface gpon 0/4" {
		t.Fatalf("per=%v", perRendered)
	}
}

func TestNormalizeVsolGponInterfaceCmd(t *testing.T) {
	if got := normalizeVsolGponInterfaceCmd("interface gpon 4", 4); got != "interface gpon 0/4" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeVsolGponInterfaceCmd("interface gpon 0/4", 4); got != "interface gpon 0/4" {
		t.Fatalf("got %q", got)
	}
}

func TestUnauthorizedNeedsPonIteration_autoFind(t *testing.T) {
	cfg := OnuReportConfig{
		UnauthorizedOnuPreCommands:    []string{"enable", "{enable}", "configure terminal"},
		UnauthorizedOnuQueryCommand:   "show onu auto-find",
	}
	if !unauthorizedNeedsPonIteration(cfg) {
		t.Fatal("expected iteration for show onu auto-find")
	}
	global, perPon := splitUnauthorizedPreCommands(cfg.UnauthorizedOnuPreCommands)
	if len(global) != 3 || len(perPon) != 0 {
		t.Fatalf("global=%v perPon=%v", global, perPon)
	}
}
