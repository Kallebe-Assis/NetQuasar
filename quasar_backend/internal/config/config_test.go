package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendSSLRootCertToDSN(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(cert, []byte("-----BEGIN CERTIFICATE-----\nYQ==\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := "postgres://u:p@localhost:5432/db?sslmode=require"
	out, err := AppendSSLRootCertToDSN(base, cert)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sslrootcert=") {
		t.Fatalf("expected sslrootcert in %q", out)
	}
	if out == base {
		t.Fatal("dsn unchanged")
	}
	twice, err := AppendSSLRootCertToDSN(out, cert)
	if err != nil {
		t.Fatal(err)
	}
	if twice != out {
		t.Fatalf("second append should noop, got %q vs %q", twice, out)
	}
}

func TestEnsureSupabaseSSLRootCertIfNeeded(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "data", "certs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(sub, "supabase-root-ca-2021.pem")
	if err := os.WriteFile(cert, []byte("-----BEGIN CERTIFICATE-----\nYQ==\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(wd) })

	base := "postgres://u:p@db.abc.supabase.co:5432/postgres?sslmode=require"
	got := EnsureSupabaseSSLRootCertIfNeeded(base)
	if got == base {
		t.Fatal("expected sslrootcert appended for supabase host")
	}
	if !strings.Contains(got, "sslrootcert=") {
		t.Fatalf("got %q", got)
	}

	other := "postgres://u:p@localhost:5432/db?sslmode=disable"
	if EnsureSupabaseSSLRootCertIfNeeded(other) != other {
		t.Fatal("non-supabase dsn should be unchanged")
	}
}
