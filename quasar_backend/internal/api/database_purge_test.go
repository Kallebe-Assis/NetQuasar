package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChoosePurgeStrategy(t *testing.T) {
	const GB = int64(1 << 30)
	cases := []struct {
		name string
		in   purgePlanInput
		want string
	}{
		{"nada antigo", purgePlanInput{Total: 100, Eligible: 0, SizeBytes: GB, Compact: true}, "nenhum"},
		{"tudo antigo → truncate", purgePlanInput{Total: 100, Eligible: 100, SizeBytes: GB, Compact: true}, "truncate"},
		{"tudo antigo → truncate mesmo sem compactar", purgePlanInput{Total: 100, Eligible: 100, SizeBytes: GB, Compact: false}, "truncate"},
		{"maioria antiga e cabe → rewrite", purgePlanInput{Total: 100, Eligible: 90, SizeBytes: 10 * GB, Compact: true, DiskKnown: true, DiskFree: 4 * GB}, "rewrite"},
		{"maioria antiga, sem espaço p/ o que fica → batch", purgePlanInput{Total: 100, Eligible: 60, SizeBytes: 10 * GB, Compact: true, DiskKnown: true, DiskFree: 1 * GB}, "batch"},
		{"poucas antigas → batch", purgePlanInput{Total: 100, Eligible: 10, SizeBytes: 10 * GB, Compact: true, DiskKnown: true, DiskFree: 20 * GB}, "batch"},
		{"compactar desligado → batch", purgePlanInput{Total: 100, Eligible: 90, SizeBytes: 10 * GB, Compact: false, DiskKnown: true, DiskFree: 20 * GB}, "batch"},
		{"tabela pequena → batch", purgePlanInput{Total: 100, Eligible: 90, SizeBytes: 10 << 20, Compact: true, DiskKnown: true, DiskFree: 20 * GB}, "batch"},
		{"disco desconhecido, o que fica é pequeno → rewrite", purgePlanInput{Total: 100, Eligible: 90, SizeBytes: 2 * GB, Compact: true}, "rewrite"},
		{"disco desconhecido, o que fica é grande → batch", purgePlanInput{Total: 100, Eligible: 60, SizeBytes: 10 * GB, Compact: true}, "batch"},
	}
	for _, c := range cases {
		if got := choosePurgeStrategy(c.in); got != c.want {
			t.Errorf("%s: %q, esperado %q", c.name, got, c.want)
		}
	}
}

func TestCondWithCutoffLiteral(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 12, 30, 0, 0, time.UTC)
	got := condWithCutoffLiteral("closed_at IS NOT NULL AND closed_at < $1", cutoff)
	want := "closed_at IS NOT NULL AND closed_at < '2026-09-01T12:30:00Z'::timestamptz"
	if got != want {
		t.Errorf("literal do corte:\n got  %s\n want %s", got, want)
	}
	if strings.Contains(got, "$1") {
		t.Error("o parâmetro $1 deveria ter sido substituído")
	}
}

// Teste de integração com PostgreSQL de verdade. Só roda com NQ_TEST_PG_DSN (DSN administrativo — ele cria e apaga um banco
// temporário próprio, nunca toca nos dados existentes). Ex. dentro do container do Postgres:
//
//	NQ_TEST_PG_DSN='postgres://USER@/postgres?host=/var/run/postgresql' NETQUASAR_DATA_DIR=/tmp ./api.test -test.run Integration -test.v
func TestDatabasePurgeIntegration(t *testing.T) {
	adminDSN := os.Getenv("NQ_TEST_PG_DSN")
	if adminDSN == "" {
		t.Skip("defina NQ_TEST_PG_DSN para rodar o teste de integração da limpeza")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("conectar (admin): %v", err)
	}
	defer admin.Close(ctx)
	dbName := "nq_purge_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatalf("criar banco temporário: %v", err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)") }()

	cfg, err := pgxpool.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = dbName
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	must := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s\n→ %v", q, err)
		}
	}
	count := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("%s\n→ %v", q, err)
		}
		return n
	}

	// --- tabela "grande" parecida com ping_history: 70% das linhas são antigas → estratégia rewrite ---
	must(`CREATE TABLE ping_history (id BIGSERIAL PRIMARY KEY, device_id UUID NOT NULL, checked_at TIMESTAMPTZ NOT NULL, ok BOOLEAN NOT NULL, detail JSONB NOT NULL DEFAULT '{}')`)
	must(`CREATE INDEX idx_ph ON ping_history (device_id, checked_at DESC)`)
	must(`INSERT INTO ping_history (device_id, checked_at, ok, detail)
		SELECT gen_random_uuid(), now() - (CASE WHEN g % 10 < 7 THEN interval '40 days' ELSE interval '1 day' END) - (g || ' seconds')::interval, true,
		       jsonb_build_object('pad', repeat('x', 300))
		FROM generate_series(1, 300000) g`)
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	keptExpected := count(`SELECT COUNT(*) FROM ping_history WHERE checked_at >= $1`, cutoff)
	maxIDBefore := count(`SELECT max(id) FROM ping_history`)

	s := &Server{}
	job := &dbPurgeJob{Status: "running", Compact: true}
	var spec dbCleanupTableSpec
	for _, sp := range dbCleanupTables {
		if sp.Table == "ping_history" {
			spec = sp
		}
	}
	res := s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	t.Logf("ping_history: %+v", res)
	if res.Error != "" {
		t.Fatalf("erro: %s", res.Error)
	}
	if res.Strategy != "rewrite" {
		t.Errorf("estratégia %q, esperado rewrite", res.Strategy)
	}
	if got := count(`SELECT COUNT(*) FROM ping_history`); got != keptExpected {
		t.Errorf("linhas após limpar: %d, esperado %d", got, keptExpected)
	}
	if count(`SELECT COUNT(*) FROM ping_history WHERE checked_at < $1`, cutoff) != 0 {
		t.Error("sobraram linhas antigas")
	}
	if res.SizeAfter >= res.SizeBefore/2 {
		t.Errorf("o arquivo deveria encolher bastante: antes %d, depois %d", res.SizeBefore, res.SizeAfter)
	}
	if count(`SELECT max(id) FROM ping_history`) > maxIDBefore || count(`SELECT COUNT(*) FROM ping_history WHERE id IS NULL`) != 0 {
		t.Error("ids devem ser preservados")
	}
	// a sequência continua de onde estava e o índice continua funcionando
	must(`INSERT INTO ping_history (device_id, checked_at, ok) VALUES (gen_random_uuid(), now(), true)`)
	if count(`SELECT min(id) FROM ping_history WHERE checked_at > now() - interval '1 minute'`) <= maxIDBefore {
		t.Error("a sequência de ids não deve voltar atrás")
	}
	if count(`SELECT COUNT(*) FROM pg_class WHERE relname = '_nq_keep_ping_history'`) != 0 {
		t.Error("a tabela temporária deveria ter sido removida")
	}

	// --- poucas linhas antigas (10%) → batch + VACUUM ---
	must(`CREATE TABLE telemetry_samples (id BIGSERIAL PRIMARY KEY, collected_at TIMESTAMPTZ NOT NULL, pad TEXT)`)
	must(`INSERT INTO telemetry_samples (collected_at, pad) SELECT now() - (CASE WHEN g % 10 = 0 THEN interval '45 days' ELSE interval '1 hour' END), repeat('y', 200) FROM generate_series(1, 50000) g`)
	for _, sp := range dbCleanupTables {
		if sp.Table == "telemetry_samples" {
			spec = sp
		}
	}
	res = s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	t.Logf("telemetry_samples: %+v", res)
	if res.Error != "" || res.Strategy != "batch" || res.Deleted != 5000 {
		t.Errorf("batch inesperado: %+v", res)
	}
	if got := count(`SELECT COUNT(*) FROM telemetry_samples`); got != 45000 {
		t.Errorf("restaram %d linhas, esperado 45000", got)
	}

	// --- 30% antigas numa tabela grande → batch + VACUUM FULL (devolve o espaço) ---
	must(`CREATE TABLE bng_session_snapshots (id BIGSERIAL PRIMARY KEY, captured_at TIMESTAMPTZ NOT NULL, pad TEXT)`)
	must(`INSERT INTO bng_session_snapshots (captured_at, pad) SELECT now() - (CASE WHEN g % 10 < 3 THEN interval '50 days' ELSE interval '2 hours' END), repeat('w', 200) FROM generate_series(1, 400000) g`)
	for _, sp := range dbCleanupTables {
		if sp.Table == "bng_session_snapshots" {
			spec = sp
		}
	}
	res = s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	t.Logf("bng_session_snapshots: %+v", res)
	if res.Error != "" || res.Strategy != "batch" || !res.Compacted || res.Deleted != 120000 {
		t.Errorf("batch+VACUUM FULL inesperado: %+v", res)
	}
	if res.SizeAfter >= res.SizeBefore*85/100 {
		t.Errorf("VACUUM FULL deveria devolver espaço: antes %d, depois %d", res.SizeBefore, res.SizeAfter)
	}
	if got := count(`SELECT COUNT(*) FROM bng_session_snapshots`); got != 280000 {
		t.Errorf("restaram %d linhas, esperado 280000", got)
	}

	// --- tudo antigo → truncate ---
	must(`CREATE TABLE interface_snapshots (id BIGSERIAL PRIMARY KEY, collected_at TIMESTAMPTZ NOT NULL)`)
	must(`INSERT INTO interface_snapshots (collected_at) SELECT now() - interval '90 days' FROM generate_series(1, 1000)`)
	for _, sp := range dbCleanupTables {
		if sp.Table == "interface_snapshots" {
			spec = sp
		}
	}
	res = s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	if res.Error != "" || res.Strategy != "truncate" || res.Deleted != 1000 || count(`SELECT COUNT(*) FROM interface_snapshots`) != 0 {
		t.Errorf("truncate inesperado: %+v", res)
	}

	// --- guarda closed_only: linha com closed_at NULL (alerta aberto) NUNCA pode ser apagada, mesmo na reescrita ---
	must(`CREATE TABLE alert_instances (id BIGSERIAL PRIMARY KEY, closed_at TIMESTAMPTZ, pad TEXT)`)
	must(`INSERT INTO alert_instances (closed_at, pad) SELECT CASE WHEN g % 10 < 8 THEN now() - interval '60 days' END, repeat('z', 4000) FROM generate_series(1, 30000) g`)
	for _, sp := range dbCleanupTables {
		if sp.Table == "alert_instances" {
			spec = sp
		}
	}
	openBefore := count(`SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`)
	res = s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	t.Logf("alert_instances: %+v", res)
	if res.Error != "" {
		t.Fatalf("erro: %s", res.Error)
	}
	if got := count(`SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`); got != openBefore || openBefore == 0 {
		t.Errorf("alertas abertos devem permanecer: %d → %d", openBefore, got)
	}

	// --- sobra de uma reescrita interrompida: o que estava guardado volta antes de continuar ---
	must(`CREATE TABLE events (id BIGSERIAL PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL)`)
	must(`INSERT INTO events (created_at) SELECT now() FROM generate_series(1, 10)`)
	must(`CREATE TABLE _nq_keep_events AS SELECT * FROM events`)
	must(`TRUNCATE events`)
	for _, sp := range dbCleanupTables {
		if sp.Table == "events" {
			spec = sp
		}
	}
	res = s.purgeOneTable(ctx, pool, spec, cutoff, job, os.TempDir())
	if res.Error != "" || count(`SELECT COUNT(*) FROM events`) != 10 || count(`SELECT COUNT(*) FROM pg_class WHERE relname='_nq_keep_events'`) != 0 {
		t.Errorf("a restauração das sobras falhou: %+v", res)
	}
}
