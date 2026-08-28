package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/bgpcollect"
)

// handlers_bgp_snmp_profiles.go — CRUD de perfis SNMP de BGP (Configurações → BGP), mirror
// directo de handlers_mikrotik_telnet_profiles.go: vários perfis nomeados, um marcado como
// padrão (usado pela coleta periódica, ver internal/monitorworker/bgp_subsweep.go).

func (s *Server) listBgpSnmpProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := bgpcollect.ListProfiles(r.Context(), s.DB())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profiles": profiles,
		"catalog":  bgpcollect.MetricCatalog,
		"sections": bgpcollect.SectionLabels,
	})
}

func (s *Server) getBgpSnmpProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	p, err := bgpcollect.LoadProfileByID(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type bgpSnmpProfileBody struct {
	Name      string                   `json:"name"`
	Metrics   bgpcollect.MetricsConfig `json:"metrics"`
	IsDefault *bool                    `json:"is_default"`
}

func (s *Server) createBgpSnmpProfile(w http.ResponseWriter, r *http.Request) {
	var body bgpSnmpProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome obrigatório", nil)
		return
	}
	taken, err := bgpcollect.IsProfileNameTaken(r.Context(), s.DB(), name, uuid.Nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if taken {
		writeErr(w, http.StatusConflict, "CONFLICT", "já existe um perfil com este nome", nil)
		return
	}
	metrics := body.Metrics.Normalize().MergeWithDefaults()
	mb, _ := json.Marshal(metrics)
	isDefault := body.IsDefault != nil && *body.IsDefault
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO bgp_snmp_profiles (name, metrics, is_default)
		VALUES ($1, $2::jsonb, $3)
		RETURNING id
	`, name, mb, isDefault).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if isDefault {
		_ = bgpcollect.ClearDefaultProfile(r.Context(), s.DB(), id)
	}
	p, _ := bgpcollect.LoadProfileByID(r.Context(), s.DB(), id)
	s.appendAuditLog(r.Context(), "bgp_snmp_profile", id.String(), "create", s.actorFromRequest(r), nil, map[string]any{"name": name})
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) patchBgpSnmpProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body bgpSnmpProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		taken, err := bgpcollect.IsProfileNameTaken(r.Context(), s.DB(), body.Name, id)
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
	mb, _ := json.Marshal(metrics)
	isDefault := false
	if body.IsDefault != nil {
		isDefault = *body.IsDefault
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE bgp_snmp_profiles SET
			name = COALESCE(NULLIF(trim($2), ''), name),
			metrics = $3::jsonb,
			is_default = CASE WHEN $4::boolean THEN true ELSE is_default END,
			updated_at = now()
		WHERE id = $1
	`, id, body.Name, mb, isDefault)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if isDefault {
		_ = bgpcollect.ClearDefaultProfile(r.Context(), s.DB(), id)
	}
	p, err := bgpcollect.LoadProfileByID(r.Context(), s.DB(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "bgp_snmp_profile", id.String(), "patch", s.actorFromRequest(r), nil, map[string]any{"name": p.Name})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteBgpSnmpProfile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var isDefault bool
	err = s.DB().QueryRow(r.Context(), `SELECT is_default FROM bgp_snmp_profiles WHERE id=$1`, id).Scan(&isDefault)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	if isDefault {
		writeErr(w, http.StatusConflict, "CONFLICT", "não é possível apagar o perfil padrão", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `DELETE FROM bgp_snmp_profiles WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "bgp_snmp_profile", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
