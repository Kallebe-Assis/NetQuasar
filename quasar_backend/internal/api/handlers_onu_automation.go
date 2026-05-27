package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/rs/zerolog"
)

type onuAutomationConfig struct {
	Enabled       bool
	Mode          string
	DayOfMonth    int
	TimeHHMM      string
	Timezone      string
	LastRunPeriod *string
	LastRunAt     *time.Time
	LastStatus    *string
	Running       bool
}

func (s *Server) ensureAutomationONUScheduler() {
	s.automationONUOnce.Do(func() {
		ctx := s.WorkerCtx
		if ctx == nil {
			ctx = context.Background()
		}
		go s.runAutomationONUScheduler(ctx)
	})
}

func (s *Server) runAutomationONUScheduler(ctx context.Context) {
	l := s.Log.With().Str("component", "onu_monthly_scheduler").Logger()
	check := func(trigger string) {
		l.Debug().Str("trigger", trigger).Msg("verificação agendamento relatório ONU")
		s.tryScheduledONUReport(ctx, &l)
	}
	// Ao arranque: recupera execuções perdidas se o serviço estava parado na hora agendada.
	check("startup")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("agendador relatório ONU encerrado")
			return
		case <-ticker.C:
			check("minute")
		}
	}
}

func (s *Server) tryScheduledONUReport(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	s.clearStaleONUAutomationRunning(ctx, pool)
	cfg, err := loadONUAutomationConfig(ctx, pool)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("ler agendamento ONU")
		}
		return
	}
	period, due := onuReportDue(cfg, time.Now())
	if !due {
		if log != nil {
			log.Debug().
				Bool("enabled", cfg.Enabled).
				Str("mode", cfg.Mode).
				Str("period", period).
				Str("last_run_period", strOpt(cfg.LastRunPeriod)).
				Bool("running", cfg.Running).
				Msg("relatório ONU: ainda não devido")
		}
		return
	}
	if log != nil {
		log.Info().Str("period", period).Str("time_hhmm", cfg.TimeHHMM).Int("day_of_month", cfg.DayOfMonth).Msg("relatório ONU mensal devido — a executar")
	}
	if err := s.executeONUMonthlyReport(ctx, period, "scheduler", true); err != nil && log != nil {
		log.Warn().Err(err).Str("period", period).Msg("relatório ONU agendado falhou")
	}
}

func (s *Server) clearStaleONUAutomationRunning(ctx context.Context, pool *pgxpool.Pool) {
	_, _ = pool.Exec(ctx, `
		UPDATE automation_onu_report SET running = false, updated_at = now()
		WHERE id = 1 AND running = true AND updated_at < now() - interval '2 hours'
	`)
}

func strOpt(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func loadONUAutomationConfig(ctx context.Context, pool *pgxpool.Pool) (onuAutomationConfig, error) {
	var c onuAutomationConfig
	var dom *int
	var lrp, ls *string
	var lra *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, COALESCE(NULLIF(trim(mode), ''), 'monthly'), day_of_month, time_hhmm, timezone,
			last_run_period, last_run_at, last_status, running
		FROM automation_onu_report WHERE id=1
	`).Scan(&c.Enabled, &c.Mode, &dom, &c.TimeHHMM, &c.Timezone, &lrp, &lra, &ls, &c.Running)
	if err != nil {
		return c, err
	}
	if dom != nil && *dom >= 1 && *dom <= 31 {
		c.DayOfMonth = *dom
	} else {
		c.DayOfMonth = 1
	}
	c.LastRunPeriod = lrp
	c.LastRunAt = lra
	c.LastStatus = ls
	if strings.TrimSpace(c.Timezone) == "" {
		c.Timezone = "America/Sao_Paulo"
	}
	if strings.TrimSpace(c.TimeHHMM) == "" {
		c.TimeHHMM = "08:00"
	}
	return c, nil
}

// onuReportDue indica se o relatório do mês (period YYYY-MM) ainda não foi concluído com sucesso e já passou o dia/hora agendados.
func onuReportDue(cfg onuAutomationConfig, now time.Time) (period string, due bool) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if !cfg.Enabled || mode == "disabled" {
		return "", false
	}
	if mode != "" && mode != "monthly" {
		return "", false
	}
	return scheduleutil.MonthlyDue(true, cfg.Timezone, cfg.TimeHHMM, cfg.DayOfMonth, cfg.LastRunPeriod, cfg.LastRunAt, cfg.Running, now)
}

func (s *Server) setONUAutomationStatus(ctx context.Context, status string, errMsg *string) {
	var em any
	if errMsg != nil && strings.TrimSpace(*errMsg) != "" {
		em = *errMsg
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_onu_report SET last_status = $1, last_error = $2, updated_at = now() WHERE id = 1
	`, status, em)
}

func (s *Server) executeONUMonthlyReport(ctx context.Context, period string, actor string, requireEnabled bool) error {
	started := time.Now()
	trigger := "manual"
	if actor == "scheduler" {
		trigger = "scheduled"
	}
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base de dados indisponível")
	}
	q := `UPDATE automation_onu_report SET running = true, updated_at = now() WHERE id = 1 AND running = false`
	if requireEnabled {
		q += ` AND enabled = true`
	}
	tag, err := pool.Exec(ctx, q)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		trigger := "manual"
		if actor == "scheduler" {
			trigger = "scheduled"
		}
		s.recordAutomationExecution(ctx, jobOnuMonthlyReport, actor, trigger, started, false,
			"Não iniciado (desativado, já em execução ou bloqueado)", nil, map[string]any{"period": period}, period)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE automation_onu_report SET running = false, updated_at = now() WHERE id = 1`)
	}()

	s.setONUAutomationStatus(ctx, "collecting", nil)
	s.setMonitoringActivity(ctx, "Relatório ONU mensal: a recolher dados OLT")

	var runID uuid.UUID
	startSummary, _ := json.Marshal(map[string]any{"period": period, "actor": actor})
	if err := pool.QueryRow(ctx, `
		INSERT INTO onu_report_runs (status, summary) VALUES ('collecting', $1::jsonb) RETURNING id
	`, string(startSummary)).Scan(&runID); err != nil {
		s.setONUAutomationStatus(ctx, "failed", strPtr(err.Error()))
		s.recordAutomationExecution(ctx, jobOnuMonthlyReport, actor, trigger, started, false, "Falha ao iniciar execução", err,
			map[string]any{"period": period}, period)
		return err
	}

	oltOK, oltFail := 0, 0
	rows, err := pool.Query(ctx, `SELECT id FROM devices WHERE lower(trim(category)) = 'olt' ORDER BY description`)
	if err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), nil, started, actor, trigger)
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if callInternalDevicePost(ctx, s.refreshOLTDevice, "id", id) == nil {
			oltOK++
		} else {
			oltFail++
		}
	}

	agg, err := s.aggregateONUReport(ctx)
	if err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), map[string]any{"olts_refreshed": oltOK, "olts_failed": oltFail}, started, actor, trigger)
		return err
	}
	agg["period"] = period
	agg["olts_refreshed"] = oltOK
	agg["olts_failed"] = oltFail

	upserted, syncErr := s.syncCommercialMonthlyFromOLTSnapshots(ctx, period)
	agg["commercial_localities_upserted"] = upserted
	if syncErr != nil {
		s.finishONURun(ctx, runID, period, false, syncErr.Error(), agg, started, actor, trigger)
		return syncErr
	}

	s.setONUAutomationStatus(ctx, "sending_telegram", nil)
	s.setMonitoringActivity(ctx, "Relatório ONU mensal: a enviar Telegram")

	cfg, err := telegramclient.LoadConfig(ctx, pool, "reports")
	if err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), agg, started, actor, trigger)
		return err
	}
	if !cfg.Ready() {
		err := fmt.Errorf("Telegram (relatórios) não configurado")
		s.finishONURun(ctx, runID, period, false, err.Error(), agg, started, actor, trigger)
		return err
	}
	text, compErr := s.commercialTelegramCompose(ctx, period, true)
	if compErr != nil {
		s.finishONURun(ctx, runID, period, false, compErr.Error(), agg, started, actor, trigger)
		return compErr
	}
	if err := telegramclient.SendMessageWithParseMode(ctx, cfg, text, "HTML"); err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), agg, started, actor, trigger)
		return err
	}

	s.finishONURun(ctx, runID, period, true, "", agg, started, actor, trigger)
	s.appendAuditLog(ctx, "automation_onu_report", "1", "run", actor, nil, agg)
	return nil
}

func (s *Server) finishONURun(ctx context.Context, runID uuid.UUID, period string, ok bool, errMsg string, agg map[string]any, started time.Time, actor, trigger string) {
	status := "completed"
	stAuto := "completed"
	var em *string
	if !ok {
		status = "failed"
		stAuto = "failed"
		if strings.TrimSpace(errMsg) != "" {
			em = &errMsg
		}
	}
	if agg == nil {
		agg = map[string]any{}
	}
	agg["period"] = period
	agg["telegram_sent"] = ok
	agg["onu_report_run_id"] = runID.String()
	if !ok && errMsg != "" {
		agg["error"] = errMsg
	}
	sb, _ := json.Marshal(agg)
	_, _ = s.DB().Exec(ctx, `
		UPDATE onu_report_runs SET finished_at = now(), status = $2, error_message = $3, summary = $4::jsonb WHERE id = $1
	`, runID, status, em, string(sb))
	if ok {
		_, _ = s.DB().Exec(ctx, `
			UPDATE automation_onu_report SET
				last_run_at = now(),
				last_run_period = $1,
				last_status = $2,
				last_error = NULL,
				running = false,
				updated_at = now()
			WHERE id = 1
		`, period, stAuto)
	} else {
		_, _ = s.DB().Exec(ctx, `
			UPDATE automation_onu_report SET
				last_run_at = now(),
				last_status = $2,
				last_error = $3,
				running = false,
				updated_at = now()
			WHERE id = 1
		`, stAuto, em)
	}
	s.setMonitoringActivity(ctx, "")
	var execErr error
	if !ok && strings.TrimSpace(errMsg) != "" {
		execErr = fmt.Errorf("%s", errMsg)
	}
	statusMsg := fmt.Sprintf("Relatório ONU %s concluído", period)
	if !ok {
		statusMsg = fmt.Sprintf("Relatório ONU %s falhou", period)
	}
	s.recordAutomationExecution(ctx, jobOnuMonthlyReport, actor, trigger, started, ok, statusMsg, execErr, agg, period)
}

func (s *Server) aggregateONUReport(ctx context.Context) (map[string]any, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT d.description, o.pons::text
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
		ORDER BY d.description
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var oltLines []map[string]any
	totalONU, totalOnline := 0, 0
	for rows.Next() {
		var desc, ponsRaw string
		if err := rows.Scan(&desc, &ponsRaw); err != nil {
			continue
		}
		comp := oltparse.SnapshotComputed(nil, []byte(ponsRaw))
		oltTotal := toInt(comp["onu_total_sum"])
		oltOn := toInt(comp["onu_online_sum"])
		oltOff := toInt(comp["onu_offline_sum"])
		ponN := toInt(comp["pon_count"])
		oltLines = append(oltLines, map[string]any{
			"olt": desc, "pons": ponN, "onu_total": oltTotal, "onu_online": oltOn, "onu_offline": oltOff,
		})
		totalONU += oltTotal
		totalOnline += oltOn
	}
	totalOffline := totalONU - totalOnline
	if totalOffline < 0 {
		totalOffline = 0
	}
	return map[string]any{
		"olt_count":   len(oltLines),
		"onu_total":   totalONU,
		"onu_online":  totalOnline,
		"onu_offline": totalOffline,
		"olt_lines":   oltLines,
	}, nil
}

func onuReportPeriodNow(tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01")
}

func (s *Server) getAutomationONU(w http.ResponseWriter, r *http.Request) {
	row := s.DB().QueryRow(r.Context(), `
		SELECT enabled, mode, day_of_month, day_of_week, time_hhmm, timezone,
			last_run_at, last_run_period, last_status, last_error, running
		FROM automation_onu_report WHERE id=1
	`)
	var en, running bool
	var mode, th, tz string
	var dom, dow *int
	var lr *time.Time
	var lrp, ls, le *string
	if err := row.Scan(&en, &mode, &dom, &dow, &th, &tz, &lr, &lrp, &ls, &le, &running); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": en, "mode": mode, "day_of_month": dom, "day_of_week": dow,
		"time_hhmm": th, "timezone": tz, "last_run_at": lr, "last_run_period": lrp,
		"last_status": ls, "last_error": le, "running": running,
	})
}

func (s *Server) patchAutomationONU(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled    *bool   `json:"enabled"`
		DayOfMonth *int    `json:"day_of_month"`
		TimeHHMM   *string `json:"time_hhmm"`
		Timezone   *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var modeArg any
	if body.Enabled != nil {
		if *body.Enabled {
			modeArg = "monthly"
		} else {
			modeArg = "disabled"
		}
	}
	if body.DayOfMonth != nil && (*body.DayOfMonth < 1 || *body.DayOfMonth > 31) {
		writeErr(w, 422, "VALIDATION", "day_of_month entre 1 e 31", nil)
		return
	}
	if body.TimeHHMM != nil {
		if _, _, ok := strings.Cut(strings.TrimSpace(*body.TimeHHMM), ":"); !ok {
			writeErr(w, 422, "VALIDATION", "time_hhmm inválido (use HH:MM)", nil)
			return
		}
	}
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	resetLast := body.DayOfMonth != nil || body.TimeHHMM != nil || body.Timezone != nil
	_, err = tx.Exec(r.Context(), `
		UPDATE automation_onu_report SET
			enabled = COALESCE($1, enabled),
			mode = COALESCE($2, mode),
			day_of_month = COALESCE($3, day_of_month),
			time_hhmm = COALESCE($4, time_hhmm),
			timezone = COALESCE($5, timezone),
			last_run_period = CASE WHEN $6 THEN NULL ELSE last_run_period END,
			last_run_at = CASE WHEN $6 THEN NULL ELSE last_run_at END,
			running = CASE WHEN $6 THEN false ELSE running END,
			updated_at = now()
		WHERE id = 1
	`, body.Enabled, modeArg, body.DayOfMonth, body.TimeHHMM, body.Timezone, resetLast)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_onu_report", "1", "patch", actorFromRequest(r), nil, body)
	s.getAutomationONU(w, r)
}

func (s *Server) runAutomationONU(w http.ResponseWriter, r *http.Request) {
	pool := s.DB()
	if pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "DB", "base de dados indisponível", nil)
		return
	}
	var running bool
	if err := pool.QueryRow(r.Context(), `SELECT running FROM automation_onu_report WHERE id=1`).Scan(&running); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if running {
		writeErr(w, http.StatusConflict, "BUSY", "Relatório ONU já em execução", nil)
		return
	}
	var tz string
	_ = pool.QueryRow(r.Context(), `SELECT timezone FROM automation_onu_report WHERE id=1`).Scan(&tz)
	period := onuReportPeriodNow(tz)
	actor := actorFromRequest(r)
	s.appendAuditLog(r.Context(), "automation_onu_report", "1", "run_manual", actor, nil, map[string]any{"period": period})
	go func() {
		bg := context.Background()
		if err := s.executeONUMonthlyReport(bg, period, actor, false); err != nil {
			s.Log.Warn().Err(err).Str("period", period).Msg("relatório ONU manual falhou")
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "period": period})
}

func (s *Server) listAutomationRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, started_at, finished_at, status, error_message, summary
		FROM onu_report_runs ORDER BY started_at DESC LIMIT 100
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var runs []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var st, em *string
		var started time.Time
		var finished *time.Time
		var sum []byte
		if err := rows.Scan(&id, &started, &finished, &st, &em, &sum); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		var sm any
		if len(sum) > 0 {
			_ = json.Unmarshal(sum, &sm)
		}
		runs = append(runs, map[string]any{
			"id": id, "started_at": started, "finished_at": finished, "status": st, "error_message": em, "summary": sm,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}
