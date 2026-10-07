package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/panicguard"
	"github.com/rs/zerolog"
)

// Automação "Coleta de ONUs (OLT)": duas cadências independentes sobre TODAS as OLTs.
//   - leve   (modo status_rx): status das ONUs/PONs + RX — barata, roda a cada poucos minutos.
//   - completa (modo full): serial, temperatura, TX, telnet… — cara, roda espaçada (horas).
//
// Reaproveita monitorworker.RunOltCollectAll (mesmo caminho do ciclo periódico e do botão manual,
// com snmpdevicelock por equipamento), em vez de um coletor novo. O modo "full" cai no
// refresh completo (refreshOLTDeviceCore), que inclui a fase de completar serial por PON.

const (
	oltOnuKindLight = "light"
	oltOnuKindFull  = "full"
)

// oltOnuCollectionDue decide, de forma pura, o que deve rodar agora. A coleta completa já
// atualiza tudo o que a leve atualizaria, então quando as duas vencem só a completa roda.
func oltOnuCollectionDue(now time.Time,
	lightEnabled bool, lightMin int, lastLight *time.Time, runningLight bool,
	fullEnabled bool, fullMin int, lastFull *time.Time, runningFull bool,
) (runFull, runLight bool) {
	elapsed := func(last *time.Time, mins int) bool {
		return last == nil || now.Sub(*last) >= time.Duration(mins)*time.Minute
	}
	if fullEnabled && !runningFull && elapsed(lastFull, fullMin) {
		return true, false
	}
	if lightEnabled && !runningLight && !runningFull && elapsed(lastLight, lightMin) {
		return false, true
	}
	return false, false
}

func (s *Server) clearStaleOltOnuCollectionRunning(ctx context.Context) {
	pool := s.DB()
	if pool == nil {
		return
	}
	_, _ = pool.Exec(ctx, `
		UPDATE automation_olt_onu_collection SET
			running_light = CASE WHEN running_light AND updated_at < now() - interval '2 hours' THEN false ELSE running_light END,
			running_full  = CASE WHEN running_full  AND updated_at < now() - interval '8 hours' THEN false ELSE running_full  END
		WHERE id = 1 AND (running_light OR running_full)
	`)
}

func (s *Server) tryScheduledOltOnuCollection(ctx context.Context, log *zerolog.Logger) {
	pool := s.DB()
	if pool == nil {
		return
	}
	s.clearStaleOltOnuCollectionRunning(ctx)
	var en, lightEn, fullEn, runLightFlag, runFullFlag bool
	var lightMin, fullMin int
	var lastLight, lastFull *time.Time
	err := pool.QueryRow(ctx, `
		SELECT enabled, light_enabled, light_interval_minutes, last_light_at, running_light,
			full_enabled, full_interval_minutes, last_full_at, running_full
		FROM automation_olt_onu_collection WHERE id = 1
	`).Scan(&en, &lightEn, &lightMin, &lastLight, &runLightFlag, &fullEn, &fullMin, &lastFull, &runFullFlag)
	if err != nil || !en {
		return
	}
	runFull, runLight := oltOnuCollectionDue(time.Now(), lightEn, lightMin, lastLight, runLightFlag,
		fullEn, fullMin, lastFull, runFullFlag)
	kind := ""
	switch {
	case runFull:
		kind = oltOnuKindFull
	case runLight:
		kind = oltOnuKindLight
	default:
		return
	}
	// Em goroutine: o loop dos agendadores (a cada 30s) não pode ficar preso numa coleta que leva minutos.
	go func() {
		defer panicguard.Recover("scheduled_olt_onu_collection")
		if err := s.executeOltOnuCollection(context.Background(), kind, automationMetaFromActor(auditActorSistema, nil)); err != nil && log != nil {
			log.Warn().Err(err).Str("kind", kind).Msg("coleta de ONUs (OLT) agendada falhou")
		}
	}()
}

func (s *Server) executeOltOnuCollection(ctx context.Context, kind string, meta automationRunMeta) error {
	started := time.Now()
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base indisponível")
	}
	runCol, atCol, sumCol := "running_light", "last_light_at", "last_light_summary"
	mode, timeout, label := "status_rx", 30*time.Minute, "leve (status + RX)"
	if kind == oltOnuKindFull {
		runCol, atCol, sumCol = "running_full", "last_full_at", "last_full_summary"
		mode, timeout, label = "full", 4*time.Hour, "completa"
	}
	runKey := kind + "-" + started.Format("20060102T150405")

	tag, err := pool.Exec(ctx, `
		UPDATE automation_olt_onu_collection SET `+runCol+` = true, updated_at = now()
		WHERE id = 1 AND `+runCol+` = false AND enabled = true
	`)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		s.recordAutomationExecution(ctx, jobOltOnuCollection, meta, started, false,
			"Não iniciado (desativado ou coleta "+label+" já em execução)", nil,
			map[string]any{"kind": kind}, runKey)
		return nil
	}
	defer func() {
		_, _ = pool.Exec(context.Background(),
			`UPDATE automation_olt_onu_collection SET `+runCol+` = false, updated_at = now() WHERE id = 1`)
	}()

	l := s.Log.With().Str("component", "olt_onu_collection").Str("kind", kind).Logger()
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, runErr := monitorworker.RunOltCollectAll(rctx, pool, &l, monitorworker.ModeFull, monitorworker.SweepOpts{
		Source: "automation_olt_" + kind,
		PipelineStep: &monitorworker.PipelineStep{
			ID: "automation-olt-" + kind, Kind: monitorworker.StepKindOltOnu, Enabled: true,
			Scope:   monitorworker.StepScope{Target: "category", Category: "olt"},
			Options: monitorworker.StepOptions{OltOnuMode: mode},
		},
	})

	failures := []map[string]any{}
	for _, o := range res.Outcomes {
		if ok, _ := o["ok"].(bool); !ok && len(failures) < 20 {
			failures = append(failures, o)
		}
	}
	summary := map[string]any{
		"kind": kind, "mode": mode, "olts_total": res.TotalInDB, "olts_eligible": res.Eligible,
		"olts_ok": res.OK, "olts_failed": res.Failed, "olts_skipped": res.Skipped,
		"duration_s": int(time.Since(started).Seconds()), "failures": failures,
	}
	status, msg := "completed", fmt.Sprintf("Coleta %s: %d OLT(s) ok", label, res.OK)
	var errText *string
	ok := true
	switch {
	case runErr != nil:
		status, ok = "failed", false
		msg = "Falha na coleta " + label
		e := runErr.Error()
		errText = &e
	case res.Eligible == 0:
		status, ok = "failed", false
		msg = "Nenhuma OLT elegível para coleta (verifique IP, SNMP, marca/modelo)"
		e := msg
		errText = &e
	case res.Failed > 0:
		status = "partial"
		msg = fmt.Sprintf("Coleta %s: %d ok, %d com falha", label, res.OK, res.Failed)
		e := fmt.Sprintf("%d OLT(s) falharam — ver detalhes no histórico", res.Failed)
		errText = &e
	}

	sumJSON, _ := json.Marshal(summary)
	// A coleta completa também atualiza o que a leve atualizaria — adia a próxima leve.
	extra := ""
	if kind == oltOnuKindFull {
		extra = ", last_light_at = now()"
	}
	_, _ = pool.Exec(context.Background(), `
		UPDATE automation_olt_onu_collection SET
			`+atCol+` = now()`+extra+`, last_run_at = now(), last_status = $1, last_error = $2,
			`+sumCol+` = $3::jsonb, updated_at = now()
		WHERE id = 1
	`, status, errText, sumJSON)

	s.recordAutomationExecution(ctx, jobOltOnuCollection, meta, started, ok, msg, runErr, summary, runKey)
	s.appendAuditLog(ctx, "automation_olt_onu_collection", "1", "run", meta.Actor, nil,
		map[string]any{"kind": kind, "run_key": runKey, "ok": ok, "olts_ok": res.OK, "olts_failed": res.Failed})
	return runErr
}

func (s *Server) getAutomationOltOnuCollection(w http.ResponseWriter, r *http.Request) {
	var en, lightEn, fullEn, runLight, runFull bool
	var lightMin, fullMin int
	var lastLight, lastFull, lastRun *time.Time
	var ls, le *string
	var lightSum, fullSum []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT enabled, light_enabled, light_interval_minutes, full_enabled, full_interval_minutes,
			last_light_at, last_full_at, last_run_at, last_status, last_error,
			running_light, running_full, last_light_summary, last_full_summary
		FROM automation_olt_onu_collection WHERE id = 1
	`).Scan(&en, &lightEn, &lightMin, &fullEn, &fullMin, &lastLight, &lastFull, &lastRun, &ls, &le,
		&runLight, &runFull, &lightSum, &fullSum)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{
		"enabled": en, "light_enabled": lightEn, "light_interval_minutes": lightMin,
		"full_enabled": fullEn, "full_interval_minutes": fullMin,
		"last_light_at": lastLight, "last_full_at": lastFull, "last_run_at": lastRun,
		"last_status": ls, "last_error": le,
		"running": runLight || runFull, "running_light": runLight, "running_full": runFull,
	}
	if len(lightSum) > 0 {
		out["last_light_summary"] = json.RawMessage(lightSum)
	}
	if len(fullSum) > 0 {
		out["last_full_summary"] = json.RawMessage(fullSum)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patchAutomationOltOnuCollection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled              *bool `json:"enabled"`
		LightEnabled         *bool `json:"light_enabled"`
		LightIntervalMinutes *int  `json:"light_interval_minutes"`
		FullEnabled          *bool `json:"full_enabled"`
		FullIntervalMinutes  *int  `json:"full_interval_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if v := body.LightIntervalMinutes; v != nil && (*v < 1 || *v > 1440) {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "intervalo da coleta leve: 1 a 1440 minutos", nil)
		return
	}
	if v := body.FullIntervalMinutes; v != nil && (*v < 15 || *v > 10080) {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "intervalo da coleta completa: 15 a 10080 minutos", nil)
		return
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE automation_olt_onu_collection SET
			enabled = COALESCE($1, enabled),
			light_enabled = COALESCE($2, light_enabled),
			light_interval_minutes = COALESCE($3, light_interval_minutes),
			full_enabled = COALESCE($4, full_enabled),
			full_interval_minutes = COALESCE($5, full_interval_minutes),
			updated_at = now()
		WHERE id = 1
	`, body.Enabled, body.LightEnabled, body.LightIntervalMinutes, body.FullEnabled, body.FullIntervalMinutes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "automation_olt_onu_collection", "1", "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runAutomationOltOnuCollection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	kind := oltOnuKindFull
	if strings.EqualFold(strings.TrimSpace(body.Kind), oltOnuKindLight) {
		kind = oltOnuKindLight
	}
	meta := s.automationMetaFromRequest(r)
	go func() {
		defer panicguard.Recover("api_run_olt_onu_collection")
		_ = s.executeOltOnuCollection(context.Background(), kind, meta)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "kind": kind})
}
