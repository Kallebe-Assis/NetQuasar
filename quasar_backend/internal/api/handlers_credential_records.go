package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/credentialvault"
)

type credentialRecordRow struct {
	ID            uuid.UUID  `json:"id"`
	OwnerUserID   uuid.UUID  `json:"owner_user_id"`
	OwnerName     string     `json:"owner_name"`
	CreatedBy     *uuid.UUID `json:"created_by,omitempty"`
	CreatedByName *string    `json:"created_by_name,omitempty"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	DeviceID      *uuid.UUID `json:"device_id,omitempty"`
	DeviceName    *string    `json:"device_name,omitempty"`
	DeviceIP      *string    `json:"device_ip,omitempty"`
	Host          *string    `json:"host,omitempty"`
	Domain        *string    `json:"domain,omitempty"`
	Username      *string    `json:"username,omitempty"`
	HasUsername   bool       `json:"has_username"`
	HasPassword   bool       `json:"has_password"`
	Notes         *string    `json:"notes,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type credentialRecordBody struct {
	OwnerUserID string  `json:"owner_user_id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	DeviceID    *string `json:"device_id"`
	Host        *string `json:"host"`
	Domain      *string `json:"domain"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	Notes       *string `json:"notes"`
	Mode        string  `json:"mode"` // password | user_password
}

func normalizeCredentialKind(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "equipment", "equipamento", "device":
		return "equipment"
	case "server", "servidor":
		return "server"
	case "site", "website", "web":
		return "site"
	default:
		return ""
	}
}

func normalizeCredentialMode(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == "password" {
		return "password"
	}
	return "user_password"
}

func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func optionalTrimmed(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Server) vaultKey() ([]byte, error) {
	return credentialvault.DeriveKey(s.Cfg.JWTSigningSecret())
}

func (s *Server) vaultActor(w http.ResponseWriter, r *http.Request) (uid uuid.UUID, admin bool, ok bool) {
	ac := s.requestAuthContext(r)
	if !ac.OK {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "inicie sessão para aceder aos registos", nil)
		return uuid.Nil, false, false
	}
	admin = s.isAdminAuth(ac)
	uid = ac.UserID
	if uid == uuid.Nil {
		if u := s.userIDFromRequest(r); u != nil {
			uid = *u
		}
	}
	if !admin && uid == uuid.Nil {
		writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "inicie sessão para aceder aos registos", nil)
		return uuid.Nil, false, false
	}
	return uid, admin, true
}

func (s *Server) credentialOwner(ctx context.Context, id uuid.UUID) (uuid.UUID, bool) {
	var owner uuid.UUID
	err := s.DB().QueryRow(ctx, `SELECT owner_user_id FROM credential_records WHERE id=$1`, id).Scan(&owner)
	if err != nil {
		return uuid.Nil, false
	}
	return owner, true
}

func (s *Server) credentialRecordsLookups(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	if s.DB() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": []any{}, "devices": []any{}})
		return
	}
	ctx := r.Context()
	type userLite struct {
		ID    uuid.UUID `json:"id"`
		Label string    `json:"label"`
	}
	users := []userLite{}
	if admin {
		if rows, err := s.DB().Query(ctx, `
			SELECT id, COALESCE(NULLIF(trim(display_name), ''), email)
			FROM users
			WHERE COALESCE(is_active, true)
			ORDER BY display_name
		`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var u userLite
				if rows.Scan(&u.ID, &u.Label) == nil {
					users = append(users, u)
				}
			}
		}
	} else if actor != uuid.Nil {
		var u userLite
		if err := s.DB().QueryRow(ctx, `
			SELECT id, COALESCE(NULLIF(trim(display_name), ''), email)
			FROM users WHERE id=$1
		`, actor).Scan(&u.ID, &u.Label); err == nil {
			users = append(users, u)
		}
	}
	type deviceLite struct {
		ID          uuid.UUID `json:"id"`
		Description string    `json:"description"`
		IP          *string   `json:"ip,omitempty"`
		Category    string    `json:"category"`
	}
	devices := []deviceLite{}
	if rows, err := s.DB().Query(ctx, `
		SELECT id, description, host(ip)::text, COALESCE(category, '')
		FROM devices
		ORDER BY description
		LIMIT 3000
	`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var d deviceLite
			var ip *string
			if rows.Scan(&d.ID, &d.Description, &ip, &d.Category) == nil {
				d.IP = ip
				devices = append(devices, d)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "devices": devices})
}

func (s *Server) listCredentialRecords(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	if s.DB() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := normalizeCredentialKind(r.URL.Query().Get("kind"))
	ownerRaw := strings.TrimSpace(r.URL.Query().Get("owner_user_id"))

	var args []any
	where := []string{"1=1"}
	if kind != "" {
		args = append(args, kind)
		where = append(where, "c.kind = $"+strconv.Itoa(len(args)))
	}
	if !admin {
		args = append(args, actor)
		where = append(where, "c.owner_user_id = $"+strconv.Itoa(len(args)))
	} else if ownerRaw != "" {
		oid, err := uuid.Parse(ownerRaw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "owner_user_id inválido", nil)
			return
		}
		args = append(args, oid)
		where = append(where, "c.owner_user_id = $"+strconv.Itoa(len(args)))
	}
	if q != "" {
		args = append(args, "%"+q+"%")
		n := strconv.Itoa(len(args))
		where = append(where, `(
			c.title ILIKE $`+n+` OR COALESCE(c.host,'') ILIKE $`+n+` OR COALESCE(c.domain,'') ILIKE $`+n+` OR
			COALESCE(c.username,'') ILIKE $`+n+` OR COALESCE(c.notes,'') ILIKE $`+n+` OR
			COALESCE(d.description,'') ILIKE $`+n+` OR COALESCE(u.display_name,'') ILIKE $`+n+`
		)`)
	}

	sql := `
		SELECT c.id, c.owner_user_id, COALESCE(NULLIF(trim(u.display_name), ''), u.email),
			c.created_by, NULLIF(trim(cb.display_name), ''),
			c.kind, c.title, c.device_id, d.description, host(d.ip)::text,
			c.host, c.domain, c.username, c.notes, c.created_at, c.updated_at
		FROM credential_records c
		JOIN users u ON u.id = c.owner_user_id
		LEFT JOIN users cb ON cb.id = c.created_by
		LEFT JOIN devices d ON d.id = c.device_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY c.updated_at DESC
		LIMIT 500`
	rows, err := s.DB().Query(r.Context(), sql, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	items := []credentialRecordRow{}
	for rows.Next() {
		var it credentialRecordRow
		var createdName *string
		if err := rows.Scan(
			&it.ID, &it.OwnerUserID, &it.OwnerName, &it.CreatedBy, &createdName,
			&it.Kind, &it.Title, &it.DeviceID, &it.DeviceName, &it.DeviceIP,
			&it.Host, &it.Domain, &it.Username, &it.Notes, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it.CreatedByName = createdName
		it.HasUsername = it.Username != nil && strings.TrimSpace(*it.Username) != ""
		it.HasPassword = true
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) createCredentialRecord(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	var body credentialRecordBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if !admin {
		body.OwnerUserID = actor.String()
	}
	row, blob, errMsg := s.prepareCredentialRecord(r, body, true, nil)
	if errMsg != "" {
		writeErr(w, 422, "VALIDATION", errMsg, nil)
		return
	}
	var createdBy *uuid.UUID
	if actor != uuid.Nil {
		createdBy = &actor
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO credential_records (
			owner_user_id, created_by, kind, title, device_id, host, domain, username, password_blob, notes
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, row.OwnerUserID, createdBy, row.Kind, row.Title, row.DeviceID, row.Host, row.Domain, row.Username, blob, row.Notes).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchCredentialRecord(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if owner, found := s.credentialOwner(r.Context(), id); !found {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
		return
	} else if !admin && owner != actor {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "sem acesso a este registo", nil)
		return
	}
	var body credentialRecordBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if !admin {
		body.OwnerUserID = actor.String()
	}
	var existingBlob []byte
	if err := s.DB().QueryRow(r.Context(), `SELECT password_blob FROM credential_records WHERE id=$1`, id).Scan(&existingBlob); err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	row, blob, errMsg := s.prepareCredentialRecord(r, body, false, existingBlob)
	if errMsg != "" {
		writeErr(w, 422, "VALIDATION", errMsg, nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE credential_records SET
			owner_user_id=$2, kind=$3, title=$4, device_id=$5, host=$6, domain=$7,
			username=$8, password_blob=$9, notes=$10, updated_at=now()
		WHERE id=$1
	`, id, row.OwnerUserID, row.Kind, row.Title, row.DeviceID, row.Host, row.Domain, row.Username, blob, row.Notes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteCredentialRecord(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if owner, found := s.credentialOwner(r.Context(), id); !found {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
		return
	} else if !admin && owner != actor {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "sem acesso a este registo", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM credential_records WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) revealCredentialRecord(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := s.vaultActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	if owner, found := s.credentialOwner(r.Context(), id); !found {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
		return
	} else if !admin && owner != actor {
		writeErr(w, http.StatusForbidden, "FORBIDDEN", "sem acesso a este registo", nil)
		return
	}
	var blob []byte
	var username *string
	if err := s.DB().QueryRow(r.Context(), `
		SELECT password_blob, username FROM credential_records WHERE id=$1
	`, id).Scan(&blob, &username); err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "registo não encontrado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	key, err := s.vaultKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "VAULT", "não foi possível abrir o cofre", nil)
		return
	}
	plain, err := credentialvault.Decrypt(key, blob)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "VAULT", "falha ao decifrar a senha", nil)
		return
	}
	var revealedBy *uuid.UUID
	if actor != uuid.Nil {
		revealedBy = &actor
	}
	_, _ = s.DB().Exec(r.Context(), `
		INSERT INTO credential_record_reveals (record_id, revealed_by) VALUES ($1, $2)
	`, id, revealedBy)
	writeJSON(w, http.StatusOK, map[string]any{
		"username": username,
		"password": string(plain),
	})
}

func (s *Server) prepareCredentialRecord(r *http.Request, body credentialRecordBody, requirePassword bool, existingBlob []byte) (credentialRecordRow, []byte, string) {
	var out credentialRecordRow
	kind := normalizeCredentialKind(body.Kind)
	if kind == "" {
		return out, nil, "tipo inválido (equipment, server ou site)"
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(body.OwnerUserID))
	if err != nil {
		if actor := s.requestAuthContext(r).UserID; actor != uuid.Nil {
			ownerID = actor
		} else {
			return out, nil, "seleccione o utilizador dono do registo"
		}
	}
	mode := normalizeCredentialMode(body.Mode)
	user := optionalTrimmed(body.Username)
	if mode == "user_password" && user == nil {
		return out, nil, "informe o utilizador ou escolha «somente senha»"
	}
	if mode == "password" {
		user = nil
	}
	title := strings.TrimSpace(body.Title)
	notes := optionalTrimmed(body.Notes)
	var deviceID *uuid.UUID
	var host, domain *string
	switch kind {
	case "equipment":
		if body.DeviceID == nil || strings.TrimSpace(*body.DeviceID) == "" {
			return out, nil, "seleccione o equipamento"
		}
		id, err := uuid.Parse(strings.TrimSpace(*body.DeviceID))
		if err != nil {
			return out, nil, "equipamento inválido"
		}
		deviceID = &id
		if title == "" {
			var desc string
			_ = s.DB().QueryRow(r.Context(), `SELECT description FROM devices WHERE id=$1`, id).Scan(&desc)
			title = strings.TrimSpace(desc)
		}
	case "server":
		h := normalizeHost("")
		if body.Host != nil {
			h = normalizeHost(*body.Host)
		}
		if h == "" {
			return out, nil, "informe o IP ou host do servidor"
		}
		if len(h) > 255 {
			return out, nil, "host demasiado longo"
		}
		host = &h
		if title == "" {
			title = h
		}
	case "site":
		d := normalizeHost("")
		if body.Domain != nil {
			d = normalizeHost(*body.Domain)
		}
		if d == "" {
			return out, nil, "informe o domínio do site"
		}
		if len(d) > 255 {
			return out, nil, "domínio demasiado longo"
		}
		domain = &d
		if title == "" {
			title = d
		}
	}
	if title == "" {
		title = "Registo"
	}
	if len(title) > 200 {
		title = title[:200]
	}

	var blob []byte
	pass := optionalTrimmed(body.Password)
	if pass != nil {
		if len(*pass) > 512 {
			return out, nil, "senha demasiado longa"
		}
		key, err := s.vaultKey()
		if err != nil {
			return out, nil, "não foi possível abrir o cofre"
		}
		blob, err = credentialvault.Encrypt(key, []byte(*pass))
		if err != nil {
			return out, nil, "falha ao cifrar a senha"
		}
	} else if requirePassword {
		return out, nil, "informe a senha"
	} else {
		blob = existingBlob
	}

	out.OwnerUserID = ownerID
	out.Kind = kind
	out.Title = title
	out.DeviceID = deviceID
	out.Host = host
	out.Domain = domain
	out.Username = user
	out.Notes = notes
	return out, blob, ""
}
