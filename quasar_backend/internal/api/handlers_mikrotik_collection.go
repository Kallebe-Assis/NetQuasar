package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

type mikrotikCollectionResponse struct {
	Metrics         mikrotikcollect.MetricsConfig `json:"metrics"`
	CollectionSteps []mikrotikcollect.Step        `json:"collection_steps"`
	Catalog         []mikrotikcollect.CatalogEntry `json:"catalog"`
	Sections        map[string]string             `json:"sections"`
	CollectModes    map[string]string             `json:"collect_mode_labels"`
}

func (s *Server) getMikrotikCollection(w http.ResponseWriter, r *http.Request) {
	var metricsRaw, stepsRaw []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT coalesce(metrics::text, '{}'), coalesce(collection_steps::text, '[]')
		FROM settings_mikrotik_collection WHERE id = 1
	`).Scan(&metricsRaw, &stepsRaw)
	metrics := mikrotikcollect.DefaultMetrics()
	if err == nil {
		if parsed := mikrotikcollect.ParseMetrics(metricsRaw); len(parsed) > 0 {
			metrics = parsed.MergeWithDefaults()
		}
	}
	writeJSON(w, http.StatusOK, mikrotikCollectionResponse{
		Metrics:         metrics,
		CollectionSteps: mikrotikcollect.ParseSteps(stepsRaw),
		Catalog:         mikrotikcollect.MetricCatalog,
		Sections:        mikrotikcollect.SectionLabels,
		CollectModes: map[string]string{
			mikrotikcollect.ModeSNMPGet:          "SNMP GET (escalar)",
			mikrotikcollect.ModeSNMPWalk:         "SNMP WALK (tabela)",
			mikrotikcollect.ModeIFMibTable:       "IF-MIB (walk tabela)",
			mikrotikcollect.ModeIFMibStatus:      "IF-MIB status (parse)",
			mikrotikcollect.ModeIFMibPPPoE:       "PPPoE activo (IF-MIB)",
			mikrotikcollect.ModeOpticalSFPParse:  "Tabela SFP parseada",
			mikrotikcollect.ModeOpticalSFPColumn: "Coluna SFP (derivada)",
		},
	})
}

type patchMikrotikCollectionBody struct {
	Metrics         mikrotikcollect.MetricsConfig `json:"metrics"`
	CollectionSteps []mikrotikcollect.Step        `json:"collection_steps"`
}

func (s *Server) patchMikrotikCollection(w http.ResponseWriter, r *http.Request) {
	var body patchMikrotikCollectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	metrics := body.Metrics.Normalize()
	if metrics == nil {
		metrics = mikrotikcollect.MetricsConfig{}
	}
	// Garantir chaves válidas do catálogo
	clean := make(mikrotikcollect.MetricsConfig)
	for _, e := range mikrotikcollect.MetricCatalog {
		if def, ok := metrics[e.Key]; ok {
			if def.CollectMode == "" {
				def.CollectMode = e.DefaultMode
			}
			clean[e.Key] = def
		}
	}
	steps := body.CollectionSteps
	if steps == nil {
		steps = []mikrotikcollect.Step{}
	}
	mb, _ := json.Marshal(clean)
	sb, _ := json.Marshal(steps)
	_, err := s.DB().Exec(r.Context(), `
		INSERT INTO settings_mikrotik_collection (id, metrics, collection_steps, updated_at)
		VALUES (1, $1::jsonb, $2::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET
			metrics = EXCLUDED.metrics,
			collection_steps = EXCLUDED.collection_steps,
			updated_at = now()
	`, mb, sb)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_mikrotik_collection", "1", "patch", actorFromRequest(r), nil, map[string]any{
		"has_enabled": mikrotikcollect.HasEnabledMetrics(clean),
		"steps":       len(steps),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"metrics":          clean,
		"collection_steps": mikrotikcollect.ParseSteps(sb),
		"has_enabled":      mikrotikcollect.HasEnabledMetrics(clean),
		"message":          patchMikrotikSaveMessage(clean),
	})
}

func patchMikrotikSaveMessage(c mikrotikcollect.MetricsConfig) string {
	var missing []string
	for _, e := range mikrotikcollect.MetricCatalog {
		def, ok := c[e.Key]
		if !ok || !def.Enabled {
			continue
		}
		if strings.TrimSpace(def.OID) == "" {
			missing = append(missing, e.Label)
		}
	}
	if len(missing) == 0 {
		return "Perfil MikroTik guardado."
	}
	return "Perfil guardado. Métricas activas sem OID (não serão colectadas): " + strings.Join(missing, ", ")
}
