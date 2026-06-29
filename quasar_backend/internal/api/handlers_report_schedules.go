package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertcorrelation"
	"github.com/netquasar/netquasar/quasar_backend/internal/mailclient"
	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/rs/zerolog"
)

func (s *Server) ensureReportSchedulers() {
	s.automationReportsOnce.Do(func() {
		ctx := s.WorkerCtx
		if ctx == nil {
			ctx = context.Background()
		}
		go s.runReportSchedulersLoop(ctx)
	})
}

func (s *Server) runReportSchedulersLoop(ctx context.Context) {
	l := s.Log.With().Str("component", "report_schedulers").Logger()
	runScheduled := func(trigger string) {
		l.Debug().Str("trigger", trigger).Msg("verificação relatórios agendados (alertas/comercial)")
		s.tryScheduledAlertsDigest(ctx, &l)
		s.tryScheduledCommercialReport(ctx, &l)
		s.tryScheduledBngStatsReport(ctx, &l)
	}
	corr := time.NewTicker(5 * time.Minute)
	defer corr.Stop()
	alertcorrelation.Reconcile(ctx, s.DB(), &l)
	runScheduled("startup")
	minute := time.NewTicker(30 * time.Second)
	defer minute.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("agendadores de relatórios encerrados")
			return
		case <-corr.C:
			alertcorrelation.Reconcile(ctx, s.DB(), &l)
		case <-minute.C:
			runScheduled("minute")
		}
	}
}

func (s *Server) clearStaleAutomationRunning(ctx context.Context, table string) {
	pool := s.DB()
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE `+table+` SET running = false, updated_at = now()
		WHERE id = 1 AND running = true AND updated_at < now() - interval '30 minutes'
	`)
}

func (s *Server) tryScheduledAlertsDigest(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	s.clearStaleAutomationRunning(ctx, "automation_alerts_digest")
	var en, running, tg, em bool
	var freq, th, tz, lastKey, emailTo *string
	var dow *int
	var lr *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_key, last_run_at, running
		FROM automation_alerts_digest WHERE id = 1
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
	thStr := "07:30"
	if th != nil {
		thStr = *th
	}
	runKey, due := scheduleutil.DailyWeeklyDue(en, frequency, tzStr, thStr, dow, lastKey, lr, running, time.Now())
	if !due {
		return
	}
	if err := s.executeAlertsDigest(ctx, runKey, auditActorSistema); err != nil && log != nil {
		log.Warn().Err(err).Str("run_key", runKey).Msg("resumo de alertas agendado falhou")
	}
}

func (s *Server) tryScheduledCommercialReport(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	s.clearStaleAutomationRunning(ctx, "automation_commercial_report")
	var en, running, tg, em bool
	var dom *int
	var th, tz, lastPeriod, emailTo *string
	var lr *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, day_of_month, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_period, last_run_at, running
		FROM automation_commercial_report WHERE id = 1
	`).Scan(&en, &dom, &th, &tz, &tg, &em, &emailTo, &lastPeriod, &lr, &running)
	if err != nil || !en {
		return
	}
	domVal := 1
	if dom != nil {
		domVal = *dom
	}
	tzStr := "America/Sao_Paulo"
	if tz != nil && strings.TrimSpace(*tz) != "" {
		tzStr = *tz
	}
	thStr := "09:00"
	if th != nil {
		thStr = *th
	}
	period, due := scheduleutil.MonthlyDue(en, tzStr, thStr, domVal, lastPeriod, lr, running, time.Now())
	if !due {
		return
	}
	if err := s.executeCommercialReportOnly(ctx, period, auditActorSistema); err != nil && log != nil {
		log.Warn().Err(err).Str("period", period).Msg("relatório comercial agendado falhou")
	}
}

func (s *Server) executeAlertsDigest(ctx context.Context, runKey, actor string) error {
	started := time.Now()
	trigger := "manual"
	if actor == "scheduler" {
		trigger = "scheduled"
	}
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE automation_alerts_digest SET running = true, updated_at = now()
		WHERE id = 1 AND running = false AND enabled = true
	`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		sum := s.alertsDigestSummary(ctx)
		sum["run_key"] = runKey
		s.recordAutomationExecution(ctx, jobAlertsDigest, actor, trigger, started, false,
			"Não iniciado (desativado, já em execução ou bloqueado)", nil, sum, runKey)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE automation_alerts_digest SET running = false, updated_at = now() WHERE id = 1`)
	}()

	var tg, em bool
	var emailTo *string
	_ = pool.QueryRow(ctx, `SELECT channel_telegram, channel_email, email_to FROM automation_alerts_digest WHERE id = 1`).
		Scan(&tg, &em, &emailTo)

	subject, body, err := s.composeAlertsDigest(ctx)
	if err != nil {
		s.setAlertsDigestStatus(ctx, "failed", strPtr(err.Error()), runKey)
		sum := s.alertsDigestSummary(ctx)
		sum["run_key"] = runKey
		s.recordAutomationExecution(ctx, jobAlertsDigest, actor, trigger, started, false, "Falha ao compor resumo", err, sum, runKey)
		return err
	}
	var sendErr error
	if tg {
		cfg, err := telegramclient.LoadConfig(ctx, pool, "reports")
		if err != nil || !cfg.Ready() {
			sendErr = fmt.Errorf("Telegram relatórios: %w", err)
		} else if err := telegramclient.SendMessage(ctx, cfg, body); err != nil {
			sendErr = err
		}
	}
	if em && sendErr == nil {
		smtpCfg, err := mailclient.LoadConfig(ctx, pool)
		if err != nil || !smtpCfg.Ready() {
			sendErr = fmt.Errorf("SMTP: %w", err)
		} else {
			to := mailclient.ParseRecipients(ptrStr(emailTo))
			if err := mailclient.Send(ctx, smtpCfg, to, subject, body); err != nil {
				sendErr = err
			}
		}
	}
	if !tg && !em {
		sendErr = fmt.Errorf("nenhum canal ativo (Telegram ou e-mail)")
	}
	if sendErr != nil {
		s.setAlertsDigestStatus(ctx, "failed", strPtr(sendErr.Error()), runKey)
		sum := s.alertsDigestSummary(ctx)
		sum["run_key"] = runKey
		s.recordAutomationExecution(ctx, jobAlertsDigest, actor, trigger, started, false, "Falha no envio", sendErr, sum, runKey)
		return sendErr
	}
	s.setAlertsDigestStatus(ctx, "completed", nil, runKey)
	sum := s.alertsDigestSummary(ctx)
	sum["run_key"] = runKey
	s.recordAutomationExecution(ctx, jobAlertsDigest, actor, trigger, started, true,
		fmt.Sprintf("Resumo enviado (%s)", subject), nil, sum, runKey)
	s.appendAuditLog(ctx, "automation_alerts_digest", "1", "run", actor, nil, map[string]any{"run_key": runKey})
	return nil
}

func (s *Server) composeAlertsDigest(ctx context.Context) (subject, body string, err error) {
	pool := s.DB()
	if pool == nil {
		return "", "", fmt.Errorf("pool nil")
	}
	var openTotal, openCrit, openWarn, openInfo int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`).Scan(&openTotal)
	_ = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE severity = 'critical'),
			COUNT(*) FILTER (WHERE severity = 'warning'),
			COUNT(*) FILTER (WHERE severity = 'info')
		FROM alert_instances WHERE closed_at IS NULL
	`).Scan(&openCrit, &openWarn, &openInfo)

	var sb strings.Builder
	subject = "Resumo de alertas"
	sb.WriteString(fmt.Sprintf("Alertas abertos: %d\n", openTotal))
	if openCrit+openWarn+openInfo > 0 {
		sb.WriteString(fmt.Sprintf("(críticos %d · aviso %d · info %d)\n", openCrit, openWarn, openInfo))
	}
	sb.WriteString("\n")

	listRows, err := pool.Query(ctx, `
		SELECT COALESCE(NULLIF(trim(device_name), ''), '—'),
			COALESCE(NULLIF(trim(ip), ''), '—'), message
		FROM alert_instances
		WHERE closed_at IS NULL
		ORDER BY
			CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,
			active_since DESC
		LIMIT 80
	`)
	if err == nil {
		for listRows.Next() {
			var name, ipAddr, msg string
			if listRows.Scan(&name, &ipAddr, &msg) == nil {
				detail := strings.TrimSpace(msg)
				if detail == "" {
					detail = "Alerta"
				}
				if len(detail) > 160 {
					detail = detail[:157] + "..."
				}
				sb.WriteString(fmt.Sprintf("%s (%s) - %s\n", name, ipAddr, detail))
			}
		}
		listRows.Close()
	}
	return subject, strings.TrimSpace(sb.String()), nil
}

func (s *Server) executeCommercialReportOnly(ctx context.Context, period, actor string) error {
	started := time.Now()
	trigger := "manual"
	if actor == "scheduler" {
		trigger = "scheduled"
	}
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE automation_commercial_report SET running = true, updated_at = now()
		WHERE id = 1 AND running = false AND enabled = true
	`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		sum := s.commercialReportSummary(ctx, period)
		s.recordAutomationExecution(ctx, jobCommercialReport, actor, trigger, started, false,
			"Não iniciado (desativado, já em execução ou bloqueado)", nil, sum, period)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE automation_commercial_report SET running = false, updated_at = now() WHERE id = 1`)
	}()

	var tg, em bool
	var emailTo *string
	_ = pool.QueryRow(ctx, `SELECT channel_telegram, channel_email, email_to FROM automation_commercial_report WHERE id = 1`).
		Scan(&tg, &em, &emailTo)

	text, err := s.commercialTelegramCompose(ctx, period, true)
	if err != nil {
		s.setCommercialReportStatus(ctx, "failed", strPtr(err.Error()), period)
		sum := s.commercialReportSummary(ctx, period)
		s.recordAutomationExecution(ctx, jobCommercialReport, actor, trigger, started, false, "Falha ao compor relatório", err, sum, period)
		return err
	}
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
			plain := text
			subject := fmt.Sprintf("NetQuasar — Base comercial %s", period)
			to := mailclient.ParseRecipients(ptrStr(emailTo))
			if err := mailclient.Send(ctx, smtpCfg, to, subject, plain); err != nil {
				sendErr = err
			}
		}
	}
	if !tg && !em {
		sendErr = fmt.Errorf("nenhum canal ativo")
	}
	if sendErr != nil {
		s.setCommercialReportStatus(ctx, "failed", strPtr(sendErr.Error()), period)
		sum := s.commercialReportSummary(ctx, period)
		s.recordAutomationExecution(ctx, jobCommercialReport, actor, trigger, started, false, "Falha no envio", sendErr, sum, period)
		return sendErr
	}
	s.setCommercialReportStatus(ctx, "completed", nil, period)
	sum := s.commercialReportSummary(ctx, period)
	s.recordAutomationExecution(ctx, jobCommercialReport, actor, trigger, started, true,
		fmt.Sprintf("Base comercial %s enviada", period), nil, sum, period)
	s.appendAuditLog(ctx, "automation_commercial_report", "1", "run", actor, nil, map[string]any{"period": period})
	return nil
}

func (s *Server) setAlertsDigestStatus(ctx context.Context, status string, errMsg *string, runKey string) {
	var em any
	if errMsg != nil {
		em = *errMsg
	}
	if status == "completed" {
		_, _ = s.DB().Exec(ctx, `
			UPDATE automation_alerts_digest SET
				last_status = $1, last_error = NULL, last_run_key = $2, last_run_at = now(), updated_at = now()
			WHERE id = 1
		`, status, runKey)
		return
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_alerts_digest SET
			last_status = $1, last_error = $2, last_run_at = now(), updated_at = now()
		WHERE id = 1
	`, status, em)
}

func (s *Server) setCommercialReportStatus(ctx context.Context, status string, errMsg *string, period string) {
	var em any
	if errMsg != nil {
		em = *errMsg
	}
	if status == "completed" {
		_, _ = s.DB().Exec(ctx, `
			UPDATE automation_commercial_report SET
				last_status = $1, last_error = NULL, last_run_period = $2, last_run_at = now(), updated_at = now()
			WHERE id = 1
		`, status, period)
		return
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_commercial_report SET
			last_status = $1, last_error = $2, last_run_at = now(), updated_at = now()
		WHERE id = 1
	`, status, em)
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) getAutomationAlertsDigest(w http.ResponseWriter, r *http.Request) {
	var en, running, tg, em bool
	var freq, th, tz, emailTo, lastKey, ls, le *string
	var dow *int
	var lr *time.Time
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, frequency, day_of_week, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_at, last_run_key, last_status, last_error, running
		FROM automation_alerts_digest WHERE id = 1
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

func (s *Server) patchAutomationAlertsDigest(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if err := patchAutomationSchedule(r.Context(), s.DB(), "automation_alerts_digest", body); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_alerts_digest", "1", "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationAlertsDigest(w http.ResponseWriter, r *http.Request) {
	runKey := time.Now().Format("2006-01-02")
	go func() {
		_ = s.executeAlertsDigest(context.Background(), runKey, s.actorFromRequest(r))
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "run_key": runKey})
}

func (s *Server) getAutomationCommercialReport(w http.ResponseWriter, r *http.Request) {
	var en, running, tg, em bool
	var th, tz, emailTo, lastPeriod, ls, le *string
	var dom *int
	var lr *time.Time
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, day_of_month, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_at, last_run_period, last_status, last_error, running
		FROM automation_commercial_report WHERE id = 1
	`).Scan(&en, &dom, &th, &tz, &tg, &em, &emailTo, &lr, &lastPeriod, &ls, &le, &running)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": en, "day_of_month": dom, "time_hhmm": th, "timezone": tz,
		"channel_telegram": tg, "channel_email": em, "email_to": emailTo,
		"last_run_at": lr, "last_run_period": lastPeriod, "last_status": ls, "last_error": le, "running": running,
	})
}

func (s *Server) patchAutomationCommercialReport(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if err := patchAutomationSchedule(r.Context(), s.DB(), "automation_commercial_report", body); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_commercial_report", "1", "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationCommercialReport(w http.ResponseWriter, r *http.Request) {
	var tz string
	_ = s.DB().QueryRow(r.Context(), `SELECT timezone FROM automation_commercial_report WHERE id=1`).Scan(&tz)
	period := onuReportPeriodNow(tz)
	go func() {
		_ = s.executeCommercialReportOnly(context.Background(), period, s.actorFromRequest(r))
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "period": period})
}

func (s *Server) getSMTPSettings(w http.ResponseWriter, r *http.Request) {
	var en, tls bool
	var host, user, from *string
	var port int
	var passSet bool
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, host, port, username, (password IS NOT NULL AND password <> ''),
			from_address, use_tls
		FROM settings_smtp WHERE id = 1
	`).Scan(&en, &host, &port, &user, &passSet, &from, &tls)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": en, "host": host, "port": port, "username": user,
		"password_configured": passSet, "from_address": from, "use_tls": tls,
	})
}

func (s *Server) patchSMTPSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled     *bool   `json:"enabled"`
		Host        *string `json:"host"`
		Port        *int    `json:"port"`
		Username    *string `json:"username"`
		Password    *string `json:"password"`
		FromAddress *string `json:"from_address"`
		UseTLS      *bool   `json:"use_tls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE settings_smtp SET
			enabled = COALESCE($1, enabled),
			host = COALESCE($2, host),
			port = COALESCE($3, port),
			username = COALESCE($4, username),
			password = CASE WHEN $5::text IS NOT NULL AND $5 <> '' THEN $5 ELSE password END,
			from_address = COALESCE($6, from_address),
			use_tls = COALESCE($7, use_tls),
			updated_at = now()
		WHERE id = 1
	`, body.Enabled, body.Host, body.Port, body.Username, body.Password, body.FromAddress, body.UseTLS)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_smtp", "1", "patch", s.actorFromRequest(r), nil, map[string]any{"enabled": body.Enabled})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testSMTPSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	cfg, err := mailclient.LoadConfig(r.Context(), s.DB())
	if err != nil || !cfg.Ready() {
		writeErr(w, 422, "VALIDATION", "Configure SMTP (host, porta, remetente) e active o envio.", nil)
		return
	}
	to := mailclient.ParseRecipients(body.To)
	if len(to) == 0 {
		writeErr(w, 400, "VALIDATION", "Campo «to» obrigatório para teste.", nil)
		return
	}
	msg := "Teste de e-mail NetQuasar — se recebeu esta mensagem, o SMTP está funcional."
	if err := mailclient.Send(r.Context(), cfg, to, "NetQuasar — teste SMTP", msg); err != nil {
		writeErr(w, 502, "SMTP_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func schedulePatchResetsLastRun(body map[string]any) bool {
	for _, k := range []string{"time_hhmm", "timezone", "frequency", "day_of_week", "day_of_month"} {
		if _, ok := body[k]; ok {
			return true
		}
	}
	return false
}

func patchAutomationSchedule(ctx context.Context, pool *pgxpool.Pool, table string, body map[string]any) error {
	if pool == nil {
		return fmt.Errorf("pool nil")
	}
	th, _ := body["time_hhmm"].(string)
	tz, _ := body["timezone"].(string)
	emailTo, _ := body["email_to"].(string)
	resetLast := schedulePatchResetsLastRun(body)
	switch table {
	case "automation_bng_stats_report":
		freq, _ := body["frequency"].(string)
		var dow *int
		if v, ok := body["day_of_week"].(float64); ok {
			d := int(v)
			dow = &d
		}
		_, err := pool.Exec(ctx, `
			UPDATE automation_bng_stats_report SET
				enabled = COALESCE($1, enabled),
				frequency = COALESCE(NULLIF($2,''), frequency),
				day_of_week = COALESCE($3, day_of_week),
				time_hhmm = COALESCE(NULLIF($4,''), time_hhmm),
				timezone = COALESCE(NULLIF($5,''), timezone),
				channel_telegram = COALESCE($6, channel_telegram),
				channel_email = COALESCE($7, channel_email),
				email_to = COALESCE($8, email_to),
				last_run_key = CASE WHEN $9 THEN NULL ELSE last_run_key END,
				last_run_at = CASE WHEN $9 THEN NULL ELSE last_run_at END,
				running = CASE WHEN $9 THEN false ELSE running END,
				updated_at = now()
			WHERE id = 1
		`, boolPtr(body, "enabled"), freq, dow, th, tz, boolPtr(body, "channel_telegram"), boolPtr(body, "channel_email"), nullStr(emailTo), resetLast)
		return err
	case "automation_alerts_digest":
		freq, _ := body["frequency"].(string)
		var dow *int
		if v, ok := body["day_of_week"].(float64); ok {
			d := int(v)
			dow = &d
		}
		_, err := pool.Exec(ctx, `
			UPDATE automation_alerts_digest SET
				enabled = COALESCE($1, enabled),
				frequency = COALESCE(NULLIF($2,''), frequency),
				day_of_week = COALESCE($3, day_of_week),
				time_hhmm = COALESCE(NULLIF($4,''), time_hhmm),
				timezone = COALESCE(NULLIF($5,''), timezone),
				channel_telegram = COALESCE($6, channel_telegram),
				channel_email = COALESCE($7, channel_email),
				email_to = COALESCE($8, email_to),
				last_run_key = CASE WHEN $9 THEN NULL ELSE last_run_key END,
				last_run_at = CASE WHEN $9 THEN NULL ELSE last_run_at END,
				running = CASE WHEN $9 THEN false ELSE running END,
				updated_at = now()
			WHERE id = 1
		`, boolPtr(body, "enabled"), freq, dow, th, tz, boolPtr(body, "channel_telegram"), boolPtr(body, "channel_email"), nullStr(emailTo), resetLast)
		return err
	default:
		var dom *int
		if v, ok := body["day_of_month"].(float64); ok {
			d := int(v)
			dom = &d
		}
		_, err := pool.Exec(ctx, `
			UPDATE automation_commercial_report SET
				enabled = COALESCE($1, enabled),
				day_of_month = COALESCE($2, day_of_month),
				time_hhmm = COALESCE(NULLIF($3,''), time_hhmm),
				timezone = COALESCE(NULLIF($4,''), timezone),
				channel_telegram = COALESCE($5, channel_telegram),
				channel_email = COALESCE($6, channel_email),
				email_to = COALESCE($7, email_to),
				last_run_period = CASE WHEN $8 THEN NULL ELSE last_run_period END,
				last_run_at = CASE WHEN $8 THEN NULL ELSE last_run_at END,
				running = CASE WHEN $8 THEN false ELSE running END,
				updated_at = now()
			WHERE id = 1
		`, boolPtr(body, "enabled"), dom, th, tz, boolPtr(body, "channel_telegram"), boolPtr(body, "channel_email"), nullStr(emailTo), resetLast)
		return err
	}
}

func boolPtr(m map[string]any, k string) any {
	v, ok := m[k].(bool)
	if !ok {
		return nil
	}
	return v
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
