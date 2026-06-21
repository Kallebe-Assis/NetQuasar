package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type authLoginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// "login" aceite por compatibilidade com clientes antigos
	Login string `json:"login"`
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "METHOD", "use POST", nil)
		return
	}
	p := s.DB()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base de dados ainda não configurada neste servidor", nil)
		return
	}
	var body authLoginBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" {
		email = strings.TrimSpace(body.Login)
	}
	pw := body.Password
	if email == "" || pw == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "email e password são obrigatórios", nil)
		return
	}
	var id uuid.UUID
	var hash []byte
	var role, displayName string
	err := p.QueryRow(r.Context(), `
		SELECT id, password_hash, role,
			COALESCE(NULLIF(trim(display_name), ''), trim(email))
		FROM users WHERE lower(trim(email)) = lower(trim($1))
	`, email).Scan(&id, &hash, &role, &displayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.appendAuditLog(r.Context(), "auth", email, "login_failed", email, nil, map[string]any{"reason": "user_not_found"})
			writeErr(w, http.StatusUnauthorized, "AUTH_FAILED", "credenciais inválidas", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(pw)); err != nil {
		s.appendAuditLog(r.Context(), "auth", id.String(), "login_failed", email, nil, map[string]any{"reason": "bad_password"})
		writeErr(w, http.StatusUnauthorized, "AUTH_FAILED", "credenciais inválidas", nil)
		return
	}
	token, err := mintUserJWT(s.Cfg, id, email, role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TOKEN", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "auth", id.String(), "login_success", displayName, nil, map[string]any{"email": email, "role": role})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":         token,
		"email":         email,
		"display_name":  displayName,
		"role":          role,
	})
}
