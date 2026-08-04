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
	return "", fmt.Errorf("%s não encontrado no PATH (instale postgresql-client)", name)
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

// RestoreFull restaura dump custom (após WipePublicSchema).
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
	err = cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("pg_restore: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
