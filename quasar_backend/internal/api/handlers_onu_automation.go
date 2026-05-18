package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
	LastStatus    *string
	Running       bool
}

func (s *Server) ensureAutomationONUScheduler() {
	if s.WorkerCtx == nil {
		return
	}
	s.automationONUOnce.Do(func() {
		go s.runAutomationONUScheduler(s.WorkerCtx)
	})
}

func (s *Server) runAutomationONUScheduler(ctx context.Context) {
	l := s.Log.With().Str("component", "onu_monthly_scheduler").Logger()
	s.tryScheduledONUReport(ctx, &l)
	align := time.Until(time.Now().Truncate(time.Hour).Add(time.Hour))
	if align > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(align):
		}
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("agendador relatório ONU encerrado")
			return
		case <-ticker.C:
			s.tryScheduledONUReport(ctx, &l)
		}
	}
}

func (s *Server) tryScheduledONUReport(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	cfg, err := loadONUAutomationConfig(ctx, pool)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("ler agendamento ONU")
		}
		return
	}
	period, due := onuReportDue(cfg, time.Now())
	if !due {
		return
	}
	if err := s.executeONUMonthlyReport(ctx, period, "scheduler", true); err != nil && log != nil {
		log.Warn().Err(err).Str("period", period).Msg("relatório ONU agendado falhou")
	}
}

func loadONUAutomationConfig(ctx context.Context, pool *pgxpool.Pool) (onuAutomationConfig, error) {
	var c onuAutomationConfig
	var dom *int
	var lrp, ls *string
	err := pool.QueryRow(ctx, `
		SELECT enabled, mode, day_of_month, time_hhmm, timezone,
			last_run_period, last_status, running
		FROM automation_onu_report WHERE id=1
	`).Scan(&c.Enabled, &c.Mode, &dom, &c.TimeHHMM, &c.Timezone, &lrp, &ls, &c.Running)
	if err != nil {
		return c, err
	}
	if dom != nil && *dom >= 1 && *dom <= 31 {
		c.DayOfMonth = *dom
	} else {
		c.DayOfMonth = 1
	}
	c.LastRunPeriod = lrp
	c.LastStatus = ls
	if strings.TrimSpace(c.Timezone) == "" {
		c.Timezone = "America/Sao_Paulo"
	}
	if strings.TrimSpace(c.TimeHHMM) == "" {
		c.TimeHHMM = "08:00"
	}
	return c, nil
}

// onuReportDue indica se o relatório do mês (period YYYY-MM) ainda não foi feito e já passou o dia/hora agendados.
func onuReportDue(cfg onuAutomationConfig, now time.Time) (period string, due bool) {
	if !cfg.Enabled {
		return "", false
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	period = now.Format("2006-01")
	if cfg.LastRunPeriod != nil && strings.TrimSpace(*cfg.LastRunPeriod) == period {
		return period, false
	}
	if cfg.Running {
		return period, false
	}
	day := effectiveDOM(cfg.DayOfMonth, now.Year(), int(now.Month()))
	hour, min := parseHHMM(cfg.TimeHHMM)
	scheduled := time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, loc)
	if now.Before(scheduled) {
		return period, false
	}
	return period, true
}

func effectiveDOM(dom, year, month int) int {
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
	if dom > last {
		return last
	}
	if dom < 1 {
		return 1
	}
	return dom
}

func parseHHMM(hhmm string) (hour, min int) {
	parts := strings.Split(strings.TrimSpace(hhmm), ":")
	if len(parts) >= 1 {
		hour, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	if len(parts) >= 2 {
		min, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	if hour < 0 || hour > 23 {
		hour = 8
	}
	if min < 0 || min > 59 {
		min = 0
	}
	return hour, min
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
		return err
	}

	oltOK, oltFail := 0, 0
	rows, err := pool.Query(ctx, `SELECT id FROM devices WHERE lower(trim(category)) = 'olt' ORDER BY description`)
	if err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), nil)
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
		s.finishONURun(ctx, runID, period, false, err.Error(), map[string]any{"olts_refreshed": oltOK, "olts_failed": oltFail})
		return err
	}
	agg["period"] = period
	agg["olts_refreshed"] = oltOK
	agg["olts_failed"] = oltFail

	upserted, syncErr := s.syncCommercialMonthlyFromOLTSnapshots(ctx, period)
	agg["commercial_localities_upserted"] = upserted
	if syncErr != nil {
		s.finishONURun(ctx, runID, period, false, syncErr.Error(), agg)
		return syncErr
	}

	s.setONUAutomationStatus(ctx, "sending_telegram", nil)
	s.setMonitoringActivity(ctx, "Relatório ONU mensal: a enviar Telegram")

	cfg, err := telegramclient.LoadConfig(ctx, pool, "reports")
	if err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), agg)
		return err
	}
	if !cfg.Ready() {
		err := fmt.Errorf("Telegram (relatórios) não configurado")
		s.finishONURun(ctx, runID, period, false, err.Error(), agg)
		return err
	}
	text, compErr := s.commercialTelegramCompose(ctx, period, true)
	if compErr != nil {
		s.finishONURun(ctx, runID, period, false, compErr.Error(), agg)
		return compErr
	}
	if err := telegramclient.SendMessageWithParseMode(ctx, cfg, text, "HTML"); err != nil {
		s.finishONURun(ctx, runID, period, false, err.Error(), agg)
		return err
	}

	s.finishONURun(ctx, runID, period, true, "", agg)
	s.appendAuditLog(ctx, "automation_onu_report", "1", "run", actor, nil, agg)
	return nil
}

func (s *Server) finishONURun(ctx context.Context, runID uuid.UUID, period string, ok bool, errMsg string, agg map[string]any) {
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
	if !ok && errMsg != "" {
		agg["error"] = errMsg
	}
	sb, _ := json.Marshal(agg)
	_, _ = s.DB().Exec(ctx, `
		UPDATE onu_report_runs SET finished_at = now(), status = $2, error_message = $3, summary = $4::jsonb WHERE id = $1
	`, runID, status, em, string(sb))
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_onu_report SET
			last_run_at = now(),
			last_run_period = $1,
			last_status = $2,
			last_error = $3,
			running = false,
			updated_at = now()
		WHERE id = 1
	`, period, stAuto, em)
	s.setMonitoringActivity(ctx, "")
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
		var pons []map[string]any
		_ = json.Unmarshal([]byte(ponsRaw), &pons)
		oltTotal, oltOn := 0, 0
		for _, p := range pons {
			oltTotal += toInt(p["onu_total"])
			oltOn += toInt(p["onu_online"])
		}
		oltLines = append(oltLines, map[string]any{
			"olt": desc, "pons": len(pons), "onu_total": oltTotal, "onu_online": oltOn,
		})
		totalONU += oltTotal
		totalOnline += oltOn
	}
	return map[string]any{
		"olt_count":   len(oltLines),
		"onu_total":   totalONU,
		"onu_online":  totalOnline,
		"onu_offline": totalONU - totalOnline,
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
	_, err = tx.Exec(r.Context(), `
		UPDATE automation_onu_report SET
			enabled = COALESCE($1, enabled),
			mode = COALESCE($2, mode),
			day_of_month = COALESCE($3, day_of_month),
			time_hhmm = COALESCE($4, time_hhmm),
			timezone = COALESCE($5, timezone),
			updated_at = now()
		WHERE id = 1
	`, body.Enabled, modeArg, body.DayOfMonth, body.TimeHHMM, body.Timezone)
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
