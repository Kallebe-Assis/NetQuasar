package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// handlers_bgp_carriers.go — cadastro de operadoras (127_bgp_carriers.sql): CNPJ, endereço,
// 1+ AS (Autonomous System) e limite de banda contratado. Global (sem device_id) — a mesma
// operadora pode servir mais de um equipamento BGP. bgp_uplink_interfaces.carrier_id referencia
// esta tabela (a antiga bgp_uplink_carrier_limits foi removida; o limite agora vive aqui).

type bgpCarrier struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Document           string   `json:"document"`
	Address            string   `json:"address"`
	BandwidthLimitMbps *float64 `json:"bandwidth_limit_mbps,omitempty"`
	ASNumbers          []int64  `json:"as_numbers"`
}

func loadBgpCarriers(r *http.Request, s *Server) ([]bgpCarrier, error) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, name, document, address, bandwidth_limit_mbps
		FROM bgp_carriers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	out := make([]bgpCarrier, 0, 8)
	byID := map[string]*bgpCarrier{}
	for rows.Next() {
		var c bgpCarrier
		var id uuid.UUID
		if err := rows.Scan(&id, &c.Name, &c.Document, &c.Address, &c.BandwidthLimitMbps); err != nil {
			rows.Close()
			return nil, err
		}
		c.ID = id.String()
		c.ASNumbers = []int64{}
		out = append(out, c)
		byID[c.ID] = &out[len(out)-1]
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	asRows, err := s.DB().Query(r.Context(), `
		SELECT carrier_id, as_number FROM bgp_carrier_as_numbers ORDER BY carrier_id, sort_order
	`)
	if err != nil {
		return nil, err
	}
	defer asRows.Close()
	for asRows.Next() {
		var carrierID uuid.UUID
		var asNum int64
		if err := asRows.Scan(&carrierID, &asNum); err != nil {
			return nil, err
		}
		if c, ok := byID[carrierID.String()]; ok {
			c.ASNumbers = append(c.ASNumbers, asNum)
		}
	}
	return out, asRows.Err()
}

func (s *Server) listBgpCarriers(w http.ResponseWriter, r *http.Request) {
	carriers, err := loadBgpCarriers(r, s)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"carriers": carriers})
}

type bgpCarrierUpsertBody struct {
	Name               string   `json:"name"`
	Document           string   `json:"document"`
	Address            string   `json:"address"`
	BandwidthLimitMbps *float64 `json:"bandwidth_limit_mbps"`
	ASNumbers          []int64  `json:"as_numbers"`
}

func replaceBgpCarrierASNumbers(r *http.Request, s *Server, carrierID uuid.UUID, asNumbers []int64) error {
	if _, err := s.DB().Exec(r.Context(), `DELETE FROM bgp_carrier_as_numbers WHERE carrier_id=$1`, carrierID); err != nil {
		return err
	}
	for i, n := range asNumbers {
		if _, err := s.DB().Exec(r.Context(), `
			INSERT INTO bgp_carrier_as_numbers (carrier_id, as_number, sort_order) VALUES ($1,$2,$3)
		`, carrierID, n, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) createBgpCarrier(w http.ResponseWriter, r *http.Request) {
	var body bgpCarrierUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome da operadora é obrigatório", nil)
		return
	}
	var newID uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO bgp_carriers (name, document, address, bandwidth_limit_mbps)
		VALUES ($1,$2,$3,$4) RETURNING id
	`, body.Name, strings.TrimSpace(body.Document), strings.TrimSpace(body.Address), body.BandwidthLimitMbps).Scan(&newID)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "já existe uma operadora com este nome", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if err := replaceBgpCarrierASNumbers(r, s, newID, body.ASNumbers); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func (s *Server) updateBgpCarrier(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "carrierId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body bgpCarrierUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nome da operadora é obrigatório", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE bgp_carriers SET name=$1, document=$2, address=$3, bandwidth_limit_mbps=$4, updated_at=now()
		WHERE id=$5
	`, body.Name, strings.TrimSpace(body.Document), strings.TrimSpace(body.Address), body.BandwidthLimitMbps, id)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "já existe uma operadora com este nome", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "operadora não encontrada", nil)
		return
	}
	if err := replaceBgpCarrierASNumbers(r, s, id, body.ASNumbers); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteBgpCarrier(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "carrierId"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var inUse int
	if err := s.DB().QueryRow(r.Context(), `SELECT count(*) FROM bgp_uplink_interfaces WHERE carrier_id=$1`, id).Scan(&inUse); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if inUse > 0 {
		writeErr(w, http.StatusConflict, "IN_USE", "esta operadora está em uso por interfaces configuradas — remova-as primeiro", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM bgp_carriers WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "operadora não encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
