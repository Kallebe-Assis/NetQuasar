package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type permissionProfileRow struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func slugifyPermissionProfile(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "perfil"
	}
	return out
}

func decodePermissionsJSON(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	valid, _ := normalizePermissions(out)
	return valid
}

func scanPermissionProfile(scanner interface {
	Scan(dest ...any) error
}) (permissionProfileRow, error) {
	var row permissionProfileRow
	var desc sql.NullString
	var permsRaw []byte
	err := scanner.Scan(&row.ID, &row.Name, &row.Slug, &desc, &permsRaw, &row.IsSystem, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return row, err
	}
	if desc.Valid {
		s := desc.String
		row.Description = &s
	}
	row.Permissions = decodePermissionsJSON(permsRaw)
	return row, nil
}

func (s *Server) loadPermissionProfileByID(ctx context.Context, id uuid.UUID) (permissionProfileRow, error) {
	row := s.DB().QueryRow(ctx, `
		SELECT id, name, slug, description, permissions, is_system, created_at, updated_at
		FROM permission_profiles WHERE id=$1
	`, id)
	return scanPermissionProfile(row)
}

func (s *Server) loadPermissionProfileBySlug(ctx context.Context, slug string) (permissionProfileRow, error) {
	row := s.DB().QueryRow(ctx, `
		SELECT id, name, slug, description, permissions, is_system, created_at, updated_at
		FROM permission_profiles WHERE slug=$1
	`, strings.TrimSpace(slug))
	return scanPermissionProfile(row)
}

func (s *Server) ensureUniqueProfileSlug(ctx context.Context, base string, excludeID *uuid.UUID) (string, error) {
	base = slugifyPermissionProfile(base)
	candidate := base
	for i := 0; i < 50; i++ {
		var exists bool
		var err error
		if excludeID != nil {
			err = s.DB().QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM permission_profiles WHERE slug=$1 AND id<>$2)
			`, candidate, *excludeID).Scan(&exists)
		} else {
			err = s.DB().QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM permission_profiles WHERE slug=$1)
			`, candidate).Scan(&exists)
		}
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = base + "-" + uuid.NewString()[:8]
	}
	return "", errors.New("não foi possível gerar slug único")
}

func (s *Server) listPermissionCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions", "settings.users") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": permissionCatalog})
}

func (s *Server) listPermissionProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions", "settings.users") {
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, name, slug, description, permissions, is_system, created_at, updated_at
		FROM permission_profiles
		ORDER BY is_system DESC, name ASC
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := make([]permissionProfileRow, 0)
	for rows.Next() {
		row, err := scanPermissionProfile(rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": list})
}

func (s *Server) getPermissionProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions", "settings.users") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	row, err := s.loadPermissionProfileByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) createPermissionProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions") {
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "name é obrigatório", nil)
		return
	}
	perms, invalid := normalizePermissions(body.Permissions)
	if len(invalid) > 0 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "permissões inválidas", map[string]any{"invalid": invalid})
		return
	}
	if permissionGranted(perms, "*") {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "apenas o perfil Administrador do sistema pode ter acesso total (*)", nil)
		return
	}
	slug, err := s.ensureUniqueProfileSlug(r.Context(), name, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	permsJSON, _ := json.Marshal(perms)
	var desc any
	if body.Description != nil {
		t := strings.TrimSpace(*body.Description)
		if t != "" {
			desc = t
		}
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO permission_profiles (name, slug, description, permissions, is_system)
		VALUES ($1,$2,$3,$4::jsonb,false)
		RETURNING id
	`, name, slug, desc, permsJSON).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "permission_profile", id.String(), "create", s.actorFromRequest(r), nil, map[string]any{
		"name": name, "slug": slug, "permissions": perms,
	})
	row, err := s.loadPermissionProfileByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) patchPermissionProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	current, err := s.loadPermissionProfileByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var body struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Permissions *[]string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	name := current.Name
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
		if name == "" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "name não pode ser vazio", nil)
			return
		}
	}
	slug := current.Slug
	if current.IsSystem {
		if current.Slug == "admin" && body.Permissions != nil {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "o perfil Administrador mantém acesso total e não pode ser alterado", nil)
			return
		}
	} else if body.Name != nil && !strings.EqualFold(name, current.Name) {
		slug, err = s.ensureUniqueProfileSlug(r.Context(), name, &id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	perms := current.Permissions
	if body.Permissions != nil {
		valid, invalid := normalizePermissions(*body.Permissions)
		if len(invalid) > 0 {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "permissões inválidas", map[string]any{"invalid": invalid})
			return
		}
		if current.Slug == "admin" {
			valid = []string{"*"}
		} else if permissionGranted(valid, "*") {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "apenas o perfil Administrador do sistema pode ter acesso total (*)", nil)
			return
		}
		perms = valid
	}
	var desc any
	descSet := false
	if body.Description != nil {
		descSet = true
		t := strings.TrimSpace(*body.Description)
		if t != "" {
			desc = t
		} else {
			desc = nil
		}
	}
	permsJSON, _ := json.Marshal(perms)
	_, err = s.DB().Exec(r.Context(), `
		UPDATE permission_profiles SET
			name = $2,
			slug = $3,
			description = CASE WHEN $4 THEN $5::text ELSE description END,
			permissions = $6::jsonb,
			updated_at = now()
		WHERE id=$1
	`, id, name, slug, descSet, desc, permsJSON)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "permission_profile", id.String(), "patch", s.actorFromRequest(r), nil, map[string]any{
		"name": name, "slug": slug, "permissions": perms,
	})
	row, err := s.loadPermissionProfileByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) deletePermissionProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "settings.permissions") {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	current, err := s.loadPermissionProfileByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if current.IsSystem {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "perfis de sistema não podem ser removidos", nil)
		return
	}
	var inUse int
	if err := s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE permission_profile_id=$1`, id).Scan(&inUse); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if inUse > 0 {
		writeErr(w, http.StatusConflict, "IN_USE", "perfil está atribuído a usuários; reatribua-os antes de apagar", map[string]any{"users": inUse})
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM permission_profiles WHERE id=$1 AND is_system=false`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "perfil não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "permission_profile", id.String(), "delete", s.actorFromRequest(r), nil, map[string]any{
		"name": current.Name, "slug": current.Slug,
	})
	w.WriteHeader(http.StatusNoContent)
}
