// Migrate — aplica migrações Goose ao Postgres (mesmo .env que netquasar/dbping).
package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
	"github.com/netquasar/netquasar/quasar_backend/internal/localdbstore"
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

func main() {
	loadEnvFiles()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := localdbstore.MergeIntoConfig(cfg); err != nil {
		log.Fatal(err)
	}
	if !cfg.CanConnectDatabase() {
		log.Fatal("sem credenciais PostgreSQL: defina NETQUASAR_DATABASE_URL ou NETQUASAR_DB_PASSWORD (ou data/database-credentials.json)")
	}
	if err := db.Migrate(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
	log.Println("migrations concluídas")
}
