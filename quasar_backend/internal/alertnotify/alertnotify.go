// Package alertnotify centraliza enriquecimento de meta (mudança de estado) e envio Telegram de monitorização com confirmação persistida em alert_instances.meta.
package alertnotify

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertcorrelation"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
	"github.com/rs/zerolog"
)

func levelEmoji(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return "🔴"
	case "warning":
		return "🟡"
	case "info":
		return "🟢"
	default:
		return "🔔"
	}
}

func shortEquipmentAndIncident(message string) (equip string, ip string, incident string, value string) {
	raw := strings.TrimSpace(message)
	if raw == "" {
		return "-", "-", "-", "-"
	}
	reEquipIP := regexp.MustCompile(`^\s*(.*?)\s*\(([^)]+)\)\s*:\s*(.*)$`)
	if m := reEquipIP.FindStringSubmatch(raw); len(m) == 4 {
		equip = strings.TrimSpace(m[1])
		ip = strings.TrimSpace(m[2])
		incident = strings.TrimSpace(m[3])
	} else {
		incident = raw
	}
	parts := strings.SplitN(raw, ":", 2)
	if equip == "" && len(parts) == 2 {
		equip = strings.TrimSpace(parts[0])
	}
	if equip == "" {
		equip = "-"
	}
	if ip == "" {
		reIP := regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
		if m := reIP.FindString(raw); m != "" {
			ip = m
		}
	}
	if ip == "" {
		ip = "-"
	}
	num := strings.ReplaceAll(incident, ",", ".")
	if m := regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s*(dBm|ms|°C|%|ONUs?|min(?:utos)?)`).FindStringSubmatch(num); len(m) >= 2 {
		value = strings.TrimSpace(m[1] + " " + m[2])
	} else {
		value = "-"
	}
	return equip, ip, incident, value
}

func metricLabel(title, incident string) string {
	hay := strings.ToLower(strings.TrimSpace(title + " " + incident))
	switch {
	case strings.Contains(hay, "lat"):
		return "Latência"
	case strings.Contains(hay, "cpu"):
		return "CPU"
	case strings.Contains(hay, "mem"):
		return "Memória"
	case strings.Contains(hay, "temp"):
		return "Temperatura"
	case strings.Contains(hay, "uptime"):
		return "Uptime"
	case strings.Contains(hay, "sfp") && strings.Contains(hay, "tx"):
		return "Potência TX"
	case strings.Contains(hay, "sfp") && strings.Contains(hay, "rx"):
		return "Potência RX"
	case strings.Contains(hay, "onu"):
		return "ONUs"
	default:
		return "Valor"
	}
}

func incidentTarget(incident string) string {
	low := strings.ToLower(incident)
	if m := regexp.MustCompile(`(?i)\bpon\s+([a-z0-9_\/.\-]+)`).FindStringSubmatch(incident); len(m) >= 2 {
		return "PON " + strings.TrimSpace(m[1])
	}
	// Mikrotik SFP: "interface sfp-sfpplus1 — potência SFP …" (nome pode ter @ + etc.)
	if m := regexp.MustCompile(`(?i)\binterface\s+(.+?)\s+[—\-]\s*potência`).FindStringSubmatch(incident); len(m) >= 2 {
		return "Interface " + strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?i)\binterface\s+(\S+)\s+mudou`).FindStringSubmatch(incident); len(m) >= 2 {
		return "Interface " + strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?i)\binterface\s+([a-z0-9_\/.@+\-]+)`).FindStringSubmatch(incident); len(m) >= 2 {
		return "Interface " + strings.TrimSpace(m[1])
	}
	if strings.Contains(low, "sfp") {
		return "Interface SFP"
	}
	return ""
}

func monitoringHeader(level, title, message string) string {
	_ = level
	low := strings.ToLower(strings.TrimSpace(title + " " + message))
	if strings.Contains(low, "offline") || strings.Contains(low, "sem resposta") || strings.Contains(low, "inalcan") {
		return "🔴 EQUIPAMENTO OFFLINE"
	}
	return "🟡 ALERTA TELEMETRIA"
}

func resolutionHeader(title, detail string) string {
	low := strings.ToLower(strings.TrimSpace(title + " " + detail))
	if strings.Contains(low, "voltou a responder") || strings.Contains(low, "icmp/tcp") || strings.Contains(low, "online") {
		return "🟢 EQUIPAMENTO ONLINE"
	}
	return "🟢 ALERTA RESOLVIDO"
}

func equipIPFromAlertRow(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) (equip string, ip string) {
	if pool == nil {
		return "", ""
	}
	var name *string
	var ipStr *string
	err := pool.QueryRow(ctx, `
		SELECT
			COALESCE(NULLIF(trim(a.device_name), ''), NULLIF(trim(d.description), '')),
			COALESCE(NULLIF(trim(a.ip), ''), NULLIF(trim(host(d.ip)::text), ''))
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		WHERE a.id = $1
	`, alertID).Scan(&name, &ipStr)
	if err != nil {
		return "", ""
	}
	if name != nil {
		equip = strings.TrimSpace(*name)
	}
	if ipStr != nil {
		ip = strings.TrimSpace(*ipStr)
	}
	return equip, ip
}

type alertResolutionRow struct {
	ActiveSince time.Time
	ClosedAt    *time.Time
	PopName     string
	Meta        map[string]any
	AlertType   string
}

func loadAlertResolutionRow(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) alertResolutionRow {
	var out alertResolutionRow
	var closed *time.Time
	var metaRaw []byte
	var pop *string
	err := pool.QueryRow(ctx, `
		SELECT a.active_since, a.closed_at, a.alert_type, COALESCE(a.meta::text, '{}'),
			COALESCE(NULLIF(trim(p.description), ''), '')
		FROM alert_instances a
		LEFT JOIN devices d ON d.id = a.device_id
		LEFT JOIN pops p ON p.id = d.pop_id
		WHERE a.id = $1
	`, alertID).Scan(&out.ActiveSince, &closed, &out.AlertType, &metaRaw, &pop)
	if err != nil {
		return out
	}
	out.ClosedAt = closed
	if pop != nil {
		out.PopName = strings.TrimSpace(*pop)
	}
	_ = json.Unmarshal(metaRaw, &out.Meta)
	if out.Meta == nil {
		out.Meta = map[string]any{}
	}
	return out
}

func resolvedValueFromMeta(alertType string, meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["resolved_value"]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	if verify, ok := meta["verify"].(map[string]any); ok {
		if v, ok := verify["dbm"]; ok && v != nil {
			return fmt.Sprintf("%v dBm", v)
		}
		if v, ok := verify["latency_ms"]; ok && v != nil {
			return fmt.Sprintf("%v ms", v)
		}
		if v, ok := verify["value"]; ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	if v, ok := meta["value"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	if v, ok := meta["dbm"]; ok && v != nil {
		return fmt.Sprintf("%v dBm", v)
	}
	_ = alertType
	return ""
}

func formatDurationPT(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	sec := int(d.Round(time.Second).Seconds())
	days := sec / 86400
	sec %= 86400
	hours := sec / 3600
	sec %= 3600
	mins := sec / 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d d", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d h", hours))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d min", mins))
	}
	return strings.Join(parts, " ")
}

func telegramMonitoringBlocks(level, title, message string, equipFallback string, ipFallback string) string {
	header := monitoringHeader(level, title, message)
	eq, ip, inc, val := shortEquipmentAndIncident(message)
	if strings.TrimSpace(equipFallback) != "" {
		eq = strings.TrimSpace(equipFallback)
	}
	if strings.TrimSpace(ipFallback) != "" {
		ip = strings.TrimSpace(ipFallback)
	}
	parts := []string{header, "", "• " + eq, "• " + ip}
	if tgt := incidentTarget(inc); tgt != "" && !strings.Contains(strings.ToLower(header), "offline") {
		parts = append(parts, "• "+tgt)
	}
	if val != "-" && !strings.Contains(strings.ToLower(header), "offline") {
		parts = append(parts, fmt.Sprintf("• %s = %s", metricLabel(title, inc), val))
	}
	parts = append(parts, "", "===============")
	return strings.Join(parts, "\n")
}

func buildMonitoringText(level, title, message string) string {
	return telegramMonitoringBlocks(level, title, message, "", "")
}

func buildResolutionText(title, detail string) string {
	return telegramResolutionBlocks(title, detail, "", "")
}

func telegramResolutionBlocks(title, detail string, equipFallback string, ipFallback string, extras ...string) string {
	header := resolutionHeader(title, detail)
	eq, ip, inc, val := shortEquipmentAndIncident(detail)
	if strings.TrimSpace(equipFallback) != "" {
		eq = strings.TrimSpace(equipFallback)
	}
	if strings.TrimSpace(ipFallback) != "" {
		ip = strings.TrimSpace(ipFallback)
	}
	parts := []string{header, "", "• " + eq, "• " + ip}
	for _, line := range extras {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, "• "+line)
		}
	}
	if tgt := incidentTarget(inc); tgt != "" {
		parts = append(parts, "• "+tgt)
	}
	if val != "-" && strings.Contains(strings.ToLower(header), "alerta") {
		parts = append(parts, fmt.Sprintf("• %s = %s", metricLabel(title, inc), val))
	}
	parts = append(parts, "", "===============")
	return strings.Join(parts, "\n")
}

// WithStatusTransition copia meta e marca transição de estado (persistido em JSON).
func WithStatusTransition(base map[string]any, previousStatus, newStatus string, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+8+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	out["status_transition"] = true
	out["status_transition_at"] = now
	if previousStatus != "" {
		out["previous_status"] = previousStatus
	}
	if newStatus != "" {
		out["new_status"] = newStatus
	}
	return out
}

// SendMonitoringTelegramAndPatchMeta envia via settings «monitoring» e grava em meta.telegram (ok, erro, tentativa).
func SendMonitoringTelegramAndPatchMeta(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, alertID uuid.UUID, level, title, message string) {
	if pool == nil || alertID == uuid.Nil {
		return
	}
	if alertcorrelation.ShouldSkipMonitoringTelegram(ctx, pool, alertID) {
		patchMeta(ctx, pool, alertID, map[string]any{
			"telegram": map[string]any{
				"skipped": true,
				"reason":  "incident_cascade",
			},
		})
		return
	}
	attempted := time.Now().UTC().Format(time.RFC3339Nano)
	cfg, err := telegramclient.LoadConfig(ctx, pool, "monitoring")
	if err != nil || !cfg.Ready() {
		patch := map[string]any{
			"telegram": map[string]any{
				"attempted_at": attempted,
				"skipped":      true,
				"reason":       "config_unavailable",
			},
		}
		patchMeta(ctx, pool, alertID, patch)
		if log != nil {
			log.Debug().Err(err).Str("alert_id", alertID.String()).Msg("telegram monitorização: config indisponível")
		}
		return
	}
	eqName, eqIP := equipIPFromAlertRow(ctx, pool, alertID)
	text := telegramMonitoringBlocks(level, title, message, eqName, eqIP)
	sendErr := telegramclient.SendMessage(ctx, cfg, text)
	tg := map[string]any{"attempted_at": attempted}
	if sendErr != nil {
		tg["ok"] = false
		tg["error"] = sendErr.Error()
		if log != nil {
			log.Warn().Err(sendErr).Str("alert_id", alertID.String()).Msg("envio Telegram monitorização falhou")
		}
	} else {
		tg["ok"] = true
	}
	patch := map[string]any{"telegram": tg}
	patchMeta(ctx, pool, alertID, patch)
}

func patchMeta(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID, patch map[string]any) {
	b, err := json.Marshal(patch)
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, `UPDATE alert_instances SET meta = COALESCE(meta, '{}'::jsonb) || $2::jsonb WHERE id = $1`, alertID, b)
}

// ResolutionHeadlineForAlertType título curto para Telegram/UI quando o alarme é encerrado.
func ResolutionHeadlineForAlertType(alertType string) string {
	switch alertType {
	case "ping_unreachable":
		return "Equipamento voltou a responder (ICMP/TCP)"
	case "uptime_restart_low":
		return "Uptime voltou acima do limiar (equipamento estável)"
	case "latency_high":
		return "Latência voltou ao intervalo normal"
	case "interface_down_transition":
		return "Interface voltou a operação UP"
	case "olt_onu_drop":
		return "Contagem de ONUs online normalizada"
	case "olt_onu_rise":
		return "Variação de ONUs online normalizada"
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx":
		return "Potência óptica SFP dentro do limiar"
	case "telemetry_threshold":
		return "Métrica de telemetria voltou ao intervalo normal"
	default:
		return "Alarme resolvido"
	}
}

// SendResolutionTelegramAndPatchMeta notifica mudança positiva (alarme ativo → resolvido), grava meta.resolution com transição e resultado do Telegram.
func SendResolutionTelegramAndPatchMeta(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, alertID uuid.UUID, title, detail string) {
	if pool == nil || alertID == uuid.Nil {
		return
	}
	attempted := time.Now().UTC().Format(time.RFC3339Nano)
	resBlock := map[string]any{
		"resolved_at":            attempted,
		"status_transition":      true,
		"status_transition_kind": "alarm_resolved",
		"previous_status":        "alarm_active",
		"new_status":             "resolved",
	}
	cfg, err := telegramclient.LoadConfig(ctx, pool, "monitoring")
	if err != nil || !cfg.Ready() {
		resBlock["telegram"] = map[string]any{
			"attempted_at": attempted,
			"skipped":      true,
			"reason":       "config_unavailable",
		}
		patchResolutionBlock(ctx, pool, alertID, resBlock)
		if log != nil {
			log.Debug().Err(err).Str("alert_id", alertID.String()).Msg("telegram resolução: config indisponível")
		}
		return
	}
	eqName, eqIP := equipIPFromAlertRow(ctx, pool, alertID)
	row := loadAlertResolutionRow(ctx, pool, alertID)
	extras := []string{}
	if row.PopName != "" {
		extras = append(extras, "POP: "+row.PopName)
	}
	if !row.ActiveSince.IsZero() {
		extras = append(extras, "Início: "+row.ActiveSince.UTC().Format("02/01/2006 15:04"))
	}
	if row.ClosedAt != nil {
		extras = append(extras, "Fim: "+row.ClosedAt.UTC().Format("02/01/2006 15:04"))
		if d := row.ClosedAt.Sub(row.ActiveSince); d > 0 {
			extras = append(extras, "Duração: "+formatDurationPT(d))
		}
	}
	if rv := resolvedValueFromMeta(row.AlertType, row.Meta); rv != "" {
		extras = append(extras, "Valor normalizado: "+rv)
	}
	text := telegramResolutionBlocks(title, detail, eqName, eqIP, extras...)
	sendErr := telegramclient.SendMessage(ctx, cfg, text)
	tg := map[string]any{"attempted_at": attempted}
	if sendErr != nil {
		tg["ok"] = false
		tg["error"] = sendErr.Error()
		if log != nil {
			log.Warn().Err(sendErr).Str("alert_id", alertID.String()).Msg("telegram resolução falhou")
		}
	} else {
		tg["ok"] = true
	}
	resBlock["telegram"] = tg
	patchResolutionBlock(ctx, pool, alertID, resBlock)
}

func patchResolutionBlock(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID, resBlock map[string]any) {
	b, err := json.Marshal(map[string]any{"resolution": resBlock})
	if err != nil {
		return
	}
	_, _ = pool.Exec(ctx, `UPDATE alert_instances SET meta = COALESCE(meta, '{}'::jsonb) || $2::jsonb WHERE id = $1`, alertID, b)
}

// SeverityRank compara severidades (para escalonamento).
func SeverityRank(sev string) int {
	switch sev {
	case "critical":
		return 2
	case "warning":
		return 1
	case "info":
		return 0
	default:
		return 0
	}
}
