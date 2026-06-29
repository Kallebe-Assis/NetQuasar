package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
)

type bngCollectionResponse struct {
	Metrics      bngcollect.MetricsConfig   `json:"metrics"`
	Options      bngcollect.CollectionOptions `json:"options"`
	Catalog      []bngcollect.CatalogEntry  `json:"catalog"`
	Sections     map[string]string          `json:"sections"`
	CollectModes map[string]string          `json:"collect_mode_labels"`
}

func (s *Server) getBngCollection(w http.ResponseWriter, r *http.Request) {
	var metricsRaw, optionsRaw []byte
	err := s.DB().QueryRow(r.Context(), `
		SELECT coalesce(metrics::text, '{}'), coalesce(options::text, '{}')
		FROM settings_bng_collection WHERE id = 1
	`).Scan(&metricsRaw, &optionsRaw)
	metrics := bngcollect.DefaultMetrics()
	if err == nil {
		if parsed := bngcollect.ParseMetrics(metricsRaw); len(parsed) > 0 {
			metrics = parsed.MergeWithDefaults()
		}
	}
	writeJSON(w, http.StatusOK, bngCollectionResponse{
		Metrics:  metrics,
		Options:  bngcollect.ParseCollectionOptions(optionsRaw),
		Catalog:  bngcollect.MetricCatalog,
		Sections: bngcollect.SectionLabels,
		CollectModes: map[string]string{
			bngcollect.ModeSNMPGet:        "SNMP GET (escalar)",
			bngcollect.ModeSNMPWalk:       "SNMP WALK (coluna/tabela)",
			bngcollect.ModeAccessSessions: "Sessões PPPoE (walk múltiplo)",
		},
	})
}

type patchBngCollectionBody struct {
	Metrics bngcollect.MetricsConfig     `json:"metrics"`
	Options bngcollect.CollectionOptions `json:"options"`
}

func (s *Server) patchBngCollection(w http.ResponseWriter, r *http.Request) {
	var body patchBngCollectionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "JSON inválido", nil)
		return
	}
	clean := body.Metrics.Normalize()
	if clean == nil {
		clean = bngcollect.MetricsConfig{}
	}
	options := body.Options
	mb, _ := json.Marshal(clean)
	ob, _ := json.Marshal(options)
	_, err := s.DB().Exec(r.Context(), `
		INSERT INTO settings_bng_collection (id, metrics, options, updated_at)
		VALUES (1, $1::jsonb, $2::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET metrics = EXCLUDED.metrics, options = EXCLUDED.options, updated_at = now()
	`, mb, ob)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "settings_bng_collection", "1", "patch", s.actorFromRequest(r), nil, map[string]any{
		"has_enabled": bngcollect.HasEnabledMetrics(clean),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"metrics":     clean,
		"options":     options,
		"has_enabled": bngcollect.HasEnabledMetrics(clean),
		"message":     patchBngSaveMessage(clean),
	})
}

func patchBngSaveMessage(c bngcollect.MetricsConfig) string {
	var missing []string
	for _, e := range bngcollect.MetricCatalog {
		def, ok := c[e.Key]
		if !ok || !def.Enabled {
			continue
		}
		if strings.TrimSpace(def.OID) == "" {
			missing = append(missing, e.Label)
		}
	}
	if len(missing) == 0 {
		return "Perfil BNG guardado."
	}
	return "Perfil guardado. Métricas activas sem OID (não serão colectadas): " + strings.Join(missing, ", ")
}
