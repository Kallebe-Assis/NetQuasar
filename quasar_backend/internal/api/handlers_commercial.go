package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Server) listLocalities(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT id, name, region_code, created_at FROM commercial_localities ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var name string
		var rcPtr *string
		var created time.Time
		if err := rows.Scan(&id, &name, &rcPtr, &created); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{"id": id, "name": name, "region_code": rcPtr, "created_at": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"localities": list})
}

func (s *Server) createLocality(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string  `json:"name"`
		RegionCode *string `json:"region_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name obrigatório", nil)
		return
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `INSERT INTO commercial_localities (name, region_code) VALUES ($1,$2) RETURNING id`, body.Name, body.RegionCode).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "localidade já existe", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "create", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (s *Server) getLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var name string
	var rc *string
	var created time.Time
	err = s.DB().QueryRow(r.Context(), `SELECT name, region_code, created_at FROM commercial_localities WHERE id=$1`, id).Scan(&name, &rc, &created)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": name, "region_code": rc, "created_at": created})
}

func (s *Server) patchLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		Name       *string `json:"name"`
		RegionCode *string `json:"region_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `UPDATE commercial_localities SET name=COALESCE($2,name), region_code=COALESCE($3, region_code) WHERE id=$1`, id, body.Name, body.RegionCode)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "patch", actorFromRequest(r), nil, body)
	s.getLocality(w, r)
}

func (s *Server) deleteLocality(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	_, err = s.DB().Exec(r.Context(), `DELETE FROM commercial_localities WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_locality", id.String(), "delete", actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listMonthlyRecords(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, locality_id, year_month, client_count, created_at FROM commercial_monthly_records ORDER BY year_month DESC LIMIT 500
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id, lid uuid.UUID
		var ym string
		var c int
		var cr time.Time
		if err := rows.Scan(&id, &lid, &ym, &c, &cr); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{"id": id, "locality_id": lid, "year_month": ym, "client_count": c, "created_at": cr})
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": list})
}

func (s *Server) createMonthlyRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LocalityID  uuid.UUID `json:"locality_id"`
		YearMonth   string    `json:"year_month"`
		ClientCount int       `json:"client_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO commercial_monthly_records (locality_id, year_month, client_count) VALUES ($1,$2,$3) RETURNING id
	`, body.LocalityID, body.YearMonth, body.ClientCount).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "registro mês+localidade duplicado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_monthly_record", id.String(), "create", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getMonthlyRecord(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var lid uuid.UUID
	var ym string
	var c int
	var created, updated time.Time
	err = s.DB().QueryRow(r.Context(), `
		SELECT locality_id, year_month, client_count, created_at, updated_at FROM commercial_monthly_records WHERE id=$1
	`, id).Scan(&lid, &ym, &c, &created, &updated)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "locality_id": lid, "year_month": ym, "client_count": c, "created_at": created, "updated_at": updated,
	})
}

func (s *Server) patchMonthlyRecord(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		YearMonth   *string `json:"year_month"`
		ClientCount *int    `json:"client_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE commercial_monthly_records SET
			year_month = COALESCE($2, year_month),
			client_count = COALESCE($3, client_count),
			updated_at = now()
		WHERE id=$1
	`, id, body.YearMonth, body.ClientCount)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "mês+localidade duplicado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_monthly_record", id.String(), "patch", actorFromRequest(r), nil, body)
	s.getMonthlyRecord(w, r)
}

func (s *Server) deleteMonthlyRecord(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM commercial_monthly_records WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_monthly_record", id.String(), "delete", actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bulkMonthlyRecords(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Records []struct {
			LocalityID  uuid.UUID `json:"locality_id"`
			YearMonth   string    `json:"year_month"`
			ClientCount int       `json:"client_count"`
		} `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Records) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "records não vazio", nil)
		return
	}
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	for _, rec := range body.Records {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO commercial_monthly_records (locality_id, year_month, client_count) VALUES ($1,$2,$3)
			ON CONFLICT (locality_id, year_month) DO UPDATE SET client_count = EXCLUDED.client_count, updated_at = now()
		`, rec.LocalityID, rec.YearMonth, rec.ClientCount); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "commercial_monthly_record", "bulk", "bulk_upsert", actorFromRequest(r), nil, map[string]any{"count": len(body.Records)})
	writeJSON(w, http.StatusOK, map[string]any{"upserted": len(body.Records)})
}

func (s *Server) commercialAggregates(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	q := `
		SELECT COALESCE(SUM(client_count),0)::bigint FROM commercial_monthly_records
	`
	args := []any{}
	if month != "" {
		q += ` WHERE year_month = $1`
		args = append(args, month)
	}
	var total int64
	if err := s.DB().QueryRow(r.Context(), q, args...).Scan(&total); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"total_clients": total, "month": month})
}

// commercialTotalsHistory devolve totais agregados por mês (YYYY-MM) para gráficos de evolução.
func (s *Server) commercialTotalsHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("months"))
	if limit <= 0 || limit > 60 {
		limit = 18
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT year_month, COALESCE(SUM(client_count), 0)::bigint AS total
		FROM commercial_monthly_records
		GROUP BY year_month
		ORDER BY year_month DESC
		LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type row struct {
		month string
		total int64
	}
	var raw []row
	for rows.Next() {
		var ym string
		var total int64
		if err := rows.Scan(&ym, &total); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		raw = append(raw, row{month: ym, total: total})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	// Ordem cronológica ascendente para gráficos.
	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}
	out := make([]map[string]any, 0, len(raw))
	var prev int64
	for i, r := range raw {
		delta := int64(0)
		var deltaPct *float64
		if i > 0 {
			delta = r.total - prev
			if prev > 0 {
				p := float64(delta) / float64(prev) * 100
				deltaPct = &p
			}
		}
		item := map[string]any{
			"year_month": r.month,
			"total":      r.total,
			"delta":      delta,
		}
		if deltaPct != nil {
			item["delta_percent"] = *deltaPct
		}
		out = append(out, item)
		prev = r.total
	}
	writeJSON(w, http.StatusOK, map[string]any{"months": out})
}
