// Dbping — utilitário de diagnóstico: lê .env na pasta atual e tenta ping ao Postgres (mesma lógica DSN que o servidor).
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
)

func loadEnvFiles() {
	merged := make(map[string]string)
	for _, name := range []string{".env", ".env.local"} {
		b, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			i := strings.IndexByte(line, '=')
			if i <= 0 {
				continue
			}
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			if k == "" {
				continue
			}
			merged[k] = v
		}
	}
	for k, v := range merged {
		if os.Getenv(k) != "" {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "(dsn inválida)"
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}

func main() {
	loadEnvFiles()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config.Load: %v\n", err)
		os.Exit(1)
	}
	dsn := cfg.PostgresDSN()
	fmt.Println("DSN (senha ocultada):", redactDSN(dsn))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewPool/Ping: %s\n", config.FormatConnectErrWithSupabaseHint(ctx, dsn, err))
		os.Exit(2)
	}
	pool.Close()
	fmt.Println("OK: ligação e ping bem-sucedidos.")
}
