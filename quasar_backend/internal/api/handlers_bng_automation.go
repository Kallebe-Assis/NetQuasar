package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/mailclient"
	"github.com/netquasar/netquasar/quasar_backend/internal/reporttelegram"
	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/rs/zerolog"
)

func (s *Server) tryScheduledBngStatsReport(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	s.clearStaleAutomationRunning(ctx, "automation_bng_stats_report")
	var en, running, tg, em bool
	var freq, th, tz, lastKey, emailTo *string
	var dow *int
	var lr *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_key, last_run_at, running
		FROM automation_bng_stats_report WHERE id = 1
	`).Scan(&en, &freq, &dow, &th, &tz, &tg, &em, &emailTo, &lastKey, &lr, &running)
	if err != nil || !en {
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
	thStr := "08:00"
	if th != nil {
		thStr = *th
	}
	runKey, due := scheduleutil.DailyWeeklyDue(en, frequency, tzStr, thStr, dow, lastKey, lr, running, time.Now())
	if !due {
		return
	}
	if err := s.executeBngStatsReport(ctx, runKey, auditActorSistema); err != nil && log != nil {
		log.Warn().Err(err).Str("run_key", runKey).Msg("relatório BNG agendado falhou")
	}
}

func (s *Server) executeBngStatsReport(ctx context.Context, runKey, actor string) error {
	started := time.Now()
	trigger := "manual"
	if actor == auditActorSistema {
		trigger = "scheduled"
	}
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE automation_bng_stats_report SET running = true, updated_at = now()
		WHERE id = 1 AND running = false AND enabled = true
	`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		sum := s.bngStatsReportSummary(ctx)
		sum["run_key"] = runKey
		s.recordAutomationExecution(ctx, jobBngStatsReport, actor, trigger, started, false,
			"Não iniciado (desativado, já em execução ou bloqueado)", nil, sum, runKey)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE automation_bng_stats_report SET running = false, updated_at = now() WHERE id = 1`)
	}()

	var tg, em bool
	var emailTo *string
	_ = pool.QueryRow(ctx, `SELECT channel_telegram, channel_email, email_to FROM automation_bng_stats_report WHERE id = 1`).
		Scan(&tg, &em, &emailTo)

	payload, err := s.buildSystemReport(ctx, "bng-subscribers", systemReportOptions{})
	if err != nil {
		s.setBngStatsReportStatus(ctx, "failed", strPtr(err.Error()), runKey)
		sum := s.bngStatsReportSummary(ctx)
		s.recordAutomationExecution(ctx, jobBngStatsReport, actor, trigger, started, false, "Falha ao compor relatório", err, sum, runKey)
		return err
	}
	title, _ := payload["title"].(string)
	text := reporttelegram.ComposeSystemReport(title, payload)

	var sendErr error
	if tg {
		cfg, err := telegramclient.LoadConfig(ctx, pool, "reports")
		if err != nil || !cfg.Ready() {
			sendErr = fmt.Errorf("Telegram relatórios: %w", err)
		} else if err := telegramclient.SendMessage(ctx, cfg, text); err != nil {
			sendErr = err
		}
	}
	if em && sendErr == nil {
		smtpCfg, err := mailclient.LoadConfig(ctx, pool)
		if err != nil || !smtpCfg.Ready() {
			sendErr = fmt.Errorf("SMTP: %w", err)
		} else {
			subject := "NetQuasar — Totais BNG"
			if strings.TrimSpace(title) != "" {
				subject = "NetQuasar — " + title
			}
			to := mailclient.ParseRecipients(ptrStr(emailTo))
			if err := mailclient.Send(ctx, smtpCfg, to, subject, text); err != nil {
				sendErr = err
			}
		}
	}
	if !tg && !em {
		sendErr = fmt.Errorf("nenhum canal ativo")
	}
	if sendErr != nil {
		s.setBngStatsReportStatus(ctx, "failed", strPtr(sendErr.Error()), runKey)
		sum := s.bngStatsReportSummary(ctx)
		s.recordAutomationExecution(ctx, jobBngStatsReport, actor, trigger, started, false, "Falha no envio", sendErr, sum, runKey)
		return sendErr
	}
	s.setBngStatsReportStatus(ctx, "completed", nil, runKey)
	sum := s.bngStatsReportSummary(ctx)
	s.recordAutomationExecution(ctx, jobBngStatsReport, actor, trigger, started, true,
		"Relatório de totais BNG enviado", nil, sum, runKey)
	s.appendAuditLog(ctx, "automation_bng_stats_report", "1", "run", actor, nil, map[string]any{"run_key": runKey})
	return nil
}

func (s *Server) bngStatsReportSummary(ctx context.Context) map[string]any {
	pool := s.DB()
	if pool == nil {
		return nil
	}
	var devices, withSamples int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE coalesce(bng_enabled, false) = true`).Scan(&devices)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT device_id) FROM bng_stats_samples b
		JOIN devices d ON d.id = b.device_id AND coalesce(d.bng_enabled, false) = true
		WHERE b.collected_at >= now() - interval '7 days'
	`).Scan(&withSamples)
	return map[string]any{
		"bng_devices":        devices,
		"devices_with_stats": withSamples,
	}
}

func (s *Server) setBngStatsReportStatus(ctx context.Context, status string, errMsg *string, runKey string) {
	var em any
	if errMsg != nil {
		em = *errMsg
	}
	if status == "completed" {
		_, _ = s.DB().Exec(ctx, `
			UPDATE automation_bng_stats_report SET
				last_status = $1, last_error = NULL, last_run_key = $2, last_run_at = now(), updated_at = now()
			WHERE id = 1
		`, status, runKey)
		return
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_bng_stats_report SET
			last_status = $1, last_error = $2, last_run_at = now(), updated_at = now()
		WHERE id = 1
	`, status, em)
}

func (s *Server) getAutomationBngStatsReport(w http.ResponseWriter, r *http.Request) {
	var en, running, tg, em bool
	var freq, th, tz, emailTo, lastKey, ls, le *string
	var dow *int
	var lr *time.Time
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_at, last_run_key, last_status, last_error, running
		FROM automation_bng_stats_report WHERE id = 1
	`).Scan(&en, &freq, &dow, &th, &tz, &tg, &em, &emailTo, &lr, &lastKey, &ls, &le, &running)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": en, "frequency": freq, "day_of_week": dow, "time_hhmm": th, "timezone": tz,
		"channel_telegram": tg, "channel_email": em, "email_to": emailTo,
		"last_run_at": lr, "last_run_key": lastKey, "last_status": ls, "last_error": le, "running": running,
	})
}

func (s *Server) patchAutomationBngStatsReport(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if err := patchAutomationSchedule(r.Context(), s.DB(), "automation_bng_stats_report", body); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_bng_stats_report", "1", "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationBngStatsReport(w http.ResponseWriter, r *http.Request) {
	runKey := time.Now().Format("2006-01-02")
	go func() {
		_ = s.executeBngStatsReport(context.Background(), runKey, s.actorFromRequest(r))
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "run_key": runKey})
}
