package backupb2

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func findTool(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		cands := []string{
			`C:\Program Files\PostgreSQL\18\bin\` + name + `.exe`,
			`C:\Program Files\PostgreSQL\17\bin\` + name + `.exe`,
			`C:\Program Files\PostgreSQL\16\bin\` + name + `.exe`,
			`C:\Program Files\PostgreSQL\18\pgAdmin 4\runtime\` + name + `.exe`,
		}
		for _, c := range cands {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("%s não encontrado no PATH (instale postgresql-client-18)", name)
}

// DumpFull cria um dump custom (schema+dados) em outPath.
func DumpFull(ctx context.Context, databaseURL, outPath string) error {
	bin, err := findTool("pg_dump")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin,
		"--dbname="+databaseURL,
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"--file="+outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// WipePublicSchema remove e recria o schema public.
func WipePublicSchema(ctx context.Context, databaseURL string) error {
	bin, err := findTool("psql")
	if err != nil {
		return err
	}
	sql := `
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO CURRENT_USER;
GRANT ALL ON SCHEMA public TO public;
GRANT USAGE ON SCHEMA public TO public;
`
	cmd := exec.CommandContext(ctx, bin, "--dbname="+databaseURL, "-v", "ON_ERROR_STOP=1", "-c", sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wipe schema: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// RestoreFull restaura dump custom (após WipePublicSchema) e valida tabelas essenciais.
func RestoreFull(ctx context.Context, databaseURL, dumpPath string) error {
	bin, err := findTool("pg_restore")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin,
		"--dbname="+databaseURL,
		"--no-owner",
		"--no-acl",
		dumpPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	errOut := strings.TrimSpace(stderr.String())

	// Exit code 1 = avisos; >1 = erro. Em ambos os casos validamos o schema —
	// um cliente pg_restore antigo pode terminar com código 1 sem criar tabelas.
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			code := ee.ExitCode()
			if code > 1 {
				return fmt.Errorf("pg_restore (exit %d): %w: %s", code, runErr, errOut)
			}
		} else {
			return fmt.Errorf("pg_restore: %w: %s", runErr, errOut)
		}
	}

	if err := validateRestoredSchema(ctx, databaseURL); err != nil {
		hint := "verifique se o postgresql-client no contentor é >= versão do dump (ex.: postgresql-client-18)"
		if errOut != "" {
			return fmt.Errorf("pg_restore incompleto: %w (%s). stderr: %s", err, hint, errOut)
		}
		return fmt.Errorf("pg_restore incompleto: %w (%s)", err, hint)
	}
	return nil
}

func validateRestoredSchema(ctx context.Context, databaseURL string) error {
	bin, err := findTool("psql")
	if err != nil {
		return err
	}
	const q = `
SELECT string_agg(t, ', ' ORDER BY t)
FROM unnest(ARRAY['devices','users','goose_db_version']) AS t
WHERE NOT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = t
);
`
	cmd := exec.CommandContext(ctx, bin,
		"--dbname="+databaseURL,
		"-v", "ON_ERROR_STOP=1",
		"-t", "-A",
		"-c", q,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validação pós-restore: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	missing := strings.TrimSpace(stdout.String())
	if missing != "" {
		return fmt.Errorf("faltam tabelas essenciais após restore: %s — cliente pg_restore demasiado antigo para este dump?", missing)
	}
	return nil
}
