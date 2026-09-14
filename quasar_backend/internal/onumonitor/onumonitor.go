// Package onumonitor avalia as ONUs colocadas em monitoramento manual/temporário (tela OLT →
// ONUs → "Monitorar", tabela onu_monitors) e levanta/fecha alertas de status e potência RX,
// opcionalmente confirmando um login PPPoE no BNG na normalização.
package onumonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

const (
	AlertTypeOffline = "onu_monitor_offline"
	AlertTypeRxLow   = "onu_monitor_rx_low"
)

// RxClass classifica a potência RX igual ao frontend (lib/onuRxQuality.ts): dbm <= bad → "ruim";
// dbm <= good → "aceitavel"; senão "bom".
func RxClass(dbm, good, bad float64) string {
	if dbm <= bad {
		return "ruim"
	}
	if dbm <= good {
		return "aceitavel"
	}
	return "bom"
}

// State é o estado corrente de uma ONU lido do último snapshot da OLT.
type State struct {
	Found   bool
	Online  *bool
	RxDbm   *float64
	RxClass string
}

// StateFromSummary procura a ONU pelo serial no vsol_onu_rows do snapshot e devolve status + RX.
func StateFromSummary(summaryJSON []byte, serial string, rxGood, rxBad float64) State {
	target := strings.ToUpper(strings.TrimSpace(serial))
	if target == "" {
		return State{}
	}
	for _, raw := range vsolparse.VsolOnuRowsFromSummaryBlob(summaryJSON) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(stringFromAny(row["serial"]))) != target {
			continue
		}
		st := State{Found: true}
		if b, ok := row["online"].(bool); ok {
			st.Online = &b
		}
		if dbm, ok := floatFromAny(row["rx_dbm"], row["rx_pwr"], row["rx"]); ok {
			st.RxDbm = &dbm
			st.RxClass = RxClass(dbm, rxGood, rxBad)
		}
		return st
	}
	return State{}
}

type monitorRow struct {
	ID             uuid.UUID
	Serial         string
	OltDeviceID    *uuid.UUID
	OltDescription string
	Pon            *int
	Onu            *int
	ClientName     string
	WatchStatus    bool
	WatchRx        bool
	WatchLogin     bool
	BngLogin       string
	NotifyTelegram bool
	ExpiresAt      *time.Time
	LastOnline     *bool
	LastRxDbm      *float64
	LastRxClass    string
}

// Evaluate roda periodicamente (ver monitorworker.TryEvaluateOnuMonitors): expira monitores
// vencidos, lê o estado corrente de cada ONU monitorada e abre/fecha alertas conforme as
// transições de status e de classe de RX.
func Evaluate(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger) error {
	if pool == nil {
		return fmt.Errorf("onumonitor: pool nulo")
	}

	var rxGood, rxBad float64
	if err := pool.QueryRow(ctx, `SELECT onu_rx_good_dbm, onu_rx_bad_dbm FROM monitoring_settings WHERE id=1`).Scan(&rxGood, &rxBad); err != nil {
		rxGood, rxBad = -23, -27
	}

	// Expira e limpa monitores vencidos (fecha os alertas abertos deles primeiro).
	expRows, err := pool.Query(ctx, `SELECT id, olt_device_id FROM onu_monitors WHERE expires_at IS NOT NULL AND expires_at < now()`)
	if err == nil {
		type exp struct {
			id  uuid.UUID
			dev *uuid.UUID
		}
		var expired []exp
		for expRows.Next() {
			var e exp
			if err := expRows.Scan(&e.id, &e.dev); err == nil {
				expired = append(expired, e)
			}
		}
		expRows.Close()
		for _, e := range expired {
			closeMonitorAlerts(ctx, pool, log, e)
			_, _ = pool.Exec(ctx, `DELETE FROM onu_monitors WHERE id=$1`, e.id)
			if log != nil {
				log.Info().Str("monitor", e.id.String()).Msg("onu_monitor: monitoramento temporário expirou e foi removido")
			}
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT id, serial, olt_device_id, olt_description, pon, onu, client_name,
			watch_status, watch_rx, watch_login, bng_login, notify_telegram, expires_at,
			last_online, last_rx_dbm, last_rx_class
		FROM onu_monitors
		ORDER BY created_at
	`)
	if err != nil {
		return err
	}
	var monitors []monitorRow
	for rows.Next() {
		var m monitorRow
		if err := rows.Scan(&m.ID, &m.Serial, &m.OltDeviceID, &m.OltDescription, &m.Pon, &m.Onu, &m.ClientName,
			&m.WatchStatus, &m.WatchRx, &m.WatchLogin, &m.BngLogin, &m.NotifyTelegram, &m.ExpiresAt,
			&m.LastOnline, &m.LastRxDbm, &m.LastRxClass); err != nil {
			rows.Close()
			return err
		}
		monitors = append(monitors, m)
	}
	rows.Close()
	if len(monitors) == 0 {
		return nil
	}

	// Um snapshot por OLT (cache local nesta passagem).
	snapCache := map[uuid.UUID][]byte{}
	getSnap := func(dev uuid.UUID) []byte {
		if b, ok := snapCache[dev]; ok {
			return b
		}
		var s string
		_ = pool.QueryRow(ctx, `SELECT COALESCE(summary::text, '{}') FROM olt_snapshots WHERE device_id=$1`, dev).Scan(&s)
		snapCache[dev] = []byte(s)
		return []byte(s)
	}

	for _, m := range monitors {
		if m.OltDeviceID == nil {
			continue
		}
		st := StateFromSummary(getSnap(*m.OltDeviceID), m.Serial, rxGood, rxBad)
		if !st.Found {
			continue // ONU ainda não vista neste snapshot — não mexe no baseline
		}
		evalOne(ctx, pool, log, m, st)
	}
	return nil
}

func evalOne(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, m monitorRow, st State) {
	dev := *m.OltDeviceID
	where := fmt.Sprintf("%s (%s, PON %s/ONU %s)",
		orDash(m.ClientName), m.Serial, intPtrStr(m.Pon), intPtrStr(m.Onu))

	// ---- Status online/offline ----
	if m.WatchStatus && st.Online != nil {
		key := "onu_monitor:" + m.ID.String() + ":status"
		if !*st.Online && (m.LastOnline == nil || *m.LastOnline) {
			openAlert(ctx, pool, log, dev, onumOfflineSpec(m, where, key))
		} else if *st.Online && m.LastOnline != nil && !*m.LastOnline {
			detail := fmt.Sprintf("ONU %s voltou a ficar ONLINE.%s", where, loginConfirmSuffix(ctx, pool, m))
			closeAlert(ctx, pool, log, dev, AlertTypeOffline, key, detail)
		}
	}

	// ---- Potência RX ----
	if m.WatchRx && st.RxDbm != nil {
		key := "onu_monitor:" + m.ID.String() + ":rx"
		cur := st.RxClass
		prev := strings.TrimSpace(m.LastRxClass)
		if cur == "ruim" && prev != "ruim" {
			openAlert(ctx, pool, log, dev, onumRxLowSpec(m, where, key, *st.RxDbm))
		} else if cur == "bom" && prev == "ruim" {
			detail := fmt.Sprintf("RX da ONU %s normalizou: %.2f dBm (bom).%s", where, *st.RxDbm, loginConfirmSuffix(ctx, pool, m))
			closeAlert(ctx, pool, log, dev, AlertTypeRxLow, key, detail)
		}
	}

	// ---- Baseline ----
	_, _ = pool.Exec(ctx, `
		UPDATE onu_monitors SET
			last_online = $2, last_rx_dbm = $3, last_rx_class = NULLIF($4,''), last_evaluated_at = now(), updated_at = now()
		WHERE id = $1
	`, m.ID, st.Online, nullFloat(st.RxDbm), st.RxClass)
}

type openSpec struct {
	severity  string
	alertType string
	message   string
	metaKey   string
	meta      map[string]any
	headline  string
	notify    bool
}

func onumOfflineSpec(m monitorRow, where, key string) openSpec {
	return openSpec{
		severity:  "warning",
		alertType: AlertTypeOffline,
		message:   fmt.Sprintf("ONU offline — %s na OLT %s.", where, orDash(m.OltDescription)),
		metaKey:   key,
		meta: map[string]any{
			"key": key, "monitor_id": m.ID.String(), "serial": m.Serial,
			"client_name": m.ClientName, "pon": m.Pon, "onu": m.Onu, "kind": "status",
		},
		headline: "ONU em monitoramento ficou OFFLINE",
		notify:   m.NotifyTelegram,
	}
}

func onumRxLowSpec(m monitorRow, where, key string, dbm float64) openSpec {
	return openSpec{
		severity:  "warning",
		alertType: AlertTypeRxLow,
		message:   fmt.Sprintf("RX baixo — %s: %.2f dBm na OLT %s.", where, dbm, orDash(m.OltDescription)),
		metaKey:   key,
		meta: map[string]any{
			"key": key, "monitor_id": m.ID.String(), "serial": m.Serial,
			"client_name": m.ClientName, "pon": m.Pon, "onu": m.Onu, "kind": "rx", "rx_dbm": dbm,
		},
		headline: "ONU em monitoramento com RX baixo",
		notify:   m.NotifyTelegram,
	}
}

func openAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, dev uuid.UUID, s openSpec) {
	var notify *alertstore.NotifyCreate
	if s.notify {
		notify = &alertstore.NotifyCreate{Log: log, Level: strings.ToUpper(s.severity), Headline: s.headline}
	}
	_, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: dev, Severity: s.severity, AlertType: s.alertType,
		Message: s.message, DeviceName: metaClientName(s.meta), Meta: s.meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: s.metaKey},
	}, notify)
	if err != nil && log != nil {
		log.Debug().Err(err).Str("alert_type", s.alertType).Msg("onu_monitor: abrir alerta")
	}
}

func closeAlert(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, dev uuid.UUID, alertType, key, detail string) {
	// Actualiza a mensagem do alerta aberto para o Telegram de resolução carregar o detalhe da
	// normalização (inclui confirmação do login no BNG quando pedido).
	_ = alertstore.PatchOpenMeta(ctx, pool, alertstore.OpenSpec{
		DeviceID: dev, AlertType: alertType, Message: detail,
		Meta:  map[string]any{"key": key, "resolved": "normalizado"},
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
	})
	_, _, err := alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: dev, AlertType: alertType,
		Match:    alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		Resolved: map[string]any{"resolved": "normalizado", "key": key},
	})
	if err != nil && log != nil {
		log.Debug().Err(err).Str("alert_type", alertType).Msg("onu_monitor: fechar alerta")
	}
}

func closeMonitorAlerts(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, e struct {
	id  uuid.UUID
	dev *uuid.UUID
}) {
	if e.dev == nil {
		return
	}
	for _, at := range []string{AlertTypeOffline, AlertTypeRxLow} {
		for _, suf := range []string{":status", ":rx"} {
			key := "onu_monitor:" + e.id.String() + suf
			_, _, _ = alertstore.Close(ctx, pool, nil, alertstore.CloseSpec{
				DeviceID: *e.dev, AlertType: at,
				Match:    alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
				Resolved: map[string]any{"resolved": "monitor_removido", "key": key},
			})
		}
	}
}

// loginConfirmSuffix consulta bng_known_logins pelo login vinculado e devolve uma frase para
// anexar à mensagem de normalização (" Login x: ONLINE no BNG ..."), ou "" se não aplicável.
func loginConfirmSuffix(ctx context.Context, pool *pgxpool.Pool, m monitorRow) string {
	if !m.WatchLogin || strings.TrimSpace(m.BngLogin) == "" {
		return ""
	}
	login := strings.ToLower(strings.TrimSpace(m.BngLogin))
	var online *bool
	var bngDesc *string
	var lastSeen *time.Time
	err := pool.QueryRow(ctx, `
		SELECT bool_or(k.is_online),
			(array_agg(d.description ORDER BY k.last_seen_at DESC))[1],
			max(k.last_seen_at)
		FROM bng_known_logins k
		JOIN devices d ON d.id = k.device_id
		WHERE lower(trim(k.login)) = $1
	`, login).Scan(&online, &bngDesc, &lastSeen)
	if err != nil || online == nil {
		return fmt.Sprintf(" Login %s: nunca visto em nenhum BNG.", m.BngLogin)
	}
	if *online {
		bng := ""
		if bngDesc != nil && strings.TrimSpace(*bngDesc) != "" {
			bng = " no BNG " + strings.TrimSpace(*bngDesc)
		}
		return fmt.Sprintf(" Login %s: ONLINE%s.", m.BngLogin, bng)
	}
	seen := ""
	if lastSeen != nil {
		seen = " (visto pela última vez em " + lastSeen.Format("02/01 15:04") + ")"
	}
	return fmt.Sprintf(" Login %s: ainda OFFLINE no BNG%s.", m.BngLogin, seen)
}

// ---- helpers ----

func metaClientName(meta map[string]any) string {
	if v, ok := meta["client_name"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	if v, ok := meta["serial"].(string); ok {
		return v
	}
	return ""
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case float64:
		return fmt.Sprintf("%v", x)
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func floatFromAny(vals ...any) (float64, bool) {
	for _, v := range vals {
		switch x := v.(type) {
		case float64:
			if !math.IsNaN(x) && !math.IsInf(x, 0) {
				return x, true
			}
		case json.Number:
			if f, err := x.Float64(); err == nil {
				return f, true
			}
		case string:
			s := strings.TrimSpace(strings.ReplaceAll(x, ",", "."))
			s = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(s, "dBm")), " ")
			var f float64
			if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func intPtrStr(p *int) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *p)
}
