package api

import "testing"

func TestNormalizePermissions(t *testing.T) {
	valid, invalid := normalizePermissions([]string{"devices.view", "devices.view", "nope", " * ", "alerts.manage"})
	if len(invalid) != 1 || invalid[0] != "nope" {
		t.Fatalf("invalid=%v", invalid)
	}
	if len(valid) != 3 {
		t.Fatalf("valid=%v", valid)
	}
	if !permissionGranted(valid, "*") {
		t.Fatal("expected * grant")
	}
	if !permissionGranted([]string{"devices.manage"}, "devices.manage") {
		t.Fatal("expected devices.manage")
	}
	if permissionGranted([]string{"devices.view"}, "devices.manage") {
		t.Fatal("view should not grant manage")
	}
}

func TestSlugifyPermissionProfile(t *testing.T) {
	if got := slugifyPermissionProfile(" Operações OLT "); got != "operaes-olt" && got != "operacoes-olt" {
		// ASCII-only slugify drops accents → "operaes-olt"
		if got != "operaes-olt" {
			t.Fatalf("slug=%q", got)
		}
	}
	if got := slugifyPermissionProfile("!!!@@@"); got != "perfil" {
		t.Fatalf("empty slug fallback=%q", got)
	}
}
