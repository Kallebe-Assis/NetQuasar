// Package alertnotify centraliza enriquecimento de meta (mudança de estado) e envio Telegram de monitorização com confirmação persistida em alert_instances.meta.
package alertnotify

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
	case strings.Contains(hay, "pppoe"):
		return "PPPoE"
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

func resolutionHeader(alertType, title, detail string) string {
	_ = alertType
	_ = title
	_ = detail
	return "🟢 ALERTA RESOLVIDO"
}

// ResolutionStatusLine mensagem personalizada por tipo (corpo do Telegram de resolução).
func ResolutionStatusLine(alertType, originalMessage string) string {
	low := strings.ToLower(strings.TrimSpace(originalMessage))
	switch strings.TrimSpace(alertType) {
	case "ping_unreachable":
		return "Equipamento online — voltou a responder (ICMP/TCP)"
	case "latency_high":
		return "Latência normalizada"
	case "interface_down_transition":
		if tgt := incidentTarget(originalMessage); tgt != "" {
			return tgt + " — operação UP"
		}
		return "Interface voltou a operação UP"
	case "olt_onu_drop":
		if strings.Contains(low, "pon") {
			if tgt := incidentTarget(originalMessage); tgt != "" {
				return tgt + " — ONUs online normalizadas"
			}
		}
		return "ONUs online normalizadas"
	case "bng_subscriber_drop":
		return "Logins BNG normalizados"
	case "olt_onu_rise":
		return "Variação de ONUs online normalizada"
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx":
		return "Potência óptica SFP dentro do limiar"
	case "telemetry_threshold":
		return "Telemetria dentro do intervalo normal"
	case "uptime_restart_low":
		return "Uptime estável — equipamento sem reinício recente"
	case "olt_onu_optical":
		return "Potência óptica ONU normalizada"
	default:
		if strings.Contains(low, "pon") && (strings.Contains(low, "up") || strings.Contains(low, "oper")) {
			return "PON UP"
		}
		return "Situação normalizada"
	}
}

func alertTelegramContextFromRow(ctx context.Context, pool *pgxpool.Pool, alertID uuid.UUID) (alertType string, meta map[string]any) {
	if pool == nil {
		return "", nil
	}
	var metaRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(alert_type, ''), COALESCE(meta::text, '{}')
		FROM alert_instances WHERE id = $1
	`, alertID).Scan(&alertType, &metaRaw)
	if err != nil {
		return "", nil
	}
	meta = map[string]any{}
	_ = json.Unmarshal(metaRaw, &meta)
	return strings.TrimSpace(alertType), meta
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
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
			return s
		}
	}
	if ms := formatLatencyMsFromMeta(meta, "curr_latency_ms", "probe_latency_ms"); ms != "" {
		return ms
	}
	if verify, ok := meta["verify"].(map[string]any); ok {
		if ms := formatLatencyMsFromMeta(verify, "latency_ms"); ms != "" {
			return ms
		}
		if v, ok := verify["dbm"]; ok && v != nil {
			return fmt.Sprintf("%v dBm", v)
		}
		if v, ok := verify["value"]; ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	if alertType != "latency_high" {
		if v, ok := meta["value"]; ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	if v, ok := meta["dbm"]; ok && v != nil {
		return fmt.Sprintf("%v dBm", v)
	}
	_ = alertType
	return ""
}

func formatLatencyMsFromMeta(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := meta[key]; ok && v != nil {
			switch n := v.(type) {
			case int64:
				if n > 0 {
					return fmt.Sprintf("%d ms", n)
				}
			case int:
				if n > 0 {
					return fmt.Sprintf("%d ms", n)
				}
			case float64:
				if n > 0 {
					return fmt.Sprintf("%d ms", int64(n))
				}
			default:
				s := strings.TrimSpace(fmt.Sprint(v))
				if s != "" && s != "0" {
					if strings.HasSuffix(s, " ms") {
						return s
					}
					return s + " ms"
				}
			}
		}
	}
	return ""
}

func resolutionMetricLabel(alertType, title, detail string) string {
	switch strings.TrimSpace(alertType) {
	case "latency_high", "ping_unreachable":
		return "Latência"
	case "mikrotik_sfp_tx", "mikrotik_sfp_rx", "olt_onu_optical":
		return "Potência"
	case "telemetry_threshold":
		return metricLabel(title, detail)
	default:
		return metricLabel(title, detail)
	}
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

func metaFloat(meta map[string]any, keys ...string) float64 {
	if meta == nil {
		return 0
	}
	for _, key := range keys {
		v, ok := meta[key]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			if n != 0 {
				return n
			}
		case int:
			if n != 0 {
				return float64(n)
			}
		case int64:
			if n != 0 {
				return float64(n)
			}
		default:
			s := strings.TrimSpace(fmt.Sprint(v))
			if s == "" || s == "0" {
				continue
			}
			if f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil && f != 0 {
				return f
			}
		}
	}
	return 0
}

func metaInt(meta map[string]any, keys ...string) int {
	f := metaFloat(meta, keys...)
	if f <= 0 {
		return 0
	}
	return int(f)
}

func metaString(meta map[string]any, keys ...string) string {
	if meta == nil {
		return ""
	}
	for _, key := range keys {
		v, ok := meta[key]
		if !ok || v == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func appendUptimeTelegramLines(parts []string, meta map[string]any, title, inc string) []string {
	parts = append(parts, "• Possível reinício — uptime abaixo do limiar")
	observed := metaFloat(meta, "observed_uptime_minutes", "uptime_minutes", "value")
	threshold := metaInt(meta, "threshold_minutes")
	if observed <= 0 {
		if m := regexp.MustCompile(`(-?\d+(?:[.,]\d+)?)\s*(?:min(?:utos)?)?`).FindStringSubmatch(inc); len(m) >= 2 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				observed = f
			}
		}
	}
	if observed > 0 {
		line := fmt.Sprintf("• Uptime = %.0f min", observed)
		if threshold > 0 {
			line += fmt.Sprintf(" (limite %d min)", threshold)
		}
		parts = append(parts, line)
		return parts
	}
	if vt := metaString(meta, "value_text"); vt != "" {
		parts = append(parts, "• Uptime = "+vt)
	} else if strings.Contains(strings.ToLower(title+" "+inc), "uptime") {
		parts = append(parts, "• Uptime abaixo do limiar configurado")
	}
	return parts
}

func appendOltOnuDeltaTelegramLines(parts []string, alertType string, meta map[string]any, inc string) []string {
	pon := metaString(meta, "pon")
	if pon == "" {
		if tgt := incidentTarget(inc); strings.HasPrefix(strings.ToLower(tgt), "pon ") {
			pon = strings.TrimSpace(strings.TrimPrefix(tgt, "PON"))
		}
	}
	if pon != "" {
		parts = append(parts, "• PON "+pon)
	}

	delta := metaFloat(meta, "drop_online_count")
	pct := metaFloat(meta, "drop_online_pct")
	verb := "Queda"
	if alertType == "olt_onu_rise" {
		delta = metaFloat(meta, "rise_online_count")
		pct = metaFloat(meta, "rise_online_pct")
		verb = "Subida"
	}
	prev := metaFloat(meta, "prev_online")
	cur := metaFloat(meta, "curr_online")

	if delta <= 0 && pct <= 0 {
		if m := regexp.MustCompile(`(?i)(?:queda|subida)\s+de\s+(-?\d+(?:[.,]\d+)?)\s+ONUs`).FindStringSubmatch(inc); len(m) >= 2 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				delta = f
			}
		}
		if m := regexp.MustCompile(`\((-?\d+(?:[.,]\d+)?)\s*%\s*de`).FindStringSubmatch(inc); len(m) >= 2 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				pct = f
			}
		}
		if m := regexp.MustCompile(`(?i)(?:de|vs\.?)\s+(-?\d+(?:[.,]\d+)?)\)`).FindStringSubmatch(inc); len(m) >= 2 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil && prev <= 0 {
				prev = f
			}
		}
	}

	if delta > 0 {
		line := fmt.Sprintf("• %s de %.0f ONUs", verb, delta)
		if pct > 0 {
			line += fmt.Sprintf(" (%.0f%%)", pct)
		}
		parts = append(parts, line)
	} else if pct > 0 {
		parts = append(parts, fmt.Sprintf("• %s = %.0f%%", verb, pct))
	}
	if prev > 0 || cur > 0 {
		parts = append(parts, fmt.Sprintf("• Online: %.0f → %.0f", prev, cur))
	}
	return parts
}

func bngSubscriberFieldLabel(field string) string {
	switch strings.TrimSpace(field) {
	case "pppoe_online":
		return "PPPoE"
	case "ipv4_online":
		return "IPv4"
	case "ipv6_online":
		return "IPv6"
	case "total_online":
		return "Total"
	case "dual_stack_online":
		return "Dual-stack"
	default:
		return ""
	}
}

func appendBngSubscriberDropTelegramLines(parts []string, meta map[string]any, title, inc string) []string {
	field := metaString(meta, "subscriber_field")
	label := bngSubscriberFieldLabel(field)
	if label == "" {
		if i := strings.LastIndex(title, "—"); i >= 0 {
			label = strings.TrimSpace(title[i+len("—"):])
		} else if i := strings.LastIndex(title, "-"); i >= 0 {
			label = strings.TrimSpace(title[i+1:])
		}
	}
	if label == "" {
		label = "logins"
	}

	drop := metaFloat(meta, "drop_count")
	prev, hasPrev := metaNumber(meta, "prev_online")
	cur, hasCur := metaNumber(meta, "curr_online")

	if drop <= 0 {
		if m := regexp.MustCompile(`(?i)queda\s+de\s+(\d+(?:[.,]\d+)?)\s+`).FindStringSubmatch(inc); len(m) >= 2 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				drop = f
			}
		}
	}
	if !hasPrev || !hasCur {
		if m := regexp.MustCompile(`\((\d+(?:[.,]\d+)?)\s*→\s*(\d+(?:[.,]\d+)?)\)`).FindStringSubmatch(inc); len(m) >= 3 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
				prev, hasPrev = f, true
			}
			if f, err := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", "."), 64); err == nil {
				cur, hasCur = f, true
			}
		}
	}

	parts = append(parts, fmt.Sprintf("• Queda de logins BNG — %s", label))
	if drop > 0 {
		sess := "sessões"
		if strings.EqualFold(label, "PPPoE") {
			sess = "PPPoEs"
		}
		parts = append(parts, fmt.Sprintf("• Queda de %.0f %s", drop, sess))
	}
	if hasPrev && hasCur {
		parts = append(parts, fmt.Sprintf("• Online: %.0f → %.0f", prev, cur))
	}
	return parts
}

func metaNumber(meta map[string]any, key string) (float64, bool) {
	if meta == nil {
		return 0, false
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
}

func telegramMonitoringBlocks(level, title, message string, equipFallback string, ipFallback string) string {
	return telegramMonitoringBlocksWithContext(level, title, message, equipFallback, ipFallback, "", nil)
}

func telegramMonitoringBlocksWithContext(level, title, message string, equipFallback string, ipFallback string, alertType string, meta map[string]any) string {
	header := monitoringHeader(level, title, message)
	eq, ip, inc, val := shortEquipmentAndIncident(message)
	if strings.TrimSpace(equipFallback) != "" {
		eq = strings.TrimSpace(equipFallback)
	}
	if strings.TrimSpace(ipFallback) != "" {
		ip = strings.TrimSpace(ipFallback)
	}
	parts := []string{header, "", "• " + eq, "• " + ip}

	alertType = strings.TrimSpace(alertType)
	switch alertType {
	case "uptime_restart_low":
		parts = appendUptimeTelegramLines(parts, meta, title, inc)
	case "telemetry_threshold":
		metricID := metaString(meta, "metric_id")
		if metricID == "uptime_minutes" {
			parts = append(parts, "• Uptime abaixo do limiar configurado")
			if observed := metaFloat(meta, "value"); observed > 0 {
				parts = append(parts, fmt.Sprintf("• Uptime = %.0f min", observed))
			} else if vt := metaString(meta, "value_text"); vt != "" {
				parts = append(parts, "• Uptime = "+vt)
			}
		} else {
			if tgt := incidentTarget(inc); tgt != "" && !strings.Contains(strings.ToLower(header), "offline") {
				parts = append(parts, "• "+tgt)
			}
			cause := strings.TrimSpace(title)
			if cause != "" {
				parts = append(parts, "• "+cause)
			}
			if val != "-" && !strings.Contains(strings.ToLower(header), "offline") {
				parts = append(parts, fmt.Sprintf("• %s = %s", metricLabel(title, inc), val))
			} else if vt := metaString(meta, "value_text"); vt != "" {
				parts = append(parts, fmt.Sprintf("• %s = %s", metricLabel(title, inc), vt))
			}
		}
	case "olt_onu_drop", "olt_onu_rise":
		parts = appendOltOnuDeltaTelegramLines(parts, alertType, meta, inc)
	case "bng_subscriber_drop":
		parts = appendBngSubscriberDropTelegramLines(parts, meta, title, inc)
	default:
		if tgt := incidentTarget(inc); tgt != "" && !strings.Contains(strings.ToLower(header), "offline") {
			parts = append(parts, "• "+tgt)
		}
		if val != "-" && !strings.Contains(strings.ToLower(header), "offline") {
			parts = append(parts, fmt.Sprintf("• %s = %s", metricLabel(title, inc), val))
		}
	}

	parts = append(parts, "", "===============")
	return strings.Join(parts, "\n")
}

func buildMonitoringText(level, title, message string) string {
	return telegramMonitoringBlocks(level, title, message, "", "")
}

func buildResolutionText(alertType, title, detail string) string {
	return telegramResolutionBlocks(alertType, title, detail, "", "", "")
}

type telegramMessageRef struct {
	ChatID    string
	MessageID int64
}

func jsonNumberToInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	default:
		return 0
	}
}

func telegramRefFromMap(meta map[string]any, key string) (telegramMessageRef, bool) {
	if meta == nil {
		return telegramMessageRef{}, false
	}
	raw, ok := meta[key].(map[string]any)
	if !ok || raw == nil {
		return telegramMessageRef{}, false
	}
	if skipped, _ := raw["skipped"].(bool); skipped {
		return telegramMessageRef{}, false
	}
	if okVal, exists := raw["ok"]; exists {
		if b, ok := okVal.(bool); ok && !b {
			return telegramMessageRef{}, false
		}
	}
	messageID := jsonNumberToInt64(raw["message_id"])
	if messageID <= 0 {
		return telegramMessageRef{}, false
	}
	chatID := strings.TrimSpace(fmt.Sprint(raw["chat_id"]))
	if chatID == "" || chatID == "<nil>" {
		return telegramMessageRef{}, false
	}
	return telegramMessageRef{ChatID: chatID, MessageID: messageID}, true
}

func telegramRefFromMeta(meta map[string]any) (telegramMessageRef, bool) {
	if ref, ok := telegramRefFromMap(meta, "telegram_problem_ref"); ok {
		return ref, true
	}
	return telegramRefFromMap(meta, "telegram")
}

// telegramResolvedEditBlocks texto para editMessageText na mensagem original do alerta.
func telegramResolvedEditBlocks(alertType, title, detail string, equipFallback string, activeSince time.Time, closedAt *time.Time) string {
	header := resolutionHeader(alertType, title, detail)
	eq, _, inc, _ := shortEquipmentAndIncident(detail)
	if strings.TrimSpace(equipFallback) != "" {
		eq = strings.TrimSpace(equipFallback)
	}
	incLine := strings.TrimSpace(inc)
	if incLine == "" {
		incLine = strings.TrimSpace(title)
	}
	parts := []string{header, "", eq, incLine, ""}
	if !activeSince.IsZero() {
		parts = append(parts, "Início: "+activeSince.UTC().Format("15:04"))
	}
	if closedAt != nil {
		parts = append(parts, "Fim: "+closedAt.UTC().Format("15:04"))
		if d := closedAt.Sub(activeSince); d > 0 {
			parts = append(parts, "Duração: "+formatDurationPT(d))
		}
	}
	parts = append(parts, "", "===============")
	return strings.Join(parts, "\n")
}

func telegramResolutionReplyShort(alertType string, duration time.Duration) string {
	if duration <= 0 {
		if alertType == "ping_unreachable" {
			return "✅ Equipamento online."
		}
		return "✅ Alerta resolvido."
	}
	dur := formatDurationPT(duration)
	if alertType == "ping_unreachable" {
		return "✅ Equipamento online após " + dur + "."
	}
	return "✅ Resolvido após " + dur + "."
}

func telegramResolutionBlocks(alertType, title, detail string, equipFallback string, ipFallback string, currentValue string, extras ...string) string {
	header := resolutionHeader(alertType, title, detail)
	eq, ip, _, _ := shortEquipmentAndIncident(detail)
	if strings.TrimSpace(equipFallback) != "" {
		eq = strings.TrimSpace(equipFallback)
	}
	if strings.TrimSpace(ipFallback) != "" {
		ip = strings.TrimSpace(ipFallback)
	}
	statusLine := ResolutionStatusLine(alertType, detail)
	parts := []string{header, "", "• " + eq, "• " + ip, "• " + statusLine}
	if cv := strings.TrimSpace(currentValue); cv != "" {
		label := resolutionMetricLabel(alertType, title, detail)
		parts = append(parts, fmt.Sprintf("• %s = %s", label, cv))
	}
	for _, line := range extras {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, "• "+line)
		}
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
	alertType, meta := alertTelegramContextFromRow(ctx, pool, alertID)
	text := telegramMonitoringBlocksWithContext(level, title, message, eqName, eqIP, alertType, meta)
	sent, sendErr := telegramclient.SendMessageWithResult(ctx, cfg, text, telegramclient.SendOpts{})
	tg := map[string]any{"attempted_at": attempted, "text": text}
	if sendErr != nil {
		tg["ok"] = false
		tg["error"] = sendErr.Error()
		if log != nil {
			log.Warn().Err(sendErr).Str("alert_id", alertID.String()).Msg("envio Telegram monitorização falhou")
		}
	} else {
		tg["ok"] = true
		tg["created_at"] = attempted
		if sent.ChatID != "" {
			tg["chat_id"] = sent.ChatID
		} else {
			tg["chat_id"] = cfg.ChatID
		}
		if sent.MessageID > 0 {
			tg["message_id"] = sent.MessageID
		}
	}
	patch := map[string]any{"telegram": tg}
	if tg["ok"] == true {
		ref := map[string]any{}
		if cid, ok := tg["chat_id"]; ok {
			ref["chat_id"] = cid
		}
		if mid, ok := tg["message_id"]; ok {
			ref["message_id"] = mid
		}
		if len(ref) > 0 {
			ref["ok"] = true
			patch["telegram_problem_ref"] = ref
		}
	}
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
	case "bng_subscriber_drop":
		return "Contagem de logins BNG normalizada"
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
	currentValue := ""
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
		currentValue = rv
	}

	tg := map[string]any{"attempted_at": attempted}
	origRef, hasOrig := telegramRefFromMeta(row.Meta)

	if hasOrig {
		text := telegramResolutionBlocks(row.AlertType, title, detail, eqName, eqIP, currentValue, extras...)
		sent, sendErr := telegramclient.SendMessageWithResult(ctx, cfg, text, telegramclient.SendOpts{
			ReplyToMessageID: origRef.MessageID,
		})
		tg["reply_to_problem_message_id"] = origRef.MessageID
		tg["text"] = text
		if sendErr != nil {
			tg["ok"] = false
			tg["error"] = sendErr.Error()
			if log != nil {
				log.Warn().Err(sendErr).Str("alert_id", alertID.String()).Int64("reply_to", origRef.MessageID).
					Msg("telegram resolução (reply ao alerta original) falhou")
			}
		} else {
			tg["ok"] = true
			if sent.MessageID > 0 {
				tg["message_id"] = sent.MessageID
			}
			if sent.ChatID != "" {
				tg["chat_id"] = sent.ChatID
			} else {
				tg["chat_id"] = origRef.ChatID
			}
		}
	} else {
		text := telegramResolutionBlocks(row.AlertType, title, detail, eqName, eqIP, currentValue, extras...)
		sendErr := telegramclient.SendMessage(ctx, cfg, text)
		tg["fallback_full_message"] = true
		tg["text"] = text
		if sendErr != nil {
			tg["ok"] = false
			tg["error"] = sendErr.Error()
			if log != nil {
				log.Warn().Err(sendErr).Str("alert_id", alertID.String()).Msg("telegram resolução falhou")
			}
		} else {
			tg["ok"] = true
		}
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
