package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

type oltVendorModelRow struct {
	Brand           string                     `json:"brand"`
	Model           string                     `json:"model"`
	OnuOnlineOID    *string                    `json:"onu_online_oid"`
	PonStatusOID    *string                    `json:"pon_status_oid"`
	TransceiverOID  *string                    `json:"transceiver_oid"`
	SNMPBaseOID     *string                    `json:"snmp_base_oid"`
	OnuMetrics         oltcollect.OnuMetricsConfig `json:"onu_metrics"`
	CollectionSteps    []oltcollect.Step           `json:"collection_steps"`
	OnuReportCommands  oltcollect.OnuReportConfig  `json:"onu_report_commands"`
	PonTelnetCommands  oltcollect.PonTelnetConfig  `json:"pon_telnet_commands"`
}

func normalizeOltBrandModel(brand, model string) (string, string, bool) {
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)
	if brand == "" || model == "" {
		return "", "", false
	}
	return brand, model, true
}

func (s *Server) getOltModelsCatalog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT brand, model FROM olt_vendor_models
		WHERE model <> 'Padrão'
		ORDER BY brand, model
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	catalog := map[string][]string{}
	for rows.Next() {
		var brand, model string
		if err := rows.Scan(&brand, &model); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		catalog[brand] = append(catalog[brand], model)
	}
	writeJSON(w, http.StatusOK, map[string]any{"catalog": catalog})
}

func (s *Server) listOltVendorModels(w http.ResponseWriter, r *http.Request) {
	brand := strings.TrimSpace(chi.URLParam(r, "brand"))
	if brand == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "marca obrigatória", nil)
		return
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT brand, model,
			onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid,
			coalesce(onu_metrics::text, '{}'),
			coalesce(collection_steps::text, '[]'),
			coalesce(onu_report_commands::text, '{}'),
			coalesce(pon_telnet_commands::text, '{}')
		FROM olt_vendor_models
		WHERE brand = $1 AND model <> 'Padrão'
		ORDER BY model
	`, brand)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []oltVendorModelRow
	for rows.Next() {
		var row oltVendorModelRow
		var stepsRaw, metricsRaw, reportRaw, ponTelnetRaw []byte
		if err := rows.Scan(&row.Brand, &row.Model, &row.OnuOnlineOID, &row.PonStatusOID, &row.TransceiverOID, &row.SNMPBaseOID, &metricsRaw, &stepsRaw, &reportRaw, &ponTelnetRaw); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		row.CollectionSteps = oltcollect.ParseSteps(stepsRaw)
		row.OnuMetrics = oltcollect.ParseOnuMetrics(metricsRaw)
		row.OnuReportCommands = oltcollect.ParseOnuReportConfig(reportRaw)
		row.PonTelnetCommands = oltcollect.ParsePonTelnetConfig(ponTelnetRaw)
		list = append(list, row)
	}
	if list == nil {
		list = []oltVendorModelRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"brand": brand, "models": list})
}

func (s *Server) getOltVendorModel(w http.ResponseWriter, r *http.Request) {
	brand, model, ok := normalizeOltBrandModel(chi.URLParam(r, "brand"), chi.URLParam(r, "model"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "marca e modelo obrigatórios", nil)
		return
	}
	row, err := s.queryOltVendorModel(r, brand, model)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "modelo não cadastrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) queryOltVendorModel(r *http.Request, brand, model string) (oltVendorModelRow, error) {
	var row oltVendorModelRow
	var stepsRaw, metricsRaw, reportRaw, ponTelnetRaw []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT brand, model, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid,
			coalesce(onu_metrics::text, '{}'),
			coalesce(collection_steps::text, '[]'),
			coalesce(onu_report_commands::text, '{}'),
			coalesce(pon_telnet_commands::text, '{}')
		FROM olt_vendor_models WHERE brand = $1 AND model = $2
	`, brand, model).Scan(&row.Brand, &row.Model, &row.OnuOnlineOID, &row.PonStatusOID, &row.TransceiverOID, &row.SNMPBaseOID, &metricsRaw, &stepsRaw, &reportRaw, &ponTelnetRaw)
	if err == nil {
		row.CollectionSteps = oltcollect.ParseSteps(stepsRaw)
		row.OnuMetrics = oltcollect.ParseOnuMetrics(metricsRaw)
		row.OnuReportCommands = oltcollect.ParseOnuReportConfig(reportRaw)
		row.PonTelnetCommands = oltcollect.ParsePonTelnetConfig(ponTelnetRaw)
	}
	return row, err
}

func (s *Server) createOltVendorModel(w http.ResponseWriter, r *http.Request) {
	brand := strings.TrimSpace(chi.URLParam(r, "brand"))
	if brand == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "marca obrigatória", nil)
		return
	}
	var body struct {
		Model           string                      `json:"model"`
		OnuOnlineOID    *string                     `json:"onu_online_oid"`
		PonStatusOID    *string                     `json:"pon_status_oid"`
		TransceiverOID  *string                     `json:"transceiver_oid"`
		SNMPBaseOID     *string                     `json:"snmp_base_oid"`
		OnuMetrics      oltcollect.OnuMetricsConfig `json:"onu_metrics"`
		CollectionSteps []oltcollect.Step           `json:"collection_steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "nome do modelo obrigatório", nil)
		return
	}
	if strings.EqualFold(model, "Padrão") {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "nome de modelo reservado", nil)
		return
	}
	stepsJSON := collectionStepsJSON(body.CollectionSteps)
	metricsJSON := onuMetricsJSON(body.OnuMetrics)
	_, err := s.DB().Exec(r.Context(), `
		INSERT INTO olt_vendor_models (brand, model, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid, onu_metrics, collection_steps)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb)
	`, brand, model, body.OnuOnlineOID, body.PonStatusOID, body.TransceiverOID, body.SNMPBaseOID, metricsJSON, stepsJSON)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			writeErr(w, http.StatusConflict, "DUPLICATE", "modelo já existe para esta marca", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "olt_vendor_model", brand+"/"+model, "create", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "brand": brand, "model": model})
}

func (s *Server) patchOltVendorModel(w http.ResponseWriter, r *http.Request) {
	brand, model, ok := normalizeOltBrandModel(chi.URLParam(r, "brand"), chi.URLParam(r, "model"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "marca e modelo obrigatórios", nil)
		return
	}
	var body struct {
		OnuOnlineOID       *string                     `json:"onu_online_oid"`
		PonStatusOID       *string                     `json:"pon_status_oid"`
		TransceiverOID     *string                     `json:"transceiver_oid"`
		SNMPBaseOID        *string                     `json:"snmp_base_oid"`
		OnuMetrics         oltcollect.OnuMetricsConfig `json:"onu_metrics"`
		CollectionSteps    []oltcollect.Step           `json:"collection_steps"`
		OnuReportCommands  *oltcollect.OnuReportConfig `json:"onu_report_commands"`
		PonTelnetCommands  *oltcollect.PonTelnetConfig `json:"pon_telnet_commands"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var stepsArg, metricsArg, reportArg, ponTelnetArg any
	if body.CollectionSteps != nil {
		stepsArg = collectionStepsJSON(body.CollectionSteps)
	}
	if body.OnuMetrics != nil {
		metricsArg = onuMetricsJSON(body.OnuMetrics)
	}
	if body.OnuReportCommands != nil {
		reportArg = onuReportCommandsJSON(*body.OnuReportCommands)
	}
	if body.PonTelnetCommands != nil {
		ponTelnetArg = ponTelnetCommandsJSON(*body.PonTelnetCommands)
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE olt_vendor_models SET
			onu_online_oid = COALESCE($3, onu_online_oid),
			pon_status_oid = COALESCE($4, pon_status_oid),
			transceiver_oid = COALESCE($5, transceiver_oid),
			snmp_base_oid = COALESCE($6, snmp_base_oid),
			onu_metrics = COALESCE($7::jsonb, onu_metrics),
			collection_steps = COALESCE($8::jsonb, collection_steps),
			onu_report_commands = COALESCE($9::jsonb, onu_report_commands),
			pon_telnet_commands = COALESCE($10::jsonb, pon_telnet_commands),
			updated_at = now()
		WHERE brand = $1 AND model = $2
	`, brand, model, body.OnuOnlineOID, body.PonStatusOID, body.TransceiverOID, body.SNMPBaseOID, metricsArg, stepsArg, reportArg, ponTelnetArg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "modelo não cadastrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "olt_vendor_model", brand+"/"+model, "patch", s.actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteOltVendorModel(w http.ResponseWriter, r *http.Request) {
	brand, model, ok := normalizeOltBrandModel(chi.URLParam(r, "brand"), chi.URLParam(r, "model"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "marca e modelo obrigatórios", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM olt_vendor_models WHERE brand = $1 AND model = $2`, brand, model)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "modelo não cadastrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "olt_vendor_model", brand+"/"+model, "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
