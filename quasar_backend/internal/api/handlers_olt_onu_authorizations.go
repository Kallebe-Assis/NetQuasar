package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type onuAuthorizationRow struct {
	ID             uuid.UUID  `json:"id"`
	OltDeviceID    *uuid.UUID `json:"olt_device_id,omitempty"`
	OltDescription string     `json:"olt_description"`
	Serial         string     `json:"serial"`
	Model          string     `json:"model"`
	Pon            *int       `json:"pon,omitempty"`
	Onu            *int       `json:"onu,omitempty"`
	Vlan           string     `json:"vlan"`
	ClientName     string     `json:"client_name"`
	AuthorizedBy   string     `json:"authorized_by"`
	AuthorizedAt   time.Time  `json:"authorized_at"`
}

// recentOnuAuthorizations devolve as últimas N autorizações de ONU (default 5). O nome do
// cliente vem do vínculo actual (onu_client_links) quando existir, senão do que ficou gravado
// no momento da autorização.
func (s *Server) recentOnuAuthorizations(w http.ResponseWriter, r *http.Request) {
	limit := 5
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT a.id, a.olt_device_id, a.olt_description, a.serial, a.model, a.pon, a.onu, a.vlan,
			COALESCE(NULLIF(l.client_name, ''), a.client_name) AS client_name,
			a.authorized_by, a.authorized_at
		FROM onu_authorizations a
		LEFT JOIN onu_client_links l ON l.serial = a.serial
		ORDER BY a.authorized_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := make([]onuAuthorizationRow, 0, limit)
	for rows.Next() {
		var it onuAuthorizationRow
		if err := rows.Scan(&it.ID, &it.OltDeviceID, &it.OltDescription, &it.Serial, &it.Model,
			&it.Pon, &it.Onu, &it.Vlan, &it.ClientName, &it.AuthorizedBy, &it.AuthorizedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// linkOnuClientDirect grava/actualiza o vínculo serial → cliente sem exigir que o serial já
// tenha aparecido num snapshot de OLT (caso da ONU recém-autorizada, ainda não coletada).
// Também carimba a autorização mais recente desse serial com o nome do cliente.
func (s *Server) linkOnuClientDirect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Serial     string `json:"serial"`
		ClientName string `json:"client_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	serial := strings.ToUpper(strings.TrimSpace(body.Serial))
	name := strings.TrimSpace(body.ClientName)
	if serial == "" || name == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "serial e client_name obrigatórios", nil)
		return
	}
	ctx := r.Context()
	if _, err := s.DB().Exec(ctx, `
		INSERT INTO onu_client_links (serial, client_name)
		VALUES ($1, $2)
		ON CONFLICT (serial) DO UPDATE SET client_name = EXCLUDED.client_name, updated_at = now()
	`, serial, name); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if _, err := s.DB().Exec(ctx, `
		UPDATE onu_authorizations SET client_name = $2
		WHERE id = (SELECT id FROM onu_authorizations WHERE serial = $1 ORDER BY authorized_at DESC LIMIT 1)
	`, serial, name); err != nil {
		s.Log.Warn().Err(err).Msg("falha ao carimbar cliente na autorização de ONU")
	}
	s.appendAuditLog(ctx, "onu_client_link", serial, "link", s.actorFromRequest(r), nil, map[string]any{
		"serial": serial, "client_name": name,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serial": serial, "client_name": name})
}
