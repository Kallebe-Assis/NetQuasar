package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/localdbstore"
)

const (
	builtinAlertSoundID = "builtin:alert"
	maxCustomAlertSounds = 8
	maxAlertSoundBytes   = 2 << 20
)

type userAlertSound struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type userPreferences struct {
	Theme                string           `json:"theme"`
	AlertToastEverywhere bool             `json:"alert_toast_everywhere"`
	AlertSoundEnabled    bool             `json:"alert_sound_enabled"`
	AlertSoundID         string           `json:"alert_sound_id"`
	CustomSounds         []userAlertSound `json:"custom_sounds,omitempty"`
}

func defaultUserPreferences() userPreferences {
	return userPreferences{
		Theme:                "dark",
		AlertToastEverywhere: true,
		AlertSoundEnabled:    true,
		AlertSoundID:         builtinAlertSoundID,
	}
}

func builtinAlertSoundIDs() map[string]string {
	return map[string]string{
		"builtin:alert":  "Alerta",
		"builtin:chime":  "Sino",
		"builtin:urgent": "Urgente",
		"builtin:ping":   "Toque",
	}
}

func normalizeUserPreferences(p userPreferences) userPreferences {
	if p.Theme != "light" && p.Theme != "dark" {
		p.Theme = "dark"
	}
	p.CustomSounds = sanitizeCustomSounds(p.CustomSounds)
	if !validAlertSoundID(p.AlertSoundID, p.CustomSounds) {
		p.AlertSoundID = builtinAlertSoundID
	}
	return p
}

func sanitizeCustomSounds(in []userAlertSound) []userAlertSound {
	var out []userAlertSound
	seen := map[string]bool{}
	for _, s := range in {
		id := strings.TrimSpace(s.ID)
		name := strings.TrimSpace(s.Name)
		fn := filepath.Base(strings.TrimSpace(s.Filename))
		if !validCustomSoundID(id) || name == "" || !strings.HasSuffix(strings.ToLower(fn), ".mp3") {
			continue
		}
		if utf8.RuneCountInString(name) > 80 {
			name = string([]rune(name)[:80])
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, userAlertSound{ID: id, Name: name, Filename: fn})
		if len(out) >= maxCustomAlertSounds {
			break
		}
	}
	return out
}

func validCustomSoundID(id string) bool {
	raw, ok := strings.CutPrefix(strings.TrimSpace(id), "custom:")
	if !ok {
		return false
	}
	_, err := uuid.Parse(raw)
	return err == nil
}

func validAlertSoundID(id string, custom []userAlertSound) bool {
	id = strings.TrimSpace(id)
	if _, ok := builtinAlertSoundIDs()[id]; ok {
		return true
	}
	for _, s := range custom {
		if s.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) requireSessionUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	ac := s.requestAuthContext(r)
	if !ac.OK || ac.UserID == uuid.Nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "inicie sessão para gerir as suas preferências", nil)
		return uuid.Nil, false
	}
	return ac.UserID, true
}

func (s *Server) loadUserPreferences(r *http.Request, userID uuid.UUID) (userPreferences, error) {
	var raw []byte
	err := s.DB().QueryRow(r.Context(), `SELECT COALESCE(preferences, '{}'::jsonb) FROM users WHERE id=$1`, userID).Scan(&raw)
	if err != nil {
		return defaultUserPreferences(), err
	}
	p := defaultUserPreferences()
	if len(raw) > 0 && string(raw) != "null" && string(raw) != "{}" {
		_ = json.Unmarshal(raw, &p)
	}
	return normalizeUserPreferences(p), nil
}

func (s *Server) saveUserPreferences(r *http.Request, userID uuid.UUID, p userPreferences) error {
	p = normalizeUserPreferences(p)
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.DB().Exec(r.Context(), `UPDATE users SET preferences=$2::jsonb, updated_at=now() WHERE id=$1`, userID, string(b))
	return err
}

func (s *Server) getMyPreferences(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	if s.DB() == nil {
		writeJSON(w, http.StatusOK, defaultUserPreferences())
		return
	}
	p, err := s.loadUserPreferences(r, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) patchMyPreferences(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	if s.DB() == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base indisponível", nil)
		return
	}
	var body struct {
		Theme                *string `json:"theme"`
		AlertToastEverywhere *bool   `json:"alert_toast_everywhere"`
		AlertSoundEnabled    *bool   `json:"alert_sound_enabled"`
		AlertSoundID         *string `json:"alert_sound_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	p, err := s.loadUserPreferences(r, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if body.Theme != nil {
		t := strings.ToLower(strings.TrimSpace(*body.Theme))
		if t != "light" && t != "dark" {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "tema deve ser dark ou light", nil)
			return
		}
		p.Theme = t
	}
	if body.AlertToastEverywhere != nil {
		p.AlertToastEverywhere = *body.AlertToastEverywhere
	}
	if body.AlertSoundEnabled != nil {
		p.AlertSoundEnabled = *body.AlertSoundEnabled
	}
	if body.AlertSoundID != nil {
		id := strings.TrimSpace(*body.AlertSoundID)
		if !validAlertSoundID(id, p.CustomSounds) {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "som de alerta desconhecido", nil)
			return
		}
		p.AlertSoundID = id
	}
	if err := s.saveUserPreferences(r, uid, p); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "user_preferences", uid.String(), "update", s.actorFromRequest(r), nil, map[string]any{
		"theme": p.Theme, "alert_toast_everywhere": p.AlertToastEverywhere, "alert_sound_enabled": p.AlertSoundEnabled,
	})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) listMyAlertSounds(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	builtins := []map[string]string{
		{"id": "builtin:alert", "name": "Alerta", "kind": "builtin"},
		{"id": "builtin:chime", "name": "Sino", "kind": "builtin"},
		{"id": "builtin:urgent", "name": "Urgente", "kind": "builtin"},
		{"id": "builtin:ping", "name": "Toque", "kind": "builtin"},
	}
	custom := []userAlertSound{}
	if s.DB() != nil {
		if p, err := s.loadUserPreferences(r, uid); err == nil {
			custom = p.CustomSounds
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"builtins": builtins, "custom": custom})
}

func userAlertSoundDir(userID uuid.UUID) string {
	return filepath.Join(localdbstore.DataDir(), "user-alert-sounds", userID.String())
}

func (s *Server) uploadMyAlertSound(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	if s.DB() == nil {
		writeErr(w, http.StatusServiceUnavailable, "NO_DB", "base indisponível", nil)
		return
	}
	if err := r.ParseMultipartForm(maxAlertSoundBytes + (1 << 20)); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_UPLOAD", "ficheiro inválido ou demasiado grande (máx. 2 MB)", nil)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "envie um ficheiro MP3 no campo file", nil)
		return
	}
	defer f.Close()
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(hdr.Filename), filepath.Ext(hdr.Filename))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Som personalizado"
	}
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxAlertSoundBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_UPLOAD", err.Error(), nil)
		return
	}
	if len(raw) == 0 || len(raw) > maxAlertSoundBytes {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "MP3 vazio ou acima de 2 MB", nil)
		return
	}
	if !looksLikeMP3(raw) {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "o ficheiro tem de ser MP3", nil)
		return
	}
	p, err := s.loadUserPreferences(r, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if len(p.CustomSounds) >= maxCustomAlertSounds {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "máximo de 8 sons personalizados", nil)
		return
	}
	id := uuid.New()
	filename := id.String() + ".mp3"
	dir := userAlertSoundDir(uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, "IO", err.Error(), nil)
		return
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "IO", err.Error(), nil)
		return
	}
	sound := userAlertSound{ID: "custom:" + id.String(), Name: name, Filename: filename}
	p.CustomSounds = append(p.CustomSounds, sound)
	p.AlertSoundID = sound.ID
	if err := s.saveUserPreferences(r, uid, p); err != nil {
		_ = os.Remove(path)
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, sound)
}

func looksLikeMP3(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	if string(b[:3]) == "ID3" {
		return true
	}
	return b[0] == 0xFF && (b[1]&0xE0) == 0xE0
}

func (s *Server) getMyAlertSoundFile(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !strings.HasPrefix(id, "custom:") {
		id = "custom:" + id
	}
	if s.DB() == nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "som não encontrado", nil)
		return
	}
	p, err := s.loadUserPreferences(r, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var fn string
	for _, snd := range p.CustomSounds {
		if snd.ID == id {
			fn = snd.Filename
			break
		}
	}
	if fn == "" {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "som não encontrado", nil)
		return
	}
	path := filepath.Join(userAlertSoundDir(uid), filepath.Base(fn))
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "ficheiro em falta", nil)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "IO", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, fn, st.ModTime(), f)
}

func (s *Server) deleteMyAlertSound(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.requireSessionUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !strings.HasPrefix(id, "custom:") {
		id = "custom:" + id
	}
	if !validCustomSoundID(id) {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "identificador inválido", nil)
		return
	}
	p, err := s.loadUserPreferences(r, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var kept []userAlertSound
	var removed *userAlertSound
	for i := range p.CustomSounds {
		if p.CustomSounds[i].ID == id {
			cp := p.CustomSounds[i]
			removed = &cp
			continue
		}
		kept = append(kept, p.CustomSounds[i])
	}
	if removed == nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "som não encontrado", nil)
		return
	}
	p.CustomSounds = kept
	if p.AlertSoundID == id {
		p.AlertSoundID = builtinAlertSoundID
	}
	if err := s.saveUserPreferences(r, uid, p); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	_ = os.Remove(filepath.Join(userAlertSoundDir(uid), filepath.Base(removed.Filename)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preferences": p})
}
