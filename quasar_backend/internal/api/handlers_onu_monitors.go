package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/onumonitor"
)

type onuMonitorRow struct {
	ID             uuid.UUID  `json:"id"`
	Serial         string     `json:"serial"`
	OltDeviceID    *uuid.UUID `json:"olt_device_id,omitempty"`
	OltDescription string     `json:"olt_description"`
	Pon            *int       `json:"pon,omitempty"`
	Onu            *int       `json:"onu,omitempty"`
	ClientName     string     `json:"client_name"`
	WatchStatus    bool       `json:"watch_status"`
	WatchRx        bool       `json:"watch_rx"`
	WatchLogin     bool       `json:"watch_login"`
	BngLogin       string     `json:"bng_login"`
	NotifyTelegram bool       `json:"notify_telegram"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`

	// Estado corrente (lido ao vivo do último snapshot da OLT + bng_known_logins).
	CurrentOnline  *bool    `json:"current_online,omitempty"`
	CurrentRxDbm   *float64 `json:"current_rx_dbm,omitempty"`
	CurrentRxClass string   `json:"current_rx_class,omitempty"`
	LoginOnline    *bool    `json:"login_online,omitempty"`
	LoginBng       string   `json:"login_bng,omitempty"`
}

// listOnuMonitors devolve as ONUs em monitoramento manual + o estado corrente de cada uma.
func (s *Server) listOnuMonitors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var rxGood, rxBad float64
	if err := s.DB().QueryRow(ctx, `SELECT onu_rx_good_dbm, onu_rx_bad_dbm FROM monitoring_settings WHERE id=1`).Scan(&rxGood, &rxBad); err != nil {
		rxGood, rxBad = -23, -27
	}
	rows, err := s.DB().Query(ctx, `
		SELECT id, serial, olt_device_id, olt_description, pon, onu, client_name,
			watch_status, watch_rx, watch_login, bng_login, notify_telegram, expires_at,
			created_by, created_at
		FROM onu_monitors
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	out := []onuMonitorRow{}
	snapCache := map[uuid.UUID][]byte{}
	for rows.Next() {
		var m onuMonitorRow
		if err := rows.Scan(&m.ID, &m.Serial, &m.OltDeviceID, &m.OltDescription, &m.Pon, &m.Onu, &m.ClientName,
			&m.WatchStatus, &m.WatchRx, &m.WatchLogin, &m.BngLogin, &m.NotifyTelegram, &m.ExpiresAt,
			&m.CreatedBy, &m.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if m.OltDeviceID != nil {
			blob, ok := snapCache[*m.OltDeviceID]
			if !ok {
				var sum string
				_ = s.DB().QueryRow(ctx, `SELECT COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, *m.OltDeviceID).Scan(&sum)
				blob = []byte(sum)
				snapCache[*m.OltDeviceID] = blob
			}
			st := onumonitor.StateFromSummary(blob, m.Serial, rxGood, rxBad)
			if st.Found {
				m.CurrentOnline = st.Online
				m.CurrentRxDbm = st.RxDbm
				m.CurrentRxClass = st.RxClass
			}
		}
		if m.WatchLogin && strings.TrimSpace(m.BngLogin) != "" {
			var online *bool
			var bng *string
			_ = s.DB().QueryRow(ctx, `
				SELECT bool_or(k.is_online), (array_agg(d.description ORDER BY k.last_seen_at DESC))[1]
				FROM bng_known_logins k JOIN devices d ON d.id = k.device_id
				WHERE lower(trim(k.login)) = $1
			`, strings.ToLower(strings.TrimSpace(m.BngLogin))).Scan(&online, &bng)
			m.LoginOnline = online
			if bng != nil {
				m.LoginBng = strings.TrimSpace(*bng)
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"monitors": out})
}

type onuMonitorUpsertRequest struct {
	Serial         string `json:"serial"`
	OltDeviceID    string `json:"olt_device_id"`
	OltDescription string `json:"olt_description"`
	Pon            *int   `json:"pon"`
	Onu            *int   `json:"onu"`
	ClientName     string `json:"client_name"`
	WatchStatus    *bool  `json:"watch_status"`
	WatchRx        *bool  `json:"watch_rx"`
	WatchLogin     *bool  `json:"watch_login"`
	BngLogin       string `json:"bng_login"`
	NotifyTelegram *bool  `json:"notify_telegram"`
	// 0 / ausente = monitora até remoção manual; > 0 = expira daqui a N horas.
	DurationHours *float64 `json:"duration_hours"`
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// upsertOnuMonitor cria ou reconfigura o monitoramento de uma ONU (chave: serial).
func (s *Server) upsertOnuMonitor(w http.ResponseWriter, r *http.Request) {
	var body onuMonitorUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	serial := strings.ToUpper(strings.TrimSpace(body.Serial))
	if serial == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "serial obrigatório", nil)
		return
	}
	var oltID *uuid.UUID
	if s := strings.TrimSpace(body.OltDeviceID); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "olt_device_id inválido", nil)
			return
		}
		oltID = &id
	}
	if oltID == nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "olt_device_id obrigatório", nil)
		return
	}
	watchStatus := boolOr(body.WatchStatus, true)
	watchRx := boolOr(body.WatchRx, true)
	watchLogin := boolOr(body.WatchLogin, false)
	notify := boolOr(body.NotifyTelegram, true)
	if !watchStatus && !watchRx && !watchLogin {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "selecione ao menos um monitoramento (status, RX ou login)", nil)
		return
	}
	bngLogin := strings.TrimSpace(body.BngLogin)
	if watchLogin && bngLogin == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "informe o login PPPoE para confirmar no BNG", nil)
		return
	}
	var expiresAt *time.Time
	if body.DurationHours != nil && *body.DurationHours > 0 {
		t := time.Now().Add(time.Duration(*body.DurationHours * float64(time.Hour)))
		expiresAt = &t
	}
	ctx := r.Context()

	// Baseline: estado corrente da ONU no snapshot, para não disparar falso alarme logo ao criar.
	var rxGood, rxBad float64
	if err := s.DB().QueryRow(ctx, `SELECT onu_rx_good_dbm, onu_rx_bad_dbm FROM monitoring_settings WHERE id=1`).Scan(&rxGood, &rxBad); err != nil {
		rxGood, rxBad = -23, -27
	}
	var sum string
	_ = s.DB().QueryRow(ctx, `SELECT COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, *oltID).Scan(&sum)
	st := onumonitor.StateFromSummary([]byte(sum), serial, rxGood, rxBad)

	_, err := s.DB().Exec(ctx, `
		INSERT INTO onu_monitors (serial, olt_device_id, olt_description, pon, onu, client_name,
			watch_status, watch_rx, watch_login, bng_login, notify_telegram, expires_at,
			last_online, last_rx_dbm, last_rx_class, last_evaluated_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,''),now(),$16)
		ON CONFLICT (serial) DO UPDATE SET
			olt_device_id = EXCLUDED.olt_device_id,
			olt_description = EXCLUDED.olt_description,
			pon = EXCLUDED.pon,
			onu = EXCLUDED.onu,
			client_name = EXCLUDED.client_name,
			watch_status = EXCLUDED.watch_status,
			watch_rx = EXCLUDED.watch_rx,
			watch_login = EXCLUDED.watch_login,
			bng_login = EXCLUDED.bng_login,
			notify_telegram = EXCLUDED.notify_telegram,
			expires_at = EXCLUDED.expires_at,
			updated_at = now()
	`, serial, *oltID, strings.TrimSpace(body.OltDescription), body.Pon, body.Onu, strings.TrimSpace(body.ClientName),
		watchStatus, watchRx, watchLogin, bngLogin, notify, expiresAt,
		st.Online, nullableFloat(st.RxDbm), st.RxClass, s.actorFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(ctx, "onu_monitor", serial, "upsert", s.actorFromRequest(r), nil, map[string]any{
		"serial": serial, "watch_status": watchStatus, "watch_rx": watchRx, "watch_login": watchLogin,
		"bng_login": bngLogin, "expires_at": expiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serial": serial})
}

// deleteOnuMonitor remove o monitoramento e fecha os alertas abertos dessa ONU.
func (s *Server) deleteOnuMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ctx := r.Context()
	var serial string
	var dev *uuid.UUID
	if err := s.DB().QueryRow(ctx, `SELECT serial, olt_device_id FROM onu_monitors WHERE id=$1`, id).Scan(&serial, &dev); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if dev != nil {
		for _, at := range []string{onumonitor.AlertTypeOffline, onumonitor.AlertTypeRxLow} {
			for _, suf := range []string{":status", ":rx"} {
				key := "onu_monitor:" + id.String() + suf
				_, _, _ = alertstore.Close(ctx, s.DB(), nil, alertstore.CloseSpec{
					DeviceID: *dev, AlertType: at,
					Match:    alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
					Resolved: map[string]any{"resolved": "monitor_removido", "key": key},
				})
			}
		}
	}
	if _, err := s.DB().Exec(ctx, `DELETE FROM onu_monitors WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(ctx, "onu_monitor", serial, "delete", s.actorFromRequest(r), nil, map[string]any{"serial": serial})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func nullableFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
