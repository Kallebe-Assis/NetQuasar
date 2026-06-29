package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/connectivity"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

// runFullMonitoringBootstrap executa a primeira sequência completa (ping → telemetria → IF MikroTik → IF OLT → PON)
// após iniciar modo full, sem depender dos intervalos do worker.
func (s *Server) runFullMonitoringBootstrap() {
	pool := s.DB()
	if pool == nil {
		return
	}
	ctx := context.Background()
	if s.WorkerCtx != nil {
		ctx = s.WorkerCtx
	}
	l := s.Log.With().Str("component", "monitor_bootstrap").Logger()
	monitorworker.RunBootstrapPipeline(ctx, pool, &l)
}

func (s *Server) loadInternetCheckConfig(ctx context.Context) ([]string, time.Duration, error) {
	var raw []byte
	var ms int
	err := s.DB().QueryRow(ctx, `
		SELECT internet_check_targets, internet_check_timeout_ms FROM monitoring_settings WHERE id = 1
	`).Scan(&raw, &ms)
	if err != nil {
		return nil, 0, err
	}
	def := []string{"https://1.1.1.1", "https://www.google.com/generate_204"}
	targets := connectivity.TargetsFromJSON(raw, def)
	return targets, time.Duration(ms) * time.Millisecond, nil
}

func (s *Server) persistInternetCheck(ctx context.Context, res connectivity.Result) {
	detail, _ := json.Marshal(res)
	_, _ = s.DB().Exec(ctx, `
		UPDATE monitoring_runtime SET
			last_internet_check_at = now(),
			last_internet_check_ok = $1,
			last_internet_check_detail = $2::jsonb,
			updated_at = now()
		WHERE id = 1
	`, res.OK, detail)
}

func (s *Server) internetCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	s.setMonitoringActivity(ctx, "Verificando conectividade com a internet")
	targets, timeout, err := s.loadInternetCheckConfig(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	res := connectivity.Check(ctx, targets, timeout)
	s.persistInternetCheck(ctx, res)
	s.setMonitoringActivity(ctx, "")
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) monitoringStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body struct {
		Mode                 string `json:"mode"`
		RefreshSnmpInventory bool   `json:"refresh_snmp_inventory"`
	}
	// Body opcional: {"mode":"simple_ping"} ou {"mode":"full"} — corpo vazio usa simple_ping.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(body.Mode))
	if mode == "" {
		mode = monitorworker.ModeSimplePing
	}
	if mode != monitorworker.ModeSimplePing && mode != monitorworker.ModeFull {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", `mode deve ser "simple_ping" ou "full"`, map[string]any{
			"allowed": []string{monitorworker.ModeSimplePing, monitorworker.ModeFull},
		})
		return
	}

	var wasRunning bool
	if err := s.DB().QueryRow(ctx, `SELECT is_running FROM monitoring_runtime WHERE id=1`).Scan(&wasRunning); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	refreshSnmpInv := false
	if !wasRunning && mode == monitorworker.ModeFull {
		refreshSnmpInv = body.RefreshSnmpInventory
	}

	if !wasRunning {
		targets, timeout, err := s.loadInternetCheckConfig(ctx)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		res := connectivity.Check(ctx, targets, timeout)
		s.persistInternetCheck(ctx, res)
		if !res.OK {
			writeErr(w, http.StatusFailedDependency, "NO_INTERNET", "Sem conectividade com a Internet; monitoramento não iniciado.", res)
			return
		}
	}

	ct, err := s.DB().Exec(ctx, `
		UPDATE monitoring_runtime SET
			is_running = true,
			monitoring_mode = $1,
			mon_session_id = CASE WHEN NOT is_running THEN gen_random_uuid() ELSE mon_session_id END,
			offer_snmp_inventory_refresh = CASE WHEN NOT is_running THEN $2 ELSE offer_snmp_inventory_refresh END,
			last_started_at = CASE WHEN is_running THEN last_started_at ELSE now() END,
			updated_at = now()
		WHERE id = 1
	`, mode, refreshSnmpInv)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusInternalServerError, "DB", "runtime row missing", nil)
		return
	}
	note := "O ciclo roda no servidor; trocar de tela no app não interrompe. Use POST /monitoring/stop para encerrar."
	if !wasRunning && mode == monitorworker.ModeFull {
		note += " Em modo full é executada de imediato uma sequência inicial: ping → telemetria → interfaces (MikroTik, depois OLT) → PON IF-MIB; acompanhe current_activity em GET /monitoring/state."
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "running",
		"monitoring_mode": mode,
		"note":            note,
		"available_modes": []string{monitorworker.ModeSimplePing, monitorworker.ModeFull},
	})
	s.setMonitoringActivity(ctx, "Monitoramento iniciado")
	s.appendAuditLog(ctx, "monitoring_runtime", "1", "start", s.actorFromRequest(r), nil, map[string]any{
		"monitoring_mode": mode, "was_running": wasRunning, "refresh_snmp_inventory": refreshSnmpInv,
	})
	if !wasRunning {
		monitorworker.BeginActiveRun(s.WorkerCtx)
	}
	if !wasRunning && mode == monitorworker.ModeFull {
		go s.runFullMonitoringBootstrap()
	}
}

func (s *Server) monitoringStop(w http.ResponseWriter, r *http.Request) {
	_, err := s.DB().Exec(r.Context(), `
		UPDATE monitoring_runtime SET
			is_running = false,
			monitoring_mode = 'off',
			offer_snmp_inventory_refresh = false,
			last_stopped_at = now(),
			updated_at = now()
		WHERE id = 1
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.setMonitoringActivity(r.Context(), "")
	monitorworker.EndActiveRun()
	s.appendAuditLog(r.Context(), "monitoring_runtime", "1", "stop", s.actorFromRequest(r), nil, map[string]any{
		"monitoring_mode": monitorworker.ModeOff,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "stopped",
		"monitoring_mode": monitorworker.ModeOff,
		"note":            "Worker deixa de executar ciclos; últimos resultados permanecem em device_probe_cache até próximo ciclo.",
	})
}

func (s *Server) monitoringState(w http.ResponseWriter, r *http.Request) {
	s.maybeKickNightlyCollection(r.Context())
	var running bool
	var started, stopped, lastCheck, lastCycle *time.Time
	var lastLatency, lastTelemetry, lastIface, lastOlt, lastBng, lastPipeline *time.Time
	var activity *string
	var activityStarted *time.Time
	var activityUpdated *time.Time
	var lastActivity *string
	var lastActivityFinished *time.Time
	var lastOK *bool
	var detail []byte
	var mode string
	var okC, failC int
	var runtimeUpdated time.Time
	var lastAlertsChange *time.Time
	err := s.DB().QueryRow(r.Context(), `
		SELECT is_running, last_started_at, last_stopped_at, last_internet_check_at, last_internet_check_ok, last_internet_check_detail,
			COALESCE(monitoring_mode, 'off'), last_cycle_at,
			last_latency_cycle_at, last_telemetry_cycle_at, last_interface_snapshot_cycle_at, last_olt_if_derived_cycle_at,
			last_bng_cycle_at, last_pipeline_cycle_at,
			COALESCE(last_cycle_ok_count, 0), COALESCE(last_cycle_fail_count, 0),
			current_activity, activity_started_at, activity_updated_at, last_activity, last_activity_finished_at,
			updated_at, last_alerts_change_at
		FROM monitoring_runtime WHERE id = 1
	`).Scan(&running, &started, &stopped, &lastCheck, &lastOK, &detail, &mode, &lastCycle,
		&lastLatency, &lastTelemetry, &lastIface, &lastOlt, &lastBng, &lastPipeline,
		&okC, &failC, &activity, &activityStarted, &activityUpdated, &lastActivity, &lastActivityFinished, &runtimeUpdated, &lastAlertsChange)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{
		"is_running":                 running,
		"monitoring_mode":            mode,
		"available_modes":            []string{monitorworker.ModeSimplePing, monitorworker.ModeFull},
		"last_started_at":            started,
		"last_stopped_at":            stopped,
		"last_internet_check_at":     lastCheck,
		"last_internet_check_ok":     lastOK,
		"last_internet_check_detail": json.RawMessage(detail),
		"last_cycle_at":                      lastCycle,
		"last_latency_cycle_at":              lastLatency,
		"last_telemetry_cycle_at":           lastTelemetry,
		"last_interface_snapshot_cycle_at":  lastIface,
		"last_olt_if_derived_cycle_at":       lastOlt,
		"last_bng_cycle_at":                  lastBng,
		"last_pipeline_cycle_at":             lastPipeline,
		"last_cycle_ok_count":        okC,
		"last_cycle_fail_count":      failC,
		"current_activity":           activity,
		"activity_started_at":        activityStarted,
		"activity_updated_at":        activityUpdated,
		"last_activity":              lastActivity,
		"last_activity_finished_at":  lastActivityFinished,
		"runtime_updated_at":         runtimeUpdated,
		"last_alerts_change_at":      lastAlertsChange,
		"persistencia":               "O monitoramento é estado no servidor (Postgres + worker); não depende da tela aberta no cliente.",
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) monitoringOltCollectReadiness(w http.ResponseWriter, r *http.Request) {
	rows, err := monitorworker.AuditOltCollectReadiness(r.Context(), s.DB())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	ready, notReady := 0, 0
	list := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row.Ready {
			ready++
		} else {
			notReady++
		}
		list = append(list, map[string]any{
			"device_id":   row.DeviceID.String(),
			"description": row.Description,
			"host":        row.Host,
			"brand":       row.Brand,
			"model":       row.Model,
			"ready":       row.Ready,
			"reason":      row.Reason,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":     len(rows),
		"ready":     ready,
		"not_ready": notReady,
		"items":     list,
	})
}

func (s *Server) monitoringReloadDevices(w http.ResponseWriter, r *http.Request) {
	// Invalidação lógica: em implementação futura com cache em memória, aqui limparíamos.
	// Por ora confirma que o banco está acessível e retorna contagem.
	var n int64
	if err := s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM devices`).Scan(&n); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "monitoring_runtime", "1", "reload_devices", s.actorFromRequest(r), nil, map[string]any{
		"device_count": n,
	})
	writeJSON(w, http.StatusOK, map[string]any{"reloaded": true, "device_count": n})
}

func (s *Server) getMonitoringIntervals(w http.ResponseWriter, r *http.Request) {
	var ps, tm, pto int
	var telSecRaw, ifaceSec, oltDerivedSec, pipelineSec, mikrotikTimeout, bngTimeout int
	var telTimeout, ifaceTimeout, oltTimeout, oltOnuTelnetTimeout int
	var icmpPB, offTh, uptimeRestart int
	var pingParallel bool
	var pipelineRaw []byte
	if err := s.DB().QueryRow(r.Context(), `
		SELECT ping_seconds, telemetry_minutes, ping_timeout_ms,
			telemetry_seconds,
			interface_snapshot_seconds, olt_if_derived_pon_seconds,
			telemetry_timeout_ms, interface_snapshot_timeout_ms, olt_if_derived_pon_timeout_ms,
			olt_onu_telnet_timeout_ms,
			icmp_payload_bytes, offline_ping_fail_threshold,
			COALESCE(uptime_restart_alert_minutes, 0),
			pipeline_cycle_seconds, mikrotik_timeout_ms, bng_timeout_ms,
			COALESCE(ping_parallel, true),
			coalesce(pipeline_steps::text, '[]')
		FROM monitoring_intervals WHERE id=1`).Scan(&ps, &tm, &pto, &telSecRaw, &ifaceSec, &oltDerivedSec,
		&telTimeout, &ifaceTimeout, &oltTimeout, &oltOnuTelnetTimeout, &icmpPB, &offTh, &uptimeRestart,
		&pipelineSec, &mikrotikTimeout, &bngTimeout, &pingParallel, &pipelineRaw); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	telSec := monitorworker.ResolveTelemetrySeconds(telSecRaw, tm)
	steps := monitorworker.ParsePipelineSteps(pipelineRaw)
	if len(steps) == 0 {
		steps = monitorworker.DefaultPipelineSteps()
	} else {
		steps = monitorworker.EnsureBngPipelineStep(steps)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ping_seconds":                    ps,
		"telemetry_seconds":               telSec,
		"interface_snapshot_seconds":      ifaceSec,
		"olt_if_derived_pon_seconds":      oltDerivedSec,
		"pipeline_cycle_seconds":          pipelineSec,
		"telemetry_minutes":               tm,
		"ping_timeout_ms":                 pto,
		"telemetry_timeout_ms":            telTimeout,
		"interface_snapshot_timeout_ms":   ifaceTimeout,
		"olt_if_derived_pon_timeout_ms":   oltTimeout,
		"olt_onu_telnet_timeout_ms":         oltOnuTelnetTimeout,
		"mikrotik_timeout_ms":             mikrotikTimeout,
		"bng_timeout_ms":                  bngTimeout,
		"icmp_payload_bytes":              icmpPB,
		"offline_ping_fail_threshold":     offTh,
		"uptime_restart_alert_minutes":    uptimeRestart,
		"ping_parallel":                   pingParallel,
		"pipeline_steps":                  steps,
	})
}

func (s *Server) patchMonitoringIntervals(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PingSeconds                   *int                           `json:"ping_seconds"`
		TelemetryMinutes              *int                           `json:"telemetry_minutes"`
		TelemetrySeconds              *int                           `json:"telemetry_seconds"`
		InterfaceSnapshotSeconds      *int                           `json:"interface_snapshot_seconds"`
		OltIfDerivedPonSeconds        *int                           `json:"olt_if_derived_pon_seconds"`
		PipelineCycleSeconds          *int                           `json:"pipeline_cycle_seconds"`
		PingTimeoutMs                 *int                           `json:"ping_timeout_ms"`
		TelemetryTimeoutMs            *int                           `json:"telemetry_timeout_ms"`
		InterfaceSnapshotTimeoutMs    *int                           `json:"interface_snapshot_timeout_ms"`
		OltIfDerivedPonTimeoutMs      *int                           `json:"olt_if_derived_pon_timeout_ms"`
		OltOnuTelnetTimeoutMs         *int                           `json:"olt_onu_telnet_timeout_ms"`
		MikrotikTimeoutMs             *int                           `json:"mikrotik_timeout_ms"`
		BngTimeoutMs                  *int                           `json:"bng_timeout_ms"`
		IcmpPayloadBytes              *int                           `json:"icmp_payload_bytes"`
		OfflinePingFailThreshold      *int                           `json:"offline_ping_fail_threshold"`
		UptimeRestartAlertMinutes     *int                           `json:"uptime_restart_alert_minutes"`
		PipelineSteps                 *[]monitorworker.PipelineStep `json:"pipeline_steps"`
		PingParallel                  *bool                         `json:"ping_parallel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var ps, tm, pto int
	var telSecRaw, ifaceSec, oltDerivedSec, pipelineSec, mikrotikTimeout, bngTimeout int
	var telTimeout, ifaceTimeout, oltTimeout, oltOnuTelnetTimeout int
	var icmpPB, offTh, uptimeRestart int
	var pingParallel bool
	var pipelineRaw []byte
	if err := s.DB().QueryRow(r.Context(), `
		SELECT ping_seconds, telemetry_minutes, ping_timeout_ms,
			telemetry_seconds,
			interface_snapshot_seconds, olt_if_derived_pon_seconds,
			telemetry_timeout_ms, interface_snapshot_timeout_ms, olt_if_derived_pon_timeout_ms,
			olt_onu_telnet_timeout_ms,
			icmp_payload_bytes, offline_ping_fail_threshold,
			COALESCE(uptime_restart_alert_minutes, 0),
			pipeline_cycle_seconds, mikrotik_timeout_ms, bng_timeout_ms,
			COALESCE(ping_parallel, true),
			coalesce(pipeline_steps::text, '[]')
		FROM monitoring_intervals WHERE id=1`).Scan(&ps, &tm, &pto, &telSecRaw, &ifaceSec, &oltDerivedSec,
		&telTimeout, &ifaceTimeout, &oltTimeout, &oltOnuTelnetTimeout, &icmpPB, &offTh, &uptimeRestart,
		&pipelineSec, &mikrotikTimeout, &bngTimeout, &pingParallel, &pipelineRaw); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	telSec := monitorworker.ResolveTelemetrySeconds(telSecRaw, tm)
	if body.PingSeconds != nil {
		ps = *body.PingSeconds
	}
	if body.TelemetryMinutes != nil {
		tm = *body.TelemetryMinutes
		telSec = tm * 60
	}
	if body.TelemetrySeconds != nil {
		telSec = *body.TelemetrySeconds
		tm = max(2, (telSec+59)/60)
	}
	if body.InterfaceSnapshotSeconds != nil {
		ifaceSec = *body.InterfaceSnapshotSeconds
	}
	if body.OltIfDerivedPonSeconds != nil {
		oltDerivedSec = *body.OltIfDerivedPonSeconds
	}
	if body.PipelineCycleSeconds != nil {
		pipelineSec = *body.PipelineCycleSeconds
	}
	if body.PingParallel != nil {
		pingParallel = *body.PingParallel
	}
	if body.PingTimeoutMs != nil {
		pto = *body.PingTimeoutMs
	}
	if body.TelemetryTimeoutMs != nil {
		telTimeout = *body.TelemetryTimeoutMs
	}
	if body.InterfaceSnapshotTimeoutMs != nil {
		ifaceTimeout = *body.InterfaceSnapshotTimeoutMs
	}
	if body.OltIfDerivedPonTimeoutMs != nil {
		oltTimeout = *body.OltIfDerivedPonTimeoutMs
	}
	if body.OltOnuTelnetTimeoutMs != nil {
		oltOnuTelnetTimeout = *body.OltOnuTelnetTimeoutMs
	}
	if body.MikrotikTimeoutMs != nil {
		mikrotikTimeout = *body.MikrotikTimeoutMs
	}
	if body.BngTimeoutMs != nil {
		bngTimeout = *body.BngTimeoutMs
	}
	if body.IcmpPayloadBytes != nil {
		icmpPB = *body.IcmpPayloadBytes
	}
	if body.OfflinePingFailThreshold != nil {
		offTh = *body.OfflinePingFailThreshold
	}
	if body.UptimeRestartAlertMinutes != nil {
		uptimeRestart = *body.UptimeRestartAlertMinutes
	}
	if ps < 30 || tm < 2 || telSec < 60 || ifaceSec < 60 || oltDerivedSec < 60 || pipelineSec < 30 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "ping_seconds >= 30; pipeline_cycle >= 30; telemetria/interfaces/PON >= 60 s; telemetry_minutes >= 2", map[string]any{
			"ping_seconds": ps, "telemetry_minutes": tm, "telemetry_seconds": telSec,
			"interface_snapshot_seconds": ifaceSec, "olt_if_derived_pon_seconds": oltDerivedSec,
			"pipeline_cycle_seconds": pipelineSec,
		})
		return
	}
	if pto < 1000 || pto > 30000 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "ping_timeout_ms entre 1000 e 30000 (1–30 s por sonda ICMP+TCP)", map[string]any{"ping_timeout_ms": pto})
		return
	}
	for _, pair := range []struct {
		name string
		val  int
	}{{"telemetry_timeout_ms", telTimeout}, {"interface_snapshot_timeout_ms", ifaceTimeout}, {"olt_if_derived_pon_timeout_ms", oltTimeout}, {"mikrotik_timeout_ms", mikrotikTimeout}, {"bng_timeout_ms", bngTimeout}} {
		if pair.val < 5000 || pair.val > 600000 {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", pair.name+" entre 5000 e 600000 (5 s – 10 min)", map[string]any{pair.name: pair.val})
			return
		}
	}
	if oltOnuTelnetTimeout < 5000 || oltOnuTelnetTimeout > 3600000 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "olt_onu_telnet_timeout_ms entre 5000 e 3600000 (5 s – 60 min)", map[string]any{"olt_onu_telnet_timeout_ms": oltOnuTelnetTimeout})
		return
	}
	if icmpPB < 0 || icmpPB > 65507 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "icmp_payload_bytes entre 0 e 65507 (0 usa defeito de 32 B no servidor)", map[string]any{"icmp_payload_bytes": icmpPB})
		return
	}
	if offTh < 1 || offTh > 50 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "offline_ping_fail_threshold entre 1 e 50 tentativas consecutivas", map[string]any{"offline_ping_fail_threshold": offTh})
		return
	}
	if uptimeRestart < 0 || uptimeRestart > 10080 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "uptime_restart_alert_minutes entre 0 (desligado) e 10080 (7 dias)", map[string]any{"uptime_restart_alert_minutes": uptimeRestart})
		return
	}
	var stepsJSON []byte
	if body.PipelineSteps != nil {
		norm := monitorworker.NormalizePipelineSteps(*body.PipelineSteps)
		if len(norm) == 0 {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "pipeline_steps não pode estar vazio", nil)
			return
		}
		stepsJSON, _ = json.Marshal(norm)
	} else {
		stepsJSON = pipelineRaw
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE monitoring_intervals SET
			ping_seconds=$1, telemetry_minutes=$2, telemetry_seconds=$3,
			interface_snapshot_seconds=$4, olt_if_derived_pon_seconds=$5,
			pipeline_cycle_seconds=$6,
			ping_timeout_ms=$7,
			telemetry_timeout_ms=$8, interface_snapshot_timeout_ms=$9, olt_if_derived_pon_timeout_ms=$10,
			olt_onu_telnet_timeout_ms=$11,
			mikrotik_timeout_ms=$12,
			bng_timeout_ms=$13,
			icmp_payload_bytes=$14, offline_ping_fail_threshold=$15,
			uptime_restart_alert_minutes=$16,
			pipeline_steps=$17::jsonb,
			ping_parallel=$18,
			updated_at=now() WHERE id=1`,
		ps, tm, telSec, ifaceSec, oltDerivedSec, pipelineSec, pto, telTimeout, ifaceTimeout, oltTimeout, oltOnuTelnetTimeout, mikrotikTimeout, bngTimeout, icmpPB, offTh, uptimeRestart, stepsJSON, pingParallel)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	payload := map[string]any{
		"ping_seconds":                    ps,
		"telemetry_seconds":               telSec,
		"interface_snapshot_seconds":      ifaceSec,
		"olt_if_derived_pon_seconds":      oltDerivedSec,
		"pipeline_cycle_seconds":          pipelineSec,
		"telemetry_minutes":               tm,
		"ping_timeout_ms":                 pto,
		"telemetry_timeout_ms":            telTimeout,
		"interface_snapshot_timeout_ms":   ifaceTimeout,
		"olt_if_derived_pon_timeout_ms":   oltTimeout,
		"olt_onu_telnet_timeout_ms":         oltOnuTelnetTimeout,
		"mikrotik_timeout_ms":             mikrotikTimeout,
		"bng_timeout_ms":                  bngTimeout,
		"icmp_payload_bytes":              icmpPB,
		"offline_ping_fail_threshold":     offTh,
		"uptime_restart_alert_minutes":    uptimeRestart,
		"ping_parallel":                   pingParallel,
		"pipeline_steps":                  json.RawMessage(stepsJSON),
	}
	s.appendAuditLog(r.Context(), "monitoring_intervals", "1", "patch", s.actorFromRequest(r), nil, payload)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) getMonitoringSettings(w http.ResponseWriter, r *http.Request) {
	var vps, timeout int
	var raw []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT vps_latency_offset_ms, internet_check_targets, internet_check_timeout_ms FROM monitoring_settings WHERE id=1
	`).Scan(&vps, &raw, &timeout)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vps_latency_offset_ms":      vps,
		"internet_check_targets":     json.RawMessage(raw),
		"internet_check_timeout_ms": timeout,
	})
}

func (s *Server) patchMonitoringSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	// leitura atual
	var vps, timeout int
	var targets []byte
	if err := s.DB().QueryRow(r.Context(), `SELECT vps_latency_offset_ms, internet_check_targets, internet_check_timeout_ms FROM monitoring_settings WHERE id=1`).Scan(&vps, &targets, &timeout); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["vps_latency_offset_ms"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_FIELD", "vps_latency_offset_ms", nil)
			return
		}
		if n < 0 {
			writeErr(w, 422, "VALIDATION", "vps_latency_offset_ms >= 0", nil)
			return
		}
		vps = n
	}
	if v, ok := body["internet_check_targets"]; ok {
		targets = v
	}
	if v, ok := body["internet_check_timeout_ms"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_FIELD", "internet_check_timeout_ms", nil)
			return
		}
		if n <= 0 || n > 30000 {
			writeErr(w, 422, "VALIDATION", "internet_check_timeout_ms entre 1 e 30000", nil)
			return
		}
		timeout = n
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE monitoring_settings SET
			vps_latency_offset_ms = $1,
			internet_check_targets = $2::jsonb,
			internet_check_timeout_ms = $3,
			updated_at = now()
		WHERE id = 1
	`, vps, targets, timeout)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "monitoring_settings", "1", "patch", s.actorFromRequest(r), nil, map[string]any{
		"vps_latency_offset_ms": vps, "internet_check_timeout_ms": timeout,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
