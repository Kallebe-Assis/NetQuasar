package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/localdbstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/panicguard"
)

// --- Limpeza de dados antigos que DE FATO devolve espaço ao disco ------------------------------------------------------
//
// Um `DELETE` no PostgreSQL só marca as linhas como mortas: o arquivo da tabela continua do mesmo tamanho e o disco do
// servidor não libera (só o próprio banco reaproveita o espaço, para novas gravações). Por isso esta rotina escolhe, por
// tabela, a estratégia que realmente encolhe o arquivo:
//
//	truncate — todas as linhas são antigas: TRUNCATE (instantâneo, devolve tudo).
//	rewrite  — a maioria é antiga: copia só o que FICA para uma tabela temporária, TRUNCATE e devolve (rápido; o arquivo
//	           volta ao tamanho do que sobrou). Precisa de espaço livre ≈ o que fica.
//	batch    — poucas linhas antigas: DELETE em lotes (sem transação gigante / pico de log) + VACUUM; opcionalmente
//	           VACUUM FULL para devolver o espaço ao disco, quando há folga.
//
// Roda em segundo plano (pode levar minutos) com progresso consultável, e só uma execução por vez.

const (
	purgeBatchRows          = 10000
	purgeBatchPause         = 40 * time.Millisecond
	purgeRewriteMinFraction = 0.5      // apagar ≥ 50% da tabela → reescrever só o que fica
	purgeCompactMinFraction = 0.2      // no modo em lotes, compactar (VACUUM FULL) se ≥ 20% saiu
	purgeMinCompactBytes    = 64 << 20 // tabelas menores que isto não valem o custo de compactar
	purgeSpaceMargin        = 256 << 20
	purgeUnknownDiskMaxKeep = 512 << 20 // sem leitura do disco, só reescreve se o que fica couber em ~512 MB
	purgeJobTimeout         = 3 * time.Hour
)

type dbPurgeTableResult struct {
	Table      string `json:"table"`
	Label      string `json:"label"`
	Strategy   string `json:"strategy"` // nenhum | truncate | rewrite | batch
	Eligible   int64  `json:"eligible"`
	Deleted    int64  `json:"deleted"`
	SizeBefore int64  `json:"size_before"`
	SizeAfter  int64  `json:"size_after"`
	Compacted  bool   `json:"compacted"`
	Note       string `json:"note,omitempty"`
	Error      string `json:"error,omitempty"`
}

type dbPurgeJob struct {
	mu            sync.Mutex
	ID            string
	Status        string // running | done | error
	Phase         string
	Table         string
	TablesTotal   int
	TablesDone    int
	DeletedTotal  int64
	OlderThanDays int
	Compact       bool
	DiskKnown     bool
	DiskFreeStart int64
	DiskFreeNow   int64
	DiskTotal     int64
	Results       []dbPurgeTableResult
	Message       string
	StartedAt     time.Time
	FinishedAt    time.Time
}

func (j *dbPurgeJob) update(fn func(j *dbPurgeJob)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn(j)
}

func (j *dbPurgeJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	var freed int64
	for _, r := range j.Results {
		if r.SizeBefore > r.SizeAfter {
			freed += r.SizeBefore - r.SizeAfter
		}
	}
	out := map[string]any{
		"job_id":          j.ID,
		"status":          j.Status,
		"phase":           j.Phase,
		"table":           j.Table,
		"tables_total":    j.TablesTotal,
		"tables_done":     j.TablesDone,
		"deleted_total":   j.DeletedTotal,
		"freed_bytes":     freed,
		"older_than_days": j.OlderThanDays,
		"compact":         j.Compact,
		"results":         append([]dbPurgeTableResult(nil), j.Results...),
		"message":         j.Message,
		"started_at":      j.StartedAt.UTC().Format(time.RFC3339),
	}
	if j.DiskKnown {
		out["disk_free_start_bytes"] = j.DiskFreeStart
		out["disk_free_bytes"] = j.DiskFreeNow
		out["disk_total_bytes"] = j.DiskTotal
	}
	if !j.FinishedAt.IsZero() {
		out["finished_at"] = j.FinishedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// Só uma limpeza por vez (duas ao mesmo tempo disputariam locks e espaço).
var dbPurgeState struct {
	mu      sync.Mutex
	current *dbPurgeJob
	byID    map[string]*dbPurgeJob
}

func startDBPurgeJob(days int, compact bool) (*dbPurgeJob, bool) {
	dbPurgeState.mu.Lock()
	defer dbPurgeState.mu.Unlock()
	if dbPurgeState.current != nil {
		dbPurgeState.current.mu.Lock()
		running := dbPurgeState.current.Status == "running"
		dbPurgeState.current.mu.Unlock()
		if running {
			return dbPurgeState.current, false
		}
	}
	j := &dbPurgeJob{ID: uuid.NewString(), Status: "running", Phase: "iniciando", OlderThanDays: days, Compact: compact, StartedAt: time.Now()}
	if dbPurgeState.byID == nil {
		dbPurgeState.byID = map[string]*dbPurgeJob{}
	}
	dbPurgeState.byID[j.ID] = j
	dbPurgeState.current = j
	return j, true
}

func findDBPurgeJob(id string) *dbPurgeJob {
	dbPurgeState.mu.Lock()
	defer dbPurgeState.mu.Unlock()
	if id == "" || id == "current" {
		return dbPurgeState.current
	}
	return dbPurgeState.byID[id]
}

// --- decisão de estratégia (pura, testável) ---------------------------------------------------------------------------

type purgePlanInput struct {
	Total, Eligible, SizeBytes int64
	Compact                    bool
	DiskKnown                  bool
	DiskFree                   int64
}

// keptBytesEstimate estima quanto o que SOBRA ocupa (proporcional ao número de linhas).
func keptBytesEstimate(total, eligible, sizeBytes int64) int64 {
	if total <= 0 {
		return 0
	}
	return int64(float64(sizeBytes) * float64(total-eligible) / float64(total))
}

// hasRoomFor diz se há espaço para uma operação que precisa de ~`need` bytes extras.
func hasRoomFor(need int64, diskKnown bool, diskFree int64) bool {
	if diskKnown {
		return diskFree >= int64(float64(need)*1.2)+purgeSpaceMargin
	}
	return need <= purgeUnknownDiskMaxKeep
}

func choosePurgeStrategy(in purgePlanInput) string {
	if in.Eligible <= 0 {
		return "nenhum"
	}
	kept := in.Total - in.Eligible
	if kept <= 0 {
		return "truncate"
	}
	fraction := float64(in.Eligible) / float64(in.Total)
	if in.Compact && fraction >= purgeRewriteMinFraction && in.SizeBytes >= purgeMinCompactBytes &&
		hasRoomFor(keptBytesEstimate(in.Total, in.Eligible, in.SizeBytes), in.DiskKnown, in.DiskFree) {
		return "rewrite"
	}
	return "batch"
}

// condWithCutoffLiteral troca $1 pelo corte como literal — DDL (CREATE TABLE AS) não aceita parâmetros.
func condWithCutoffLiteral(cond string, cutoff time.Time) string {
	return strings.ReplaceAll(cond, "$1", fmt.Sprintf("'%s'::timestamptz", cutoff.UTC().Format("2006-01-02T15:04:05.999999Z07:00")))
}

// --- execução ---------------------------------------------------------------------------------------------------------

func relTotalSize(ctx context.Context, pool *pgxpool.Pool, table string) int64 {
	var n int64
	_ = pool.QueryRow(ctx, `SELECT pg_total_relation_size($1::regclass)`, pgx.Identifier{table}.Sanitize()).Scan(&n)
	return n
}

func (s *Server) runDatabasePurge(job *dbPurgeJob, pool *pgxpool.Pool, specs []dbCleanupTableSpec, cutoff time.Time, actor string) {
	defer panicguard.Recover("db_purge")
	ctx, cancel := context.WithTimeout(context.Background(), purgeJobTimeout)
	defer cancel()

	dataDir := localdbstore.DataDir()
	free, total, diskOK := diskUsage(dataDir)
	job.update(func(j *dbPurgeJob) {
		j.DiskKnown, j.DiskFreeStart, j.DiskFreeNow, j.DiskTotal = diskOK, free, free, total
		j.TablesTotal = len(specs)
		j.Phase = "apagando"
	})

	for _, spec := range specs {
		job.update(func(j *dbPurgeJob) { j.Table = spec.Table; j.Phase = "apagando" })
		res := s.purgeOneTable(ctx, pool, spec, cutoff, job, dataDir)
		f, _, ok := diskUsage(dataDir)
		job.update(func(j *dbPurgeJob) {
			j.Results = append(j.Results, res)
			j.TablesDone++
			j.DeletedTotal += res.Deleted
			if ok {
				j.DiskFreeNow = f
			}
		})
	}

	job.update(func(j *dbPurgeJob) {
		j.Status, j.Phase, j.Table, j.FinishedAt = "done", "concluído", "", time.Now()
		j.Message = fmt.Sprintf("%d registro(s) apagado(s).", j.DeletedTotal)
	})
	snap := job.snapshot()
	s.appendAuditLog(context.Background(), "database", "cleanup", "purge_old_data", actor, nil, map[string]any{
		"older_than_days": job.OlderThanDays,
		"cutoff_at":       cutoff.UTC().Format(time.RFC3339),
		"compact":         job.Compact,
		"deleted_total":   snap["deleted_total"],
		"freed_bytes":     snap["freed_bytes"],
		"tables":          snap["results"],
	})
}

func (s *Server) purgeOneTable(ctx context.Context, pool *pgxpool.Pool, spec dbCleanupTableSpec, cutoff time.Time, job *dbPurgeJob, dataDir string) (res dbPurgeTableResult) {
	res = dbPurgeTableResult{Table: spec.Table, Label: spec.Label, Strategy: "nenhum"}
	fail := func(err error, what string) dbPurgeTableResult {
		res.Error = fmt.Sprintf("%s: %v", what, err)
		res.SizeAfter = relTotalSize(ctx, pool, spec.Table)
		return res
	}
	tbl := pgx.Identifier{spec.Table}.Sanitize()
	keepName := "_nq_keep_" + spec.Table
	keep := pgx.Identifier{keepName}.Sanitize()
	cond := cleanupWhereClause(spec)

	job.mu.Lock()
	compact := job.Compact
	job.mu.Unlock()

	// 0) sobras de uma reescrita interrompida: o que estava guardado volta para a tabela antes de qualquer coisa
	var keepExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, keepName).Scan(&keepExists); err == nil && keepExists {
		if err := restoreKept(ctx, pool, tbl, keep); err != nil {
			return fail(err, "restaurar dados guardados de uma limpeza anterior")
		}
		res.Note = "dados de uma limpeza interrompida foram restaurados antes de continuar. "
	}

	var total, eligible int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tbl)).Scan(&total); err != nil {
		return fail(err, "contar registros")
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s`, tbl, cond), cutoff).Scan(&eligible); err != nil {
		return fail(err, "contar registros antigos")
	}
	res.Eligible = eligible
	res.SizeBefore = relTotalSize(ctx, pool, spec.Table)
	res.SizeAfter = res.SizeBefore
	if eligible == 0 {
		return res
	}

	free, _, diskOK := diskUsage(dataDir)
	var hasGenerated bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND is_generated='ALWAYS')`, spec.Table).Scan(&hasGenerated)
	if hasGenerated {
		compact = false // a reescrita (INSERT … SELECT *) não funciona com colunas geradas; segue só truncate/lotes
	}
	strategy := choosePurgeStrategy(purgePlanInput{Total: total, Eligible: eligible, SizeBytes: res.SizeBefore, Compact: compact, DiskKnown: diskOK, DiskFree: free})
	res.Strategy = strategy

	switch strategy {
	case "truncate":
		job.update(func(j *dbPurgeJob) { j.Phase = "esvaziando " + spec.Table })
		if _, err := pool.Exec(ctx, `TRUNCATE TABLE `+tbl); err != nil {
			res.Note += "TRUNCATE não foi possível (" + err.Error() + "); usando exclusão em lotes. "
			strategy, res.Strategy = "batch", "batch"
		} else {
			res.Deleted = eligible
		}
	case "rewrite":
		job.update(func(j *dbPurgeJob) { j.Phase = "reescrevendo " + spec.Table + " (só o que fica)" })
		if err := rewriteKeepingRecent(ctx, pool, tbl, keep, condWithCutoffLiteral(cond, cutoff)); err != nil {
			var pending restorePendingError
			if errors.As(err, &pending) {
				return fail(err, "reescrita")
			}
			res.Note += "reescrita não foi possível (" + err.Error() + "); usando exclusão em lotes. "
			strategy, res.Strategy = "batch", "batch"
		} else {
			res.Deleted = eligible
			res.Compacted = true
			_, _ = pool.Exec(ctx, `ANALYZE `+tbl)
		}
	}

	if strategy == "batch" {
		deleted, err := deleteInBatches(ctx, pool, tbl, cond, cutoff, func(n int64) {
			job.update(func(j *dbPurgeJob) { j.Phase = fmt.Sprintf("apagando %s (%d)", spec.Table, n) })
		})
		res.Deleted = deleted
		if err != nil {
			return fail(err, "apagar em lotes")
		}
		job.update(func(j *dbPurgeJob) { j.Phase = "VACUUM " + spec.Table })
		if _, err := pool.Exec(ctx, `VACUUM (ANALYZE) `+tbl); err != nil {
			res.Note += "VACUUM falhou: " + err.Error() + ". "
		}
		fraction := float64(deleted) / float64(maxInt64(total, 1))
		if compact && res.SizeBefore >= purgeMinCompactBytes && fraction >= purgeCompactMinFraction {
			f2, _, ok2 := diskUsage(dataDir)
			if hasRoomFor(keptBytesEstimate(total, deleted, res.SizeBefore), ok2, f2) {
				job.update(func(j *dbPurgeJob) { j.Phase = "compactando " + spec.Table + " (VACUUM FULL)" })
				if _, err := pool.Exec(ctx, `VACUUM (FULL, ANALYZE) `+tbl); err != nil {
					res.Note += "compactação falhou: " + err.Error() + ". "
				} else {
					res.Compacted = true
				}
			} else {
				res.Note += "sem espaço livre suficiente para compactar — o espaço foi liberado só dentro do banco. Libere disco e rode de novo. "
			}
		}
	}
	res.SizeAfter = relTotalSize(ctx, pool, spec.Table)
	// aviso só quando faz diferença: tabela grande cujo arquivo não encolheu
	if !res.Compacted && res.SizeAfter >= res.SizeBefore && res.Deleted > 0 && res.Note == "" && res.SizeBefore >= purgeMinCompactBytes {
		if compact {
			res.Note = "poucas linhas antigas: o espaço liberado fica reservado dentro do banco e será reaproveitado nas próximas gravações. "
		} else {
			res.Note = "linhas apagadas; o espaço fica reservado dentro do banco (ative «Compactar» para devolvê-lo ao disco). "
		}
	}
	return res
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// deleteInBatches apaga em lotes de purgeBatchRows (cada lote é uma transação curta: sem pico de log nem lock longo).
func deleteInBatches(ctx context.Context, pool *pgxpool.Pool, tbl, cond string, cutoff time.Time, progress func(deleted int64)) (int64, error) {
	q := fmt.Sprintf(`DELETE FROM %s WHERE ctid = ANY(ARRAY(SELECT ctid FROM %s WHERE %s LIMIT %d))`, tbl, tbl, cond, purgeBatchRows)
	var deleted int64
	for {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		tag, err := pool.Exec(ctx, q, cutoff)
		if err != nil {
			return deleted, err
		}
		n := tag.RowsAffected()
		if n == 0 {
			return deleted, nil
		}
		deleted += n
		progress(deleted)
		time.Sleep(purgeBatchPause)
	}
}

// rewriteKeepingRecent: copia o que FICA (logada, para sobreviver a uma queda), esvazia a tabela (devolve o arquivo ao disco)
// e devolve os dados. A 1ª transação trava a tabela, então nada é gravado entre copiar e esvaziar.
func rewriteKeepingRecent(ctx context.Context, pool *pgxpool.Pool, tbl, keep, condLiteral string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE `+tbl+` IN ACCESS EXCLUSIVE MODE`); err != nil {
		return err
	}
	// COALESCE: linha com data NULL nunca é considerada «antiga» — fica
	if _, err := tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s AS SELECT * FROM %s WHERE NOT COALESCE((%s), false)`, keep, tbl, condLiteral)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `TRUNCATE TABLE `+tbl); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// a partir daqui o arquivo grande já foi devolvido ao disco; falta repor o que ficou guardado
	if err := restoreKept(ctx, pool, tbl, keep); err != nil {
		return restorePendingError{fmt.Errorf("os dados mantidos estão guardados em %s e serão restaurados na próxima execução: %w", keep, err)}
	}
	return nil
}

// restorePendingError: a tabela já foi esvaziada e os dados mantidos estão na tabela temporária. Não há «plano B».
type restorePendingError struct{ err error }

func (e restorePendingError) Error() string { return e.err.Error() }
func (e restorePendingError) Unwrap() error { return e.err }

func restoreKept(ctx context.Context, pool *pgxpool.Pool, tbl, keep string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s OVERRIDING SYSTEM VALUE SELECT * FROM %s`, tbl, keep)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DROP TABLE `+keep); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
