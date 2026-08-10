package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type localityRow struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	RegionCode   *string    `json:"region_code,omitempty"`
	UF           *string    `json:"uf,omitempty"`
	Address      *string    `json:"address,omitempty"`
	Latitude     *float64   `json:"latitude,omitempty"`
	Longitude    *float64   `json:"longitude,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	ClientCount  *int       `json:"client_count,omitempty"`
	ClientMonth  *string    `json:"client_month,omitempty"`
	VLANs        []string   `json:"vlans"`
	Pops         []popBrief `json:"pops"`
	PrimaryPopID *uuid.UUID `json:"pop_id,omitempty"`
	PrimaryPop   *string    `json:"pop_description,omitempty"`
}

type popBrief struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	DeviceCount int64     `json:"device_count"`
}

func (s *Server) listLocalities(w http.ResponseWriter, r *http.Request) {
	list, err := s.loadLocalities(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"localities": list})
}

func (s *Server) loadLocalities(ctx context.Context) ([]localityRow, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT cl.id, cl.name, cl.region_code, cl.uf, cl.address, cl.latitude, cl.longitude,
		       cl.created_at, cl.updated_at,
		       cmr.client_count, cmr.year_month
		FROM commercial_localities cl
		LEFT JOIN LATERAL (
			SELECT client_count, year_month
			FROM commercial_monthly_records
			WHERE locality_id = cl.id
			ORDER BY year_month DESC
			LIMIT 1
		) cmr ON true
		ORDER BY cl.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]localityRow, 0)
	idx := map[uuid.UUID]int{}
	for rows.Next() {
		var lr localityRow
		var updated *time.Time
		if err := rows.Scan(
			&lr.ID, &lr.Name, &lr.RegionCode, &lr.UF, &lr.Address, &lr.Latitude, &lr.Longitude,
			&lr.CreatedAt, &updated, &lr.ClientCount, &lr.ClientMonth,
		); err != nil {
			return nil, err
		}
		lr.UpdatedAt = updated
		lr.VLANs = []string{}
		lr.Pops = []popBrief{}
		idx[lr.ID] = len(out)
		out = append(out, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	vrows, err := s.DB().Query(ctx, `SELECT locality_id, vlan FROM locality_vlans ORDER BY vlan`)
	if err == nil {
		defer vrows.Close()
		for vrows.Next() {
			var lid uuid.UUID
			var vlan string
			if err := vrows.Scan(&lid, &vlan); err != nil {
				return nil, err
			}
			if i, ok := idx[lid]; ok {
				out[i].VLANs = append(out[i].VLANs, vlan)
			}
		}
	}

	prows, err := s.DB().Query(ctx, `
		SELECT p.id, p.locality_id, p.description,
			(SELECT COUNT(*) FROM devices d WHERE d.pop_id = p.id)
		FROM pops p
		WHERE p.locality_id IS NOT NULL
		ORDER BY p.description
	`)
	if err == nil {
		defer prows.Close()
		for prows.Next() {
			var pid, lid uuid.UUID
			var desc string
			var dc int64
			if err := prows.Scan(&pid, &lid, &desc, &dc); err != nil {
				return nil, err
			}
			if i, ok := idx[lid]; ok {
				out[i].Pops = append(out[i].Pops, popBrief{ID: pid, Description: desc, DeviceCount: dc})
				if out[i].PrimaryPopID == nil {
					idCopy := pid
					out[i].PrimaryPopID = &idCopy
					d := desc
					out[i].PrimaryPop = &d
				}
			}
		}
	}
	return out, nil
}

func (s *Server) createLocality(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name              string   `json:"name"`
		RegionCode        *string  `json:"region_code"`
		UF                *string  `json:"uf"`
		Address           *string  `json:"address"`
		Latitude          *float64 `json:"latitude"`
		Longitude         *float64 `json:"longitude"`
		CreatePop         bool     `json:"create_pop"`
		PopName           *string  `json:"pop_name"`
		VLANs             []string `json:"vlans"`
		ConfirmSharedVLAN bool     `json:"confirm_shared_vlan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name obrigatório", nil)
		return
	}
	uf := coalesceTrimPtr(body.UF, body.RegionCode)
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO commercial_localities (name, region_code, uf, address, latitude, longitude, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6, now()) RETURNING id
	`, strings.TrimSpace(body.Name), uf, uf, nullBlankPtr(body.Address), body.Latitude, body.Longitude).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "localidade já existe", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	if body.CreatePop {
		popName := strings.TrimSpace(body.Name)
		if body.PopName != nil && strings.TrimSpace(*body.PopName) != "" {
			popName = strings.TrimSpace(*body.PopName)
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO pops (description, address, latitude, longitude, locality_id)
			VALUES ($1,$2,$3,$4,$5)
		`, popName, nullBlankPtr(body.Address), body.Latitude, body.Longitude, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	vlans := normalizeVLANList(body.VLANs)
	if len(vlans) > 0 {
		shared, err := s.findSharedVLANsTx(r.Context(), tx, uuid.Nil, vlans)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if len(shared) > 0 && !body.ConfirmSharedVLAN {
			_ = tx.Rollback(r.Context())
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "SHARED_VLAN",
				"message":          "Uma ou mais VLANs ja estao atreladas a outras localidades.",
				"shared_vlans":     shared,
				"requires_confirm": true,
			})
			return
		}
		if err := s.replaceLocalityVLANsTx(r.Context(), tx, id, vlans); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	list, err := s.loadLocalities(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	for _, lr := range list {
		if lr.ID == id {
			writeJSON(w, http.StatusOK, lr)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
}

func (s *Server) patchLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Name              *string   `json:"name"`
		RegionCode        *string   `json:"region_code"`
		UF                *string   `json:"uf"`
		Address           *string   `json:"address"`
		Latitude          *float64  `json:"latitude"`
		Longitude         *float64  `json:"longitude"`
		CreatePop         *bool     `json:"create_pop"`
		PopName           *string   `json:"pop_name"`
		VLANs             *[]string `json:"vlans"`
		ConfirmSharedVLAN bool      `json:"confirm_shared_vlan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}

	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	uf := body.UF
	if uf == nil {
		uf = body.RegionCode
	}
	ct, err := tx.Exec(r.Context(), `
		UPDATE commercial_localities SET
			name = COALESCE($2, name),
			region_code = COALESCE($3, region_code),
			uf = COALESCE($3, uf),
			address = COALESCE($4, address),
			latitude = COALESCE($5, latitude),
			longitude = COALESCE($6, longitude),
			updated_at = now()
		WHERE id=$1
	`, id, body.Name, uf, body.Address, body.Latitude, body.Longitude)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "localidade ja existe", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}

	if body.CreatePop != nil && *body.CreatePop {
		var name string
		var addr *string
		var lat, lon *float64
		_ = tx.QueryRow(r.Context(), `SELECT name, address, latitude, longitude FROM commercial_localities WHERE id=$1`, id).Scan(&name, &addr, &lat, &lon)
		popName := strings.TrimSpace(name)
		if body.PopName != nil && strings.TrimSpace(*body.PopName) != "" {
			popName = strings.TrimSpace(*body.PopName)
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO pops (description, address, latitude, longitude, locality_id)
			VALUES ($1,$2,$3,$4,$5)
		`, popName, addr, lat, lon, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	if body.VLANs != nil {
		vlans := normalizeVLANList(*body.VLANs)
		shared, err := s.findSharedVLANsTx(r.Context(), tx, id, vlans)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if len(shared) > 0 && !body.ConfirmSharedVLAN {
			_ = tx.Rollback(r.Context())
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "SHARED_VLAN",
				"message":          "Uma ou mais VLANs ja estao atreladas a outras localidades.",
				"shared_vlans":     shared,
				"requires_confirm": true,
			})
			return
		}
		if err := s.replaceLocalityVLANsTx(r.Context(), tx, id, vlans); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "patch", s.actorFromRequest(r), nil, body)
	s.getLocality(w, r)
}

func (s *Server) deleteLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var popN int
	_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM pops WHERE locality_id=$1`, id).Scan(&popN)
	if popN > 0 {
		writeErr(w, http.StatusConflict, "HAS_POP", "Remova ou reassocie os POPs antes de excluir a localidade.", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `DELETE FROM commercial_localities WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listBNGCollectedVLANs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT DISTINCT trim(vlan) AS vlan
		FROM bng_known_logins
		WHERE vlan IS NOT NULL AND trim(vlan) <> '' AND trim(vlan) <> '0'
		ORDER BY 1
	`)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"vlans": []string{}})
		return
	}
	defer rows.Close()
	vlans := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		vlans = append(vlans, v)
	}
	if len(vlans) == 0 {
		var raw []byte
		err = s.DB().QueryRow(r.Context(), `
			SELECT data::text FROM bng_session_snapshots
			ORDER BY captured_at DESC LIMIT 1
		`).Scan(&raw)
		if err == nil && len(raw) > 0 {
			seen := map[string]struct{}{}
			var doc map[string]any
			if json.Unmarshal(raw, &doc) == nil {
				if arr, ok := doc["sessions"].([]any); ok {
					for _, it := range arr {
						m, _ := it.(map[string]any)
						if m == nil {
							continue
						}
						v := strings.TrimSpace(fmt.Sprint(m["vlan"]))
						if v == "" || v == "0" || v == "<nil>" {
							continue
						}
						if _, ok := seen[v]; ok {
							continue
						}
						seen[v] = struct{}{}
						vlans = append(vlans, v)
					}
				}
			}
			sort.Strings(vlans)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"vlans": vlans, "count": len(vlans)})
}

func (s *Server) checkLocalityVLANShare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LocalityID *string  `json:"locality_id"`
		VLANs      []string `json:"vlans"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	exclude := uuid.Nil
	if body.LocalityID != nil && strings.TrimSpace(*body.LocalityID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.LocalityID))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "locality_id invalido", nil)
			return
		}
		exclude = id
	}
	shared, err := s.findSharedVLANs(r.Context(), exclude, normalizeVLANList(body.VLANs))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shared_vlans": shared, "has_shared": len(shared) > 0})
}

type sharedVLANInfo struct {
	VLAN       string   `json:"vlan"`
	Localities []string `json:"localities"`
}

func (s *Server) findSharedVLANs(ctx context.Context, excludeLocality uuid.UUID, vlans []string) ([]sharedVLANInfo, error) {
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return s.findSharedVLANsTx(ctx, tx, excludeLocality, vlans)
}

func (s *Server) findSharedVLANsTx(ctx context.Context, tx pgx.Tx, excludeLocality uuid.UUID, vlans []string) ([]sharedVLANInfo, error) {
	if len(vlans) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT lv.vlan, cl.name
		FROM locality_vlans lv
		JOIN commercial_localities cl ON cl.id = lv.locality_id
		WHERE lv.vlan = ANY($1::text[])
		  AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR lv.locality_id <> $2)
		ORDER BY lv.vlan, cl.name
	`, vlans, excludeLocality)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byVLAN := map[string][]string{}
	for rows.Next() {
		var vlan, name string
		if err := rows.Scan(&vlan, &name); err != nil {
			return nil, err
		}
		byVLAN[vlan] = append(byVLAN[vlan], name)
	}
	out := make([]sharedVLANInfo, 0, len(byVLAN))
	for _, v := range vlans {
		if locs, ok := byVLAN[v]; ok && len(locs) > 0 {
			out = append(out, sharedVLANInfo{VLAN: v, Localities: locs})
		}
	}
	return out, rows.Err()
}

func (s *Server) replaceLocalityVLANsTx(ctx context.Context, tx pgx.Tx, localityID uuid.UUID, vlans []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM locality_vlans WHERE locality_id=$1`, localityID); err != nil {
		return err
	}
	for _, v := range vlans {
		if _, err := tx.Exec(ctx, `
			INSERT INTO locality_vlans (locality_id, vlan) VALUES ($1,$2)
			ON CONFLICT (locality_id, vlan) DO NOTHING
		`, localityID, v); err != nil {
			return err
		}
	}
	return nil
}

func normalizeVLANList(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" || v == "0" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func coalesceTrimPtr(a, b *string) *string {
	if a != nil && strings.TrimSpace(*a) != "" {
		s := strings.TrimSpace(*a)
		return &s
	}
	if b != nil && strings.TrimSpace(*b) != "" {
		s := strings.TrimSpace(*b)
		return &s
	}
	return nil
}

func nullBlankPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}
