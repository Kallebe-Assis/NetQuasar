// handlers_system_config_io.go — endpoints HTTP de exportação/importação de configuração (aba Base de dados).
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// exportSystemConfiguration GET /settings/system-config/export — descarrega JSON completo.
func (s *Server) exportSystemConfiguration(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.collectSystemConfigurationBundle(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "EXPORT", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "system_configuration", "export", "export", s.actorFromRequest(r), nil, map[string]any{
		"schema_version": systemConfigExportVersion,
	})
	filename := fmt.Sprintf("netquasar-config-%s.json", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(bundle)
}

// startSystemConfigurationImport POST /settings/system-config/import — inicia job assíncrono.
func (s *Server) startSystemConfigurationImport(w http.ResponseWriter, r *http.Request) {
	var body systemConfigImportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Bundle == nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "campo bundle obrigatório", nil)
		return
	}
	schema, _ := body.Bundle["_schema"].(map[string]any)
	kind, _ := schema["kind"].(string)
	if kind != "" && kind != "netquasar_system_configuration" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "ficheiro não é um backup NetQuasar válido", nil)
		return
	}

	jobID := uuid.NewString()
	job := &sysConfigImportJob{
		ID: jobID, Status: "running", ProgressPct: 0,
		CurrentStep: "A preparar…", Logs: []string{}, Errors: []string{},
		StartedAt: time.Now(),
	}
	s.sysCfgImportMu.Lock()
	s.sysCfgImportJobs[jobID] = job
	s.sysCfgImportMu.Unlock()

	go s.runSystemConfigurationImport(jobID, body.Bundle, body.Options)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":  jobID,
		"status":  "running",
		"message": "Importação iniciada. Consulte GET /settings/system-config/import/{jobId} para progresso.",
	})
}

// getSystemConfigurationImportJob GET /settings/system-config/import/{jobId} — estado e logs do job.
func (s *Server) getSystemConfigurationImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	s.sysCfgImportMu.Lock()
	job, ok := s.sysCfgImportJobs[jobID]
	s.sysCfgImportMu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "job de importação não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
