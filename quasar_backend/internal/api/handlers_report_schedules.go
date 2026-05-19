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
	if s.WorkerCtx == nil {
		return
	}
	s.automationReportsOnce.Do(func() {
		go s.runReportSchedulersLoop(s.WorkerCtx)
	})
}

func (s *Server) runReportSchedulersLoop(ctx context.Context) {
	l := s.Log.With().Str("component", "report_schedulers").Logger()
	corr := time.NewTicker(5 * time.Minute)
	defer corr.Stop()
	alertcorrelation.Reconcile(ctx, s.DB(), &l)
	align := time.Until(time.Now().Truncate(time.Hour).Add(time.Hour))
	if align > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(align):
		}
	}
	hourly := time.NewTicker(time.Hour)
	defer hourly.Stop()
	s.tryScheduledAlertsDigest(ctx, &l)
	s.tryScheduledCommercialReport(ctx, &l)
	for {
		select {
		case <-ctx.Done():
			l.Info().Msg("agendadores de relatórios encerrados")
			return
		case <-corr.C:
			alertcorrelation.Reconcile(ctx, s.DB(), &l)
		case <-hourly.C:
			s.tryScheduledAlertsDigest(ctx, &l)
			s.tryScheduledCommercialReport(ctx, &l)
		}
	}
}

func (s *Server) tryScheduledAlertsDigest(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
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
	runKey, due := scheduleutil.DailyWeeklyDue(en, frequency, tzStr, thStr, dow, lastKey, running, time.Now())
	if !due {
		return
	}
	if err := s.executeAlertsDigest(ctx, runKey, "scheduler"); err != nil && log != nil {
		log.Warn().Err(err).Str("run_key", runKey).Msg("resumo de alertas agendado falhou")
	}
}

func (s *Server) tryScheduledCommercialReport(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	var en, running, tg, em bool
	var dom *int
	var th, tz, lastPeriod, emailTo *string
	err := pool.QueryRow(ctx, `
		SELECT enabled, day_of_month, time_hhmm, timezone,
			channel_telegram, channel_email, email_to, last_run_period, running
		FROM automation_commercial_report WHERE id = 1
	`).Scan(&en, &dom, &th, &tz, &tg, &em, &emailTo, &lastPeriod, &running)
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
	period, due := scheduleutil.MonthlyDue(en, tzStr, thStr, domVal, lastPeriod, running, time.Now())
	if !due {
		return
	}
	if err := s.executeCommercialReportOnly(ctx, period, "scheduler"); err != nil && log != nil {
		log.Warn().Err(err).Str("period", period).Msg("relatório comercial agendado falhou")
	}
}

func (s *Server) executeAlertsDigest(ctx context.Context, runKey, actor string) error {
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
		sendErr = fmt.Errorf("nenhum canal activo (Telegram ou e-mail)")
	}
	if sendErr != nil {
		s.setAlertsDigestStatus(ctx, "failed", strPtr(sendErr.Error()), runKey)
		return sendErr
	}
	s.setAlertsDigestStatus(ctx, "completed", nil, runKey)
	s.appendAuditLog(ctx, "automation_alerts_digest", "1", "run", actor, nil, map[string]any{"run_key": runKey})
	return nil
}

func (s *Server) composeAlertsDigest(ctx context.Context) (subject, body string, err error) {
	pool := s.DB()
	if pool == nil {
		return "", "", fmt.Errorf("pool nil")
	}
	var openTotal, openCrit, openWarn, openInfo int64
	var closed24h, opened24h int64
	var openIncidents int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE closed_at IS NULL`).Scan(&openTotal)
	_ = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE severity = 'critical'),
			COUNT(*) FILTER (WHERE severity = 'warning'),
			COUNT(*) FILTER (WHERE severity = 'info')
		FROM alert_instances WHERE closed_at IS NULL
	`).Scan(&openCrit, &openWarn, &openInfo)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alert_instances
		WHERE closed_at IS NOT NULL AND closed_at >= now() - interval '24 hours'
	`).Scan(&closed24h)
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alert_instances
		WHERE active_since >= now() - interval '24 hours'
	`).Scan(&opened24h)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_incidents WHERE status = 'open'`).Scan(&openIncidents)

	var sb strings.Builder
	now := time.Now().Format("02/01/2006 15:04")
	subject = fmt.Sprintf("NetQuasar — Resumo de alertas (%s)", now)
	sb.WriteString("NetQuasar — Resumo de alertas\n")
	sb.WriteString("Gerado em: ")
	sb.WriteString(now)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Alertas abertos: %d (críticos %d · aviso %d · info %d)\n", openTotal, openCrit, openWarn, openInfo))
	sb.WriteString(fmt.Sprintf("Incidentes correlacionados abertos: %d\n", openIncidents))
	sb.WriteString(fmt.Sprintf("Abertos nas últimas 24 h: %d · Resolvidos nas últimas 24 h: %d\n\n", opened24h, closed24h))

	rows, err := pool.Query(ctx, `
		SELECT alert_type, COUNT(*)::bigint
		FROM alert_instances WHERE closed_at IS NULL
		GROUP BY alert_type ORDER BY COUNT(*) DESC LIMIT 8
	`)
	if err == nil {
		sb.WriteString("Por tipo (abertos):\n")
		for rows.Next() {
			var typ string
			var n int64
			if rows.Scan(&typ, &n) == nil {
				sb.WriteString(fmt.Sprintf("  • %s: %d\n", typ, n))
			}
		}
		rows.Close()
	}

	incRows, err := pool.Query(ctx, `
		SELECT title, root_cause,
			(SELECT COUNT(*) FROM alert_incident_alerts a WHERE a.incident_id = i.id) AS n
		FROM alert_incidents i WHERE status = 'open'
		ORDER BY opened_at DESC LIMIT 5
	`)
	if err == nil {
		sb.WriteString("\nIncidentes em destaque:\n")
		for incRows.Next() {
			var title, cause string
			var n int64
			if incRows.Scan(&title, &cause, &n) == nil {
				sb.WriteString(fmt.Sprintf("  • %s [%s] — %d alerta(s)\n", title, cause, n))
			}
		}
		incRows.Close()
	}
	sb.WriteString("\n— Enviado automaticamente pelo NetQuasar\n")
	return subject, sb.String(), nil
}

func (s *Server) executeCommercialReportOnly(ctx context.Context, period, actor string) error {
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
		return err
	}
	var sendErr error
	if tg {
		cfg, err := telegramclient.LoadConfig(ctx, pool, "reports")
		if err != nil || !cfg.Ready() {
			sendErr = fmt.Errorf("Telegram relatórios: %w", err)
		} else if err := telegramclient.SendMessageWithParseMode(ctx, cfg, text, "HTML"); err != nil {
			sendErr = err
		}
	}
	if em && sendErr == nil {
		smtpCfg, err := mailclient.LoadConfig(ctx, pool)
		if err != nil || !smtpCfg.Ready() {
			sendErr = fmt.Errorf("SMTP: %w", err)
		} else {
			plain := strings.ReplaceAll(text, "<b>", "")
			plain = strings.ReplaceAll(plain, "</b>", "")
			subject := fmt.Sprintf("NetQuasar — Base comercial %s", period)
			to := mailclient.ParseRecipients(ptrStr(emailTo))
			if err := mailclient.Send(ctx, smtpCfg, to, subject, plain); err != nil {
				sendErr = err
			}
		}
	}
	if !tg && !em {
		sendErr = fmt.Errorf("nenhum canal activo")
	}
	if sendErr != nil {
		s.setCommercialReportStatus(ctx, "failed", strPtr(sendErr.Error()), period)
		return sendErr
	}
	s.setCommercialReportStatus(ctx, "completed", nil, period)
	s.appendAuditLog(ctx, "automation_commercial_report", "1", "run", actor, nil, map[string]any{"period": period})
	return nil
}

func (s *Server) setAlertsDigestStatus(ctx context.Context, status string, errMsg *string, runKey string) {
	var em any
	if errMsg != nil {
		em = *errMsg
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_alerts_digest SET
			last_status = $1, last_error = $2, last_run_key = $3, last_run_at = now(), updated_at = now()
		WHERE id = 1
	`, status, em, runKey)
}

func (s *Server) setCommercialReportStatus(ctx context.Context, status string, errMsg *string, period string) {
	var em any
	if errMsg != nil {
		em = *errMsg
	}
	_, _ = s.DB().Exec(ctx, `
		UPDATE automation_commercial_report SET
			last_status = $1, last_error = $2, last_run_period = $3, last_run_at = now(), updated_at = now()
		WHERE id = 1
	`, status, em, period)
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
	s.appendAuditLog(r.Context(), "automation_alerts_digest", "1", "patch", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationAlertsDigest(w http.ResponseWriter, r *http.Request) {
	runKey := time.Now().Format("2006-01-02")
	go func() {
		_ = s.executeAlertsDigest(context.Background(), runKey, actorFromRequest(r))
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
	s.appendAuditLog(r.Context(), "automation_commercial_report", "1", "patch", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationCommercialReport(w http.ResponseWriter, r *http.Request) {
	var tz string
	_ = s.DB().QueryRow(r.Context(), `SELECT timezone FROM automation_commercial_report WHERE id=1`).Scan(&tz)
	period := onuReportPeriodNow(tz)
	go func() {
		_ = s.executeCommercialReportOnly(context.Background(), period, actorFromRequest(r))
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
	s.appendAuditLog(r.Context(), "settings_smtp", "1", "patch", actorFromRequest(r), nil, map[string]any{"enabled": body.Enabled})
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

func patchAutomationSchedule(ctx context.Context, pool *pgxpool.Pool, table string, body map[string]any) error {
	if pool == nil {
		return fmt.Errorf("pool nil")
	}
	th, _ := body["time_hhmm"].(string)
	tz, _ := body["timezone"].(string)
	emailTo, _ := body["email_to"].(string)
	switch table {
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
				updated_at = now()
			WHERE id = 1
		`, boolPtr(body, "enabled"), freq, dow, th, tz, boolPtr(body, "channel_telegram"), boolPtr(body, "channel_email"), nullStr(emailTo))
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
				updated_at = now()
			WHERE id = 1
		`, boolPtr(body, "enabled"), dom, th, tz, boolPtr(body, "channel_telegram"), boolPtr(body, "channel_email"), nullStr(emailTo))
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
