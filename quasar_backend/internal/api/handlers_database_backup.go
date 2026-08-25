package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/backupb2"
	"github.com/netquasar/netquasar/quasar_backend/internal/bootstrap"
	"github.com/netquasar/netquasar/quasar_backend/internal/config"
	"github.com/netquasar/netquasar/quasar_backend/internal/db"
	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
	"github.com/rs/zerolog"
)

type dbRestoreJob struct {
	ID          string     `json:"job_id"`
	Status      string     `json:"status"`
	ProgressPct int        `json:"progress_pct"`
	CurrentStep string     `json:"current_step"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func (s *Server) ensureDBRestoreJobs() {
	s.dbRestoreMu.Lock()
	if s.dbRestoreJobs == nil {
		s.dbRestoreJobs = make(map[string]*dbRestoreJob)
	}
	s.dbRestoreMu.Unlock()
}

// ensureAutomationDatabaseBackupSchema cria tabelas do backup automático se faltarem
// (ex.: migration 092 não aplicada ou restore incompleto).
func (s *Server) ensureAutomationDatabaseBackupSchema(ctx context.Context) error {
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS settings_b2_backup (
			id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			key_id TEXT,
			application_key TEXT,
			bucket TEXT,
			bucket_id TEXT,
			endpoint TEXT,
			region TEXT NOT NULL DEFAULT 'us-east-005',
			prefix TEXT NOT NULL DEFAULT 'netquasar/postgres',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settings_b2_backup (id) VALUES (1) ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS automation_database_backup (
			id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			enabled BOOLEAN NOT NULL DEFAULT false,
			frequency TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly', 'custom')),
			day_of_week INT CHECK (day_of_week IS NULL OR (day_of_week >= 0 AND day_of_week <= 6)),
			days_of_week INT[] NOT NULL DEFAULT '{}',
			time_hhmm TEXT NOT NULL DEFAULT '03:00' CHECK (time_hhmm ~ '^\d{2}:\d{2}$'),
			timezone TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
			keep_last INT NOT NULL DEFAULT 14 CHECK (keep_last >= 1 AND keep_last <= 365),
			last_run_at TIMESTAMPTZ,
			last_run_key TEXT,
			last_status TEXT,
			last_error TEXT,
			last_object_key TEXT,
			last_size_bytes BIGINT,
			running BOOLEAN NOT NULL DEFAULT false,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE automation_database_backup ADD COLUMN IF NOT EXISTS days_of_week INT[] NOT NULL DEFAULT '{}'`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE automation_database_backup DROP CONSTRAINT IF EXISTS automation_database_backup_frequency_check`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE automation_database_backup
		ADD CONSTRAINT automation_database_backup_frequency_check
		CHECK (frequency IN ('daily', 'weekly', 'custom'))
	`); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO automation_database_backup (id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
	return err
}

func (s *Server) loadB2Creds(ctx context.Context) (backupb2.Creds, error) {
	pool := s.DB()
	if pool == nil {
		return backupb2.Creds{}, fmt.Errorf("base indisponível")
	}
	if err := s.ensureAutomationDatabaseBackupSchema(ctx); err != nil {
		return backupb2.Creds{}, err
	}
	var keyID, appKey, bucket, bucketID, endpoint, region, prefix *string
	err := pool.QueryRow(ctx, `
		SELECT key_id, application_key, bucket, bucket_id, endpoint, region, prefix
		FROM settings_b2_backup WHERE id = 1
	`).Scan(&keyID, &appKey, &bucket, &bucketID, &endpoint, &region, &prefix)
	if err != nil {
		return backupb2.Creds{}, err
	}
	return backupb2.Creds{
		KeyID:          strOrEmpty(keyID),
		ApplicationKey: strOrEmpty(appKey),
		Bucket:         strOrEmpty(bucket),
		BucketID:       strOrEmpty(bucketID),
		Endpoint:       strOrEmpty(endpoint),
		Region:         strOrDef(region, "us-east-005"),
		Prefix:         strOrDef(prefix, "netquasar/postgres"),
	}, nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func strOrDef(p *string, def string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return def
	}
	return strings.TrimSpace(*p)
}

func (s *Server) activeDatabaseDSN(ctx context.Context) (string, error) {
	pool := s.DB()
	if pool != nil {
		var host, user, name, ssl, pass *string
		var port *int
		err := pool.QueryRow(ctx, `
			SELECT host, port, db_user, db_name, ssl_mode, db_password
			FROM settings_database_meta WHERE id = 1
		`).Scan(&host, &port, &user, &name, &ssl, &pass)
		if err == nil && host != nil && user != nil && name != nil && pass != nil &&
			*host != "" && *user != "" && *name != "" && *pass != "" {
			p := 5432
			if port != nil && *port > 0 {
				p = *port
			}
			sm := "disable"
			if ssl != nil && *ssl != "" {
				sm = *ssl
			}
			return config.EnsureReadWriteSessionAttrs(
				config.EnsureSupabaseSSLRootCertIfNeeded(
					config.PostgresURLFromParts(*host, p, *user, *pass, *name, sm),
				),
			), nil
		}
	}
	if s.Cfg != nil {
		dsn := strings.TrimSpace(s.Cfg.PostgresDSN())
		if dsn != "" {
			return dsn, nil
		}
	}
	return "", fmt.Errorf("não foi possível determinar a DSN activa")
}

func (s *Server) getSettingsB2Backup(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	if err := s.ensureAutomationDatabaseBackupSchema(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var keyID, bucket, bucketID, endpoint, region, prefix *string
	var hasKey bool
	err := pool.QueryRow(r.Context(), `
		SELECT key_id, bucket, bucket_id, endpoint, region, prefix,
			(application_key IS NOT NULL AND length(trim(application_key)) > 0)
		FROM settings_b2_backup WHERE id = 1
	`).Scan(&keyID, &bucket, &bucketID, &endpoint, &region, &prefix, &hasKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key_id":              keyID,
		"bucket":              bucket,
		"bucket_id":           bucketID,
		"endpoint":            endpoint,
		"region":              region,
		"prefix":              prefix,
		"application_key_set": hasKey,
	})
}

func (s *Server) patchSettingsB2Backup(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	if err := s.ensureAutomationDatabaseBackupSchema(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	keyID, _ := body["key_id"].(string)
	appKey, _ := body["application_key"].(string)
	bucket, _ := body["bucket"].(string)
	bucketID, _ := body["bucket_id"].(string)
	endpoint, _ := body["endpoint"].(string)
	region, _ := body["region"].(string)
	prefix, _ := body["prefix"].(string)

	_, err := pool.Exec(r.Context(), `
		UPDATE settings_b2_backup SET
			key_id = COALESCE(NULLIF($1,''), key_id),
			application_key = CASE WHEN NULLIF($2,'') IS NULL THEN application_key ELSE $2 END,
			bucket = COALESCE(NULLIF($3,''), bucket),
			bucket_id = COALESCE(NULLIF($4,''), bucket_id),
			endpoint = COALESCE(NULLIF($5,''), endpoint),
			region = COALESCE(NULLIF($6,''), region),
			prefix = COALESCE(NULLIF($7,''), prefix),
			updated_at = now()
		WHERE id = 1
	`, keyID, appKey, bucket, bucketID, endpoint, region, prefix)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_b2_backup", "1", "patch", s.actorFromRequest(r), nil, map[string]any{
		"bucket": bucket, "prefix": prefix,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testSettingsB2Backup(w http.ResponseWriter, r *http.Request) {
	creds, err := s.loadB2Creds(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2", err.Error(), nil)
		return
	}
	cli, err := backupb2.NewClient(r.Context(), creds)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2_AUTH", err.Error(), nil)
		return
	}
	files, err := cli.ListDumps(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2_LIST", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dump_count": len(files)})
}

func (s *Server) getAutomationDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	if err := s.ensureAutomationDatabaseBackupSchema(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var en, running bool
	var freq, th, tz, lastKey, lastStatus, lastErr, lastObj *string
	var dow, keep *int
	var days []int32
	var lr *time.Time
	var lastSize *int64
	err := pool.QueryRow(r.Context(), `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone, keep_last,
			last_run_key, last_run_at, last_status, last_error, last_object_key, last_size_bytes, running, days_of_week
		FROM automation_database_backup WHERE id = 1
	`).Scan(&en, &freq, &dow, &th, &tz, &keep, &lastKey, &lr, &lastStatus, &lastErr, &lastObj, &lastSize, &running, &days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         en,
		"frequency":       freq,
		"day_of_week":     dow,
		"days_of_week":    intSliceFromInt32(days),
		"time_hhmm":       th,
		"timezone":        tz,
		"keep_last":       keep,
		"last_run_key":    lastKey,
		"last_run_at":     lr,
		"last_status":     lastStatus,
		"last_error":      lastErr,
		"last_object_key": lastObj,
		"last_size_bytes": lastSize,
		"running":         running,
	})
}

func (s *Server) patchAutomationDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "pool não configurado", nil)
		return
	}
	if err := s.ensureAutomationDatabaseBackupSchema(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	freq, _ := body["frequency"].(string)
	th, _ := body["time_hhmm"].(string)
	tz, _ := body["timezone"].(string)
	var dow *int
	if v, ok := body["day_of_week"].(float64); ok {
		d := int(v)
		dow = &d
	}
	days := parseDaysOfWeekBody(body)
	var keep *int
	if v, ok := body["keep_last"].(float64); ok {
		k := int(v)
		keep = &k
	}
	resetLast := schedulePatchResetsLastRun(body)
	_, err := pool.Exec(r.Context(), `
		UPDATE automation_database_backup SET
			enabled = COALESCE($1, enabled),
			frequency = COALESCE(NULLIF($2,''), frequency),
			day_of_week = COALESCE($3, day_of_week),
			time_hhmm = COALESCE(NULLIF($4,''), time_hhmm),
			timezone = COALESCE(NULLIF($5,''), timezone),
			keep_last = COALESCE($6, keep_last),
			last_run_key = CASE WHEN $7 THEN NULL ELSE last_run_key END,
			last_run_at = CASE WHEN $7 THEN NULL ELSE last_run_at END,
			running = CASE WHEN $7 THEN false ELSE running END,
			days_of_week = COALESCE($8, days_of_week),
			updated_at = now()
		WHERE id = 1
	`, boolPtr(body, "enabled"), freq, dow, th, tz, keep, resetLast, days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_database_backup", "1", "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	runKey := time.Now().Format("2006-01-02") + "-manual"
	meta := s.automationMetaFromRequest(r)
	go func() {
		_ = s.executeDatabaseBackup(context.Background(), runKey, meta)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "Backup iniciado"})
}

func (s *Server) tryScheduledDatabaseBackup(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	if err := s.ensureAutomationDatabaseBackupSchema(ctx); err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("automation database backup: schema")
		}
		return
	}
	s.clearStaleAutomationRunning(ctx, "automation_database_backup")
	var en, running bool
	var freq, th, tz, lastKey, lastStatus *string
	var dow *int
	var days []int32
	var lr, updatedAt *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone, last_run_key, last_run_at, running, days_of_week,
			last_status, updated_at
		FROM automation_database_backup WHERE id = 1
	`).Scan(&en, &freq, &dow, &th, &tz, &lastKey, &lr, &running, &days, &lastStatus, &updatedAt)
	if err != nil || !en {
		return
	}
	// Em falha (ex.: 503 B2), o due permanece true e o loop de 30s martelava o B2.
	if lastStatus != nil && strings.EqualFold(strings.TrimSpace(*lastStatus), "error") &&
		updatedAt != nil && time.Since(*updatedAt) < 20*time.Minute {
		return
	}
	frequency := "daily"
	if freq != nil {
		frequency = *freq
	}
	tzStr := "America/Sao_Paulo"
	if tz != nil && strings.TrimSpace(*tz) != "" {
		tzStr = *tz
	}
	thStr := "03:00"
	if th != nil {
		thStr = *th
	}
	runKey, due := scheduleutil.DailyWeeklyDueOnDays(en, frequency, tzStr, thStr, dow, intSliceFromInt32(days), lastKey, lr, running, time.Now())
	if !due {
		return
	}
	// Igual ao manual: não herda o WorkerCtx do loop (evita cancel/timeout curto no upload).
	meta := automationMetaFromActor(auditActorSistema, nil)
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if err := s.executeDatabaseBackup(runCtx, runKey, meta); err != nil && log != nil {
			log.Warn().Err(err).Str("run_key", runKey).Msg("backup B2 agendado falhou")
		}
	}()
}

func (s *Server) executeDatabaseBackup(ctx context.Context, runKey string, meta automationRunMeta) error {
	started := time.Now()
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	if err := s.ensureAutomationDatabaseBackupSchema(ctx); err != nil {
		return err
	}
	tag, err := pool.Exec(ctx, `
		UPDATE automation_database_backup SET running = true, updated_at = now()
		WHERE id = 1 AND running = false
	`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		s.recordAutomationExecution(ctx, jobDatabaseBackup, meta, started, false,
			"Não iniciado (já em execução)", nil, map[string]any{"run_key": runKey}, runKey)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE automation_database_backup SET running = false, updated_at = now() WHERE id = 1`)
	}()

	dsn, err := s.activeDatabaseDSN(ctx)
	if err != nil {
		s.failDBBackup(ctx, meta, started, runKey, err)
		return err
	}
	creds, err := s.loadB2Creds(ctx)
	if err != nil {
		s.failDBBackup(ctx, meta, started, runKey, err)
		return err
	}
	tmpDir := filepath.Join(os.TempDir(), "netquasar-backup")
	_ = os.MkdirAll(tmpDir, 0o755)
	fileName := backupb2.StampFileName(time.Now())
	localPath := filepath.Join(tmpDir, fileName)
	defer os.Remove(localPath)

	if err := backupb2.DumpFull(ctx, dsn, localPath); err != nil {
		s.failDBBackup(ctx, meta, started, runKey, err)
		return err
	}
	cli, err := backupb2.NewClient(ctx, creds)
	if err != nil {
		s.failDBBackup(ctx, meta, started, runKey, err)
		return err
	}
	objKey := backupb2.ObjectKeyForDump(creds.Prefix, fileName)
	fileID, size, err := cli.UploadFile(ctx, localPath, objKey)
	if err != nil {
		s.failDBBackup(ctx, meta, started, runKey, err)
		return err
	}
	_, _ = pool.Exec(ctx, `
		UPDATE automation_database_backup SET
			last_run_at = now(), last_run_key = $1, last_status = 'ok', last_error = NULL,
			last_object_key = $2, last_size_bytes = $3, updated_at = now()
		WHERE id = 1
	`, runKey, objKey, size)
	sum := map[string]any{"run_key": runKey, "object_key": objKey, "size_bytes": size, "file_id": fileID}
	s.recordAutomationExecution(ctx, jobDatabaseBackup, meta, started, true,
		"Backup enviado ao B2", nil, sum, runKey)
	s.appendAuditLog(ctx, "automation_database_backup", "1", "run", meta.Actor, nil, sum)
	return nil
}

func (s *Server) failDBBackup(ctx context.Context, meta automationRunMeta, started time.Time, runKey string, err error) {
	pool := s.DB()
	if pool != nil {
		msg := err.Error()
		_, _ = pool.Exec(ctx, `
			UPDATE automation_database_backup SET last_status = 'error', last_error = $1, updated_at = now() WHERE id = 1
		`, msg)
	}
	s.recordAutomationExecution(ctx, jobDatabaseBackup, meta, started, false,
		"Falha no backup", err, map[string]any{"run_key": runKey}, runKey)
}

func (s *Server) listDatabaseBackupsB2(w http.ResponseWriter, r *http.Request) {
	creds, err := s.loadB2Creds(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2", err.Error(), nil)
		return
	}
	cli, err := backupb2.NewClient(r.Context(), creds)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2_AUTH", err.Error(), nil)
		return
	}
	files, err := cli.ListDumps(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "B2_LIST", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) uploadDatabaseBackupRestore(w http.ResponseWriter, r *http.Request) {
	s.ensureDBRestoreJobs()
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_MULTIPART", err.Error(), nil)
		return
	}
	if strings.TrimSpace(r.FormValue("confirm")) != "RESTORE" {
		writeErr(w, http.StatusUnprocessableEntity, "CONFIRM", `envie confirm=RESTORE para confirmar o wipe`, nil)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NO_FILE", "campo file obrigatório", nil)
		return
	}
	defer file.Close()
	tmpDir := filepath.Join(os.TempDir(), "netquasar-restore")
	_ = os.MkdirAll(tmpDir, 0o755)
	jobID := uuid.NewString()
	localPath := filepath.Join(tmpDir, jobID+"-"+filepath.Base(hdr.Filename))
	out, err := os.Create(localPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TEMP", err.Error(), nil)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		_ = os.Remove(localPath)
		writeErr(w, http.StatusInternalServerError, "TEMP", err.Error(), nil)
		return
	}
	out.Close()

	job := &dbRestoreJob{ID: jobID, Status: "running", ProgressPct: 5, CurrentStep: "Upload recebido", StartedAt: time.Now()}
	s.dbRestoreMu.Lock()
	s.dbRestoreJobs[jobID] = job
	s.dbRestoreMu.Unlock()
	go s.runRestoreJob(jobID, localPath, true)
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "status": "running"})
}

func (s *Server) restoreDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	s.ensureDBRestoreJobs()
	var body struct {
		Source   string `json:"source"`
		FileName string `json:"file_name"`
		Confirm  string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if strings.TrimSpace(body.Confirm) != "RESTORE" {
		writeErr(w, http.StatusUnprocessableEntity, "CONFIRM", `envie confirm=RESTORE`, nil)
		return
	}
	if body.Source != "b2" || strings.TrimSpace(body.FileName) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "source=b2 e file_name obrigatórios", nil)
		return
	}
	jobID := uuid.NewString()
	job := &dbRestoreJob{ID: jobID, Status: "running", ProgressPct: 5, CurrentStep: "A descarregar do B2…", StartedAt: time.Now()}
	s.dbRestoreMu.Lock()
	s.dbRestoreJobs[jobID] = job
	s.dbRestoreMu.Unlock()
	go func() {
		tmpDir := filepath.Join(os.TempDir(), "netquasar-restore")
		_ = os.MkdirAll(tmpDir, 0o755)
		localPath := filepath.Join(tmpDir, jobID+"-"+filepath.Base(body.FileName))
		creds, err := s.loadB2Creds(context.Background())
		if err != nil {
			s.finishRestoreJob(jobID, false, err.Error())
			return
		}
		cli, err := backupb2.NewClient(context.Background(), creds)
		if err != nil {
			s.finishRestoreJob(jobID, false, err.Error())
			return
		}
		remote := body.FileName
		if !strings.Contains(remote, "/") {
			remote = backupb2.ObjectKeyForDump(creds.Prefix, remote)
		}
		s.setRestoreStep(jobID, 20, "Download B2")
		if err := cli.DownloadToFile(context.Background(), remote, localPath); err != nil {
			s.finishRestoreJob(jobID, false, err.Error())
			return
		}
		s.runRestoreJob(jobID, localPath, true)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "status": "running"})
}

func (s *Server) getDatabaseRestoreJob(w http.ResponseWriter, r *http.Request) {
	s.ensureDBRestoreJobs()
	jobID := chi.URLParam(r, "jobId")
	s.dbRestoreMu.Lock()
	job, ok := s.dbRestoreJobs[jobID]
	s.dbRestoreMu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "job não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) setRestoreStep(jobID string, pct int, step string) {
	s.dbRestoreMu.Lock()
	if j, ok := s.dbRestoreJobs[jobID]; ok {
		j.ProgressPct = pct
		j.CurrentStep = step
	}
	s.dbRestoreMu.Unlock()
}

func (s *Server) finishRestoreJob(jobID string, ok bool, errMsg string) {
	now := time.Now()
	s.dbRestoreMu.Lock()
	if j, exists := s.dbRestoreJobs[jobID]; exists {
		j.FinishedAt = &now
		if ok {
			j.Status = "ok"
			j.ProgressPct = 100
			j.CurrentStep = "Concluído"
		} else {
			j.Status = "error"
			j.Error = errMsg
			j.CurrentStep = "Falhou"
		}
	}
	s.dbRestoreMu.Unlock()
}

func (s *Server) runRestoreJob(jobID, dumpPath string, removeAfter bool) {
	ctx := context.Background()
	if removeAfter {
		defer os.Remove(dumpPath)
	}
	s.setRestoreStep(jobID, 30, "A obter DSN…")
	dsn, err := s.activeDatabaseDSN(ctx)
	if err != nil {
		s.finishRestoreJob(jobID, false, err.Error())
		return
	}
	s.setRestoreStep(jobID, 40, "A limpar schema public…")
	if err := backupb2.WipePublicSchema(ctx, dsn); err != nil {
		s.finishRestoreJob(jobID, false, err.Error())
		return
	}
	s.setRestoreStep(jobID, 55, "pg_restore…")
	if err := backupb2.RestoreFull(ctx, dsn, dumpPath); err != nil {
		s.finishRestoreJob(jobID, false, err.Error())
		return
	}
	cfg := config.ConfigFromPostgresDSN(dsn)
	s.setRestoreStep(jobID, 75, "A aplicar migrações…")
	if err := db.Migrate(ctx, cfg); err != nil {
		s.finishRestoreJob(jobID, false, "migrações após restore: "+err.Error())
		return
	}
	s.setRestoreStep(jobID, 85, "A reabrir pool…")
	newPool, err := db.NewPool(ctx, cfg)
	if err != nil {
		s.finishRestoreJob(jobID, false, "pool após restore: "+err.Error())
		return
	}
	if err := bootstrap.AssertWritablePostgres(ctx, newPool); err != nil {
		newPool.Close()
		s.finishRestoreJob(jobID, false, err.Error())
		return
	}
	if s.DBHolder != nil {
		old := s.DBHolder.Swap(newPool)
		if old != nil {
			go func() {
				time.Sleep(2 * time.Second)
				old.Close()
			}()
		}
	} else {
		newPool.Close()
	}
	s.appendAuditLog(ctx, "database_restore", jobID, "restore", auditActorSistema, nil, map[string]any{"dump": filepath.Base(dumpPath)})
	s.finishRestoreJob(jobID, true, "")
}
