package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

func (s *Server) listMikrotikTelnetProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := mikrotikcollect.ListTelnetProfiles(r.Context(), s.DB())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profiles": profiles,
		"catalog":  mikrotikcollect.TelnetMetricCatalog,
		"sections": mikrotikcollect.TelnetSectionLabels,
	})
}

func (s *Server) getMikrotikTelnetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	p, err := mikrotikcollect.LoadTelnetProfileByID(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type mikrotikTelnetProfileBody struct {
	Name        string                           `json:"name"`
	Metrics     mikrotikcollect.TelnetMetricsConfig `json:"metrics"`
	PreCommands []string                         `json:"pre_commands"`
	IsDefault   *bool                            `json:"is_default"`
}

func (s *Server) createMikrotikTelnetProfile(w http.ResponseWriter, r *http.Request) {
	var body mikrotikTelnetProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome obrigatório", nil)
		return
	}
	taken, err := mikrotikcollect.IsTelnetProfileNameTaken(r.Context(), s.DB(), name, uuid.Nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if taken {
		writeErr(w, http.StatusConflict, "CONFLICT", "já existe um perfil com este nome", nil)
		return
	}
	metrics := body.Metrics.Normalize().MergeWithDefaults()
	pre := body.PreCommands
	if pre == nil {
		pre = []string{}
	}
	mb, _ := json.Marshal(metrics)
	pb, _ := json.Marshal(pre)
	isDefault := body.IsDefault != nil && *body.IsDefault
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO mikrotik_telnet_profiles (name, metrics, pre_commands, is_default)
		VALUES ($1, $2::jsonb, $3::jsonb, $4)
		RETURNING id
	`, name, mb, pb, isDefault).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if isDefault {
		_ = mikrotikcollect.ClearDefaultTelnetProfile(r.Context(), s.DB(), id)
	}
	p, _ := mikrotikcollect.LoadTelnetProfileByID(r.Context(), s.DB(), id)
	s.appendAuditLog(r.Context(), "mikrotik_telnet_profile", id.String(), "create", s.actorFromRequest(r), nil, map[string]any{"name": name})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) patchMikrotikTelnetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body mikrotikTelnetProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		taken, err := mikrotikcollect.IsTelnetProfileNameTaken(r.Context(), s.DB(), body.Name, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if taken {
			writeErr(w, http.StatusConflict, "CONFLICT", "já existe um perfil com este nome", nil)
			return
		}
	}
	metrics := body.Metrics.Normalize().MergeWithDefaults()
	pre := body.PreCommands
	if pre == nil {
		pre = []string{}
	}
	mb, _ := json.Marshal(metrics)
	pb, _ := json.Marshal(pre)
	isDefault := false
	if body.IsDefault != nil {
		isDefault = *body.IsDefault
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE mikrotik_telnet_profiles SET
			name = COALESCE(NULLIF(trim($2), ''), name),
			metrics = $3::jsonb,
			pre_commands = $4::jsonb,
			is_default = CASE WHEN $5::boolean THEN true ELSE is_default END,
			updated_at = now()
		WHERE id = $1
	`, id, body.Name, mb, pb, isDefault)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if isDefault {
		_ = mikrotikcollect.ClearDefaultTelnetProfile(r.Context(), s.DB(), id)
	}
	p, err := mikrotikcollect.LoadTelnetProfileByID(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "mikrotik_telnet_profile", id.String(), "patch", s.actorFromRequest(r), nil, map[string]any{"name": p.Name})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteMikrotikTelnetProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var isDefault bool
	err = s.DB().QueryRow(r.Context(), `SELECT is_default FROM mikrotik_telnet_profiles WHERE id=$1`, id).Scan(&isDefault)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	if isDefault {
		writeErr(w, http.StatusConflict, "CONFLICT", "não é possível apagar o perfil padrão", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `DELETE FROM mikrotik_telnet_profiles WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "mikrotik_telnet_profile", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
