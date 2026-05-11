package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/api"
	"github.com/netquasar/netquasar/quasar_backend/internal/bootstrap"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
	"github.com/netquasar/netquasar/quasar_backend/internal/localdbstore"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// loadEnvFiles mescla .env e depois .env.local (chaves repetidas: vale o .env.local).
// Não sobrescreve variáveis já definidas no ambiente do processo (shell / sistema).
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
		log.Fatal().Err(err).Msg("config")
	}
	if err := localdbstore.MergeIntoConfig(cfg); err != nil {
		log.Fatal().Err(err).Msg("credenciais locais (data/database-credentials.json)")
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NETQUASAR_LOG_CONSOLE")))
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	if lvl, e := zerolog.ParseLevel(strings.ToLower(cfg.LogLevel)); e == nil {
		zerolog.SetGlobalLevel(lvl)
	}

	ctx := context.Background()
	var dbHolder atomic.Pointer[pgxpool.Pool]
	defer func() {
		if p := dbHolder.Swap(nil); p != nil {
			p.Close()
		}
	}()

	if cfg.CanConnectDatabase() {
		if err := db.Migrate(ctx, cfg); err != nil {
			log.Fatal().Err(err).Msg("migrate")
		}
		pool, err := db.NewPool(ctx, cfg)
		if err != nil {
			log.Fatal().Err(err).Msg("db pool")
		}
		dbHolder.Store(pool)
		if err := bootstrap.EnsureDefaultUsers(ctx, pool); err != nil {
			log.Fatal().Err(err).Msg("seed users")
		}
		if err := bootstrap.EnsureDatabaseMetaRow(ctx, pool, cfg); err != nil {
			log.Fatal().Err(err).Msg("database meta")
		}
	} else {
		log.Warn().Msg("sem ligação PostgreSQL: configure em /api/v1/setup ou defina NETQUASAR_DATABASE_URL / credenciais NETQUASAR_DB_*")
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewServer(log.Logger, cfg, &dbHolder, workerCtx),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
	}

	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("http listen")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http")
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("shutdown")
	}
	workerCancel()
}
