package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertignore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertverify"
)

func (s *Server) alertIgnore(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ignoreID, err := alertignore.IgnoreFromAlert(r.Context(), s.DB(), id, body.Reason, nil)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "alerta não encontrado ou já fechado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "alert", id.String(), "ignore", actorFromRequest(r), nil, map[string]any{"ignore_id": ignoreID.String()})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignore_id": ignoreID})
}

func (s *Server) alertVerifyOne(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	res, err := alertverify.VerifyAlert(r.Context(), s.DB(), &s.Log, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "alerta não encontrado ou já fechado", nil)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) alertsVerifyAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	closedPing, err := alertverify.RevalidatePingAlerts(ctx, s.DB(), &s.Log)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	verified, resolved, err := alertverify.VerifyAllOpen(ctx, s.DB(), &s.Log, 250)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"closed_ping_count": closedPing,
		"verified_count":    verified,
		"resolved_count":    resolved,
		"note":              "Recalculou ping offline e reverificou alertas abertos com coleta actualizada.",
	})
}

func (s *Server) alertsIgnoredList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := alertignore.ListActive(r.Context(), s.DB(), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	list := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":                 row.ID,
			"device_id":          row.DeviceID,
			"type":               row.AlertType,
			"meta_key":           row.MetaKey,
			"device_name":        row.DeviceName,
			"ip":                 row.IP,
			"severity":           row.Severity,
			"message":            row.LastMessage,
			"meta":               row.LastMeta,
			"reason":             row.Reason,
			"ignored_at":         row.IgnoredAt,
			"source_alert_id":    row.SourceAlertID,
			"last_verified_at":   row.LastVerifiedAt,
			"last_verify_result": row.LastVerifyResult,
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ignored": list})
}

func (s *Server) alertIgnoreReactivate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	if err := alertignore.Reactivate(r.Context(), s.DB(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "alert_ignore", id.String(), "reactivate", actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
