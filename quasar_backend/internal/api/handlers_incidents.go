package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertcorrelation"
)

func (s *Server) incidentsActive(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT i.id, i.root_cause, i.title, i.summary, i.pop_id, i.root_device_id,
			i.opened_at, i.updated_at,
			(SELECT COUNT(*)::bigint FROM alert_incident_alerts a WHERE a.incident_id = i.id) AS alert_count,
			(SELECT COUNT(*)::bigint FROM alert_incident_alerts a
			 JOIN alert_instances ai ON ai.id = a.alert_id
			 WHERE a.incident_id = i.id AND ai.closed_at IS NULL) AS open_alert_count
		FROM alert_incidents i
		WHERE i.status = 'open'
		ORDER BY i.opened_at DESC
		LIMIT 200
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var cause, title string
		var summary *string
		var popID, rootDev *uuid.UUID
		var opened, updated interface{}
		var total, open int64
		if err := rows.Scan(&id, &cause, &title, &summary, &popID, &rootDev, &opened, &updated, &total, &open); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "root_cause": cause, "title": title, "summary": summary,
			"pop_id": popID, "root_device_id": rootDev,
			"opened_at": opened, "updated_at": updated,
			"alert_count": total, "open_alert_count": open,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": list})
}

func (s *Server) incidentDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var cause, title, status string
	var summary *string
	var popID, rootDev *uuid.UUID
	var opened, resolved, updated interface{}
	err = s.DB().QueryRow(r.Context(), `
		SELECT root_cause, title, summary, status, pop_id, root_device_id, opened_at, resolved_at, updated_at
		FROM alert_incidents WHERE id = $1
	`, id).Scan(&cause, &title, &summary, &status, &popID, &rootDev, &opened, &resolved, &updated)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT aia.role, ai.id, ai.severity, ai.alert_type, ai.message, ai.device_name, ai.ip,
			ai.active_since, ai.closed_at
		FROM alert_incident_alerts aia
		JOIN alert_instances ai ON ai.id = aia.alert_id
		WHERE aia.incident_id = $1
		ORDER BY aia.role DESC, ai.active_since ASC
	`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var alerts []map[string]any
	for rows.Next() {
		var role, sev, typ, msg, dname, ip string
		var aid uuid.UUID
		var since interface{}
		var closed interface{}
		if err := rows.Scan(&role, &aid, &sev, &typ, &msg, &dname, &ip, &since, &closed); err != nil {
			continue
		}
		alerts = append(alerts, map[string]any{
			"role": role, "id": aid, "severity": sev, "type": typ, "message": msg,
			"device_name": dname, "ip": ip, "active_since": since, "closed_at": closed,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incident": map[string]any{
			"id": id, "root_cause": cause, "title": title, "summary": summary, "status": status,
			"pop_id": popID, "root_device_id": rootDev,
			"opened_at": opened, "resolved_at": resolved, "updated_at": updated,
		},
		"alerts": alerts,
	})
}

func (s *Server) incidentsReconcile(w http.ResponseWriter, r *http.Request) {
	alertcorrelation.Reconcile(r.Context(), s.DB(), &s.Log)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
