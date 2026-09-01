package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/reporttelegram"
	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
)

// handlers_automation_schedules.go — "Nova automação" (Configurações → Automações): ao
// contrário das 5 automações "singleton" (automation_alerts_digest e afins, 1 linha fixa cada em
// automation_execution_log.go/handlers_report_schedules.go), automation_schedules
// (129_automation_schedules.sql) permite QUALQUER número de automações, cada uma escolhendo um
// relatório do catálogo do sistema (/api/v1/reports/system — inclui BGP, HubSoft, alertas, etc.,
// ver handlers_system_reports.go) ou um relatório de frota/combustível (composeFleetReportTelegram,
// handlers_fleet_fuelings.go) e a sua própria recorrência. Mesmo motor de "due"
// (scheduleutil) e mesmo bot Telegram "reports" das automações singleton.

type automationSchedule struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Domain          string  `json:"domain"`
	ReportID        string  `json:"report_id"`
	PeriodDays      int     `json:"period_days"`
	ChannelTelegram bool    `json:"channel_telegram"`
	Enabled         bool    `json:"enabled"`
	Frequency       string  `json:"frequency"`
	DayOfWeek       *int    `json:"day_of_week,omitempty"`
	DaysOfWeek      []int   `json:"days_of_week"`
	DayOfMonth      int     `json:"day_of_month"`
	TimeHHMM        string  `json:"time_hhmm"`
	Timezone        string  `json:"timezone"`
	LastRunAt       *string `json:"last_run_at,omitempty"`
	LastRunOK       *bool   `json:"last_run_ok,omitempty"`
	LastRunMessage  *string `json:"last_run_message,omitempty"`
	Running         bool    `json:"running"`

	// Não expostos em JSON (letra minúscula) — usados só pelo motor de "due" (scheduleutil).
	lastRunKey   *string
	lastRunAtRaw *time.Time
}

func scanAutomationSchedule(row interface {
	Scan(dest ...any) error
}) (automationSchedule, error) {
	var a automationSchedule
	var id uuid.UUID
	var dow *int32
	var days []int32
	var lastRunAt *time.Time
	var lastRunKey *string
	err := row.Scan(&id, &a.Name, &a.Domain, &a.ReportID, &a.PeriodDays, &a.ChannelTelegram, &a.Enabled,
		&a.Frequency, &dow, &days, &a.DayOfMonth, &a.TimeHHMM, &a.Timezone,
		&lastRunAt, &a.LastRunOK, &a.LastRunMessage, &a.Running, &lastRunKey)
	if err != nil {
		return automationSchedule{}, err
	}
	a.ID = id.String()
	if dow != nil {
		v := int(*dow)
		a.DayOfWeek = &v
	}
	a.DaysOfWeek = intSliceFromInt32(days)
	a.lastRunAtRaw = lastRunAt
	a.lastRunKey = lastRunKey
	if lastRunAt != nil {
		s := lastRunAt.UTC().Format(time.RFC3339)
		a.LastRunAt = &s
	}
	return a, nil
}

const automationScheduleCols = `id, name, domain, report_id, period_days, channel_telegram, enabled,
	frequency, day_of_week, days_of_week, day_of_month, time_hhmm, timezone,
	last_run_at, last_run_ok, last_run_message, running, last_run_key`

func (s *Server) listAutomationSchedules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT `+automationScheduleCols+` FROM automation_schedules ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := make([]automationSchedule, 0, 8)
	for rows.Next() {
		a, err := scanAutomationSchedule(rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": out})
}

type automationScheduleBody struct {
	Name            string `json:"name"`
	Domain          string `json:"domain"`
	ReportID        string `json:"report_id"`
	PeriodDays      int    `json:"period_days"`
	ChannelTelegram *bool  `json:"channel_telegram"`
	Enabled         *bool  `json:"enabled"`
	Frequency       string `json:"frequency"`
	DayOfWeek       *int   `json:"day_of_week"`
	DaysOfWeek      []int  `json:"days_of_week"`
	DayOfMonth      int    `json:"day_of_month"`
	TimeHHMM        string `json:"time_hhmm"`
	Timezone        string `json:"timezone"`
}

func validAutomationDomain(d string) bool {
	return d == "system" || d == "fleet"
}

func validAutomationReportID(domain, id string) bool {
	if domain == "fleet" {
		switch id {
		case "fuelings", "by-vehicle", "by-driver", "by-station", "by-cost-center":
			return true
		}
		return false
	}
	return systemReportIDValid(id)
}

func (s *Server) createAutomationSchedule(w http.ResponseWriter, r *http.Request) {
	var body automationScheduleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Domain = strings.TrimSpace(body.Domain)
	if body.Domain == "" {
		body.Domain = "system"
	}
	body.ReportID = strings.TrimSpace(body.ReportID)
	if body.Name == "" || !validAutomationDomain(body.Domain) || !validAutomationReportID(body.Domain, body.ReportID) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome, domínio e relatório válidos são obrigatórios", nil)
		return
	}
	if body.PeriodDays <= 0 {
		body.PeriodDays = 30
	}
	channelTelegram := true
	if body.ChannelTelegram != nil {
		channelTelegram = *body.ChannelTelegram
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	freq := strings.ToLower(strings.TrimSpace(body.Frequency))
	if freq == "" {
		freq = "daily"
	}
	timeHHMM := strings.TrimSpace(body.TimeHHMM)
	if timeHHMM == "" {
		timeHHMM = "08:00"
	}
	tz := strings.TrimSpace(body.Timezone)
	if tz == "" {
		tz = "America/Sao_Paulo"
	}
	dom := body.DayOfMonth
	if dom <= 0 {
		dom = 1
	}
	days := scheduleutil.NormalizeWeekdays(body.DaysOfWeek, body.DayOfWeek)

	var newID uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO automation_schedules
			(name, domain, report_id, period_days, channel_telegram, enabled, frequency,
			 day_of_week, days_of_week, day_of_month, time_hhmm, timezone)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id
	`, body.Name, body.Domain, body.ReportID, body.PeriodDays, channelTelegram, enabled, freq,
		body.DayOfWeek, int32SliceFromInt(days), dom, timeHHMM, tz).Scan(&newID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func int32SliceFromInt(in []int) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}

func (s *Server) updateAutomationSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "scheduleId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body automationScheduleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Domain = strings.TrimSpace(body.Domain)
	body.ReportID = strings.TrimSpace(body.ReportID)
	if body.Name == "" || !validAutomationDomain(body.Domain) || !validAutomationReportID(body.Domain, body.ReportID) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome, domínio e relatório válidos são obrigatórios", nil)
		return
	}
	if body.PeriodDays <= 0 {
		body.PeriodDays = 30
	}
	channelTelegram := true
	if body.ChannelTelegram != nil {
		channelTelegram = *body.ChannelTelegram
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	freq := strings.ToLower(strings.TrimSpace(body.Frequency))
	if freq == "" {
		freq = "daily"
	}
	timeHHMM := strings.TrimSpace(body.TimeHHMM)
	if timeHHMM == "" {
		timeHHMM = "08:00"
	}
	tz := strings.TrimSpace(body.Timezone)
	if tz == "" {
		tz = "America/Sao_Paulo"
	}
	dom := body.DayOfMonth
	if dom <= 0 {
		dom = 1
	}
	days := scheduleutil.NormalizeWeekdays(body.DaysOfWeek, body.DayOfWeek)

	ct, err := s.DB().Exec(r.Context(), `
		UPDATE automation_schedules SET
			name=$1, domain=$2, report_id=$3, period_days=$4, channel_telegram=$5, enabled=$6,
			frequency=$7, day_of_week=$8, days_of_week=$9, day_of_month=$10, time_hhmm=$11,
			timezone=$12, updated_at=now(),
			-- muda de agendamento => reset do "já corri hoje/este mês" para não perder o próximo disparo
			last_run_key = CASE WHEN frequency<>$7 OR time_hhmm<>$11 OR day_of_month<>$10 THEN NULL ELSE last_run_key END
		WHERE id=$13
	`, body.Name, body.Domain, body.ReportID, body.PeriodDays, channelTelegram, enabled, freq,
		body.DayOfWeek, int32SliceFromInt(days), dom, timeHHMM, tz, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "automação não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteAutomationSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "scheduleId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM automation_schedules WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "automação não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationScheduleNow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "scheduleId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	row := s.DB().QueryRow(r.Context(), `SELECT `+automationScheduleCols+` FROM automation_schedules WHERE id=$1`, id)
	a, err := scanAutomationSchedule(row)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "automação não encontrada", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if execErr := s.executeAutomationSchedule(r.Context(), a, "manual-"+time.Now().Format("20060102150405"),
		automationMetaFromActor(s.actorFromRequest(r), s.userIDFromRequest(r))); execErr != nil {
		writeErr(w, http.StatusBadGateway, "RUN_FAILED", execErr.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// executeAutomationSchedule executa UMA automação personalizada: compõe o texto (relatório de
// sistema ou de frota, consoante o domínio) e envia pelos canais activos — hoje só Telegram
// (mesmo bot "reports" das automações singleton); grava em automation_execution_log.
func (s *Server) executeAutomationSchedule(ctx context.Context, a automationSchedule, runKey string, meta automationRunMeta) error {
	pool := s.DB()
	started := time.Now().UTC()
	_, _ = pool.Exec(ctx, `UPDATE automation_schedules SET running=true, updated_at=now() WHERE id=$1`, a.ID)

	var text string
	var buildErr error
	if a.Domain == "fleet" {
		to := started.Format("2006-01-02")
		from := started.AddDate(0, 0, -a.PeriodDays).Format("2006-01-02")
		text, buildErr = s.composeFleetReportTelegram(ctx, a.ReportID, from, to)
	} else {
		opts := systemReportOptions{PeriodMode: periodModeReportOptions{Mode: "summary"}}
		if reportUsesPeriodMode(a.ReportID) {
			to := started
			from := started.AddDate(0, 0, -a.PeriodDays)
			opts.PeriodMode.From = from.Format("2006-01-02")
			opts.PeriodMode.To = to.Format("2006-01-02")
		}
		var payload map[string]any
		payload, buildErr = s.buildSystemReport(ctx, a.ReportID, opts)
		if buildErr == nil {
			title, _ := payload["title"].(string)
			text = reporttelegram.ComposeSystemReport(title, payload)
		}
	}

	var sendErr error
	if buildErr == nil && a.ChannelTelegram {
		cfg, cfgErr := telegramclient.LoadConfig(ctx, pool, "reports")
		if cfgErr != nil {
			sendErr = cfgErr
		} else if !cfg.Ready() {
			sendErr = fmt.Errorf("Telegram de relatórios não configurado (bot_token/chat_id)")
		} else {
			sendErr = telegramclient.SendMessageChunks(ctx, cfg, "Automação: "+a.Name+"\n\n"+text)
		}
	}

	finalErr := buildErr
	if finalErr == nil {
		finalErr = sendErr
	}
	ok := finalErr == nil
	statusMsg := "Concluído com sucesso"
	if !ok {
		statusMsg = "Falhou: " + finalErr.Error()
	}
	_, _ = pool.Exec(ctx, `
		UPDATE automation_schedules SET
			running=false, last_run_at=now(), last_run_ok=$1, last_run_message=$2, last_run_key=$3, updated_at=now()
		WHERE id=$4
	`, ok, statusMsg, runKey, a.ID)

	jobType := "custom:" + a.Domain + ":" + a.ReportID
	s.recordAutomationExecution(ctx, jobType, meta, started, ok, statusMsg, finalErr,
		map[string]any{"schedule_id": a.ID, "schedule_name": a.Name}, runKey)
	return finalErr
}

// tryScheduledCustomAutomations verifica TODAS as automation_schedules e dispara as que estão
// vencidas — chamada a cada tick de runReportSchedulersLoop (handlers_report_schedules.go), tal
// como tryScheduledAlertsDigest/tryScheduledCommercialReport.
func (s *Server) tryScheduledCustomAutomations(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE automation_schedules SET running=false, updated_at=now()
		WHERE running=true AND updated_at < now() - interval '30 minutes'
	`)
	rows, err := pool.Query(ctx, `SELECT `+automationScheduleCols+` FROM automation_schedules WHERE enabled=true`)
	if err != nil {
		return
	}
	var due []struct {
		sched  automationSchedule
		runKey string
	}
	for rows.Next() {
		a, serr := scanAutomationSchedule(rows)
		if serr != nil {
			continue
		}
		var runKey string
		var isDue bool
		if a.Frequency == "monthly" {
			runKey, isDue = scheduleutil.MonthlyDue(a.Enabled, a.Timezone, a.TimeHHMM, a.DayOfMonth,
				a.lastRunKey, a.lastRunAtRaw, a.Running, time.Now())
		} else {
			runKey, isDue = scheduleutil.DailyWeeklyDueOnDays(a.Enabled, a.Frequency, a.Timezone, a.TimeHHMM,
				a.DayOfWeek, a.DaysOfWeek, a.lastRunKey, a.lastRunAtRaw, a.Running, time.Now())
		}
		if isDue {
			due = append(due, struct {
				sched  automationSchedule
				runKey string
			}{a, runKey})
		}
	}
	rows.Close()
	for _, d := range due {
		if err := s.executeAutomationSchedule(ctx, d.sched, d.runKey, automationMetaFromActor(auditActorSistema, nil)); err != nil && log != nil {
			log.Warn().Err(err).Str("schedule_id", d.sched.ID).Str("name", d.sched.Name).Msg("automação personalizada agendada falhou")
		}
	}
}
