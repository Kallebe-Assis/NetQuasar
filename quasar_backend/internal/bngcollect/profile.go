package bngcollect

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile perfil global de coleta BNG.
type Profile struct {
	Metrics MetricsConfig     `json:"metrics"`
	Options CollectionOptions `json:"options"`
}

// ParseCollectionOptions lê opções globais da coleta BNG.
func ParseCollectionOptions(raw []byte) CollectionOptions {
	var o CollectionOptions
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &o)
	}
	return o
}

// LoadGlobalProfile carrega perfil da BD (id=1) com merge de defaults.
func LoadGlobalProfile(ctx context.Context, pool *pgxpool.Pool) Profile {
	var metricsRaw, optionsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT coalesce(metrics::text, '{}'), coalesce(options::text, '{}')
		FROM settings_bng_collection WHERE id = 1
	`).Scan(&metricsRaw, &optionsRaw)
	p := Profile{Metrics: DefaultMetrics(), Options: ParseCollectionOptions(optionsRaw)}
	if err != nil {
		return p
	}
	if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	return p
}

// ProfileWithCollectMode restringe métricas activas conforme o modo do passo de monitoramento.
// Modos: totals, health, system, monitoring (sistema+saúde+totais), full.
func ProfileWithCollectMode(p Profile, mode string) Profile {
	mode = strings.ToLower(strings.TrimSpace(mode))
	allowed := sectionsForCollectMode(mode)
	if allowed == nil {
		return p
	}
	p.Metrics = p.Metrics.MergeWithDefaults()
	for _, e := range MetricCatalog {
		def := p.Metrics[e.Key]
		if !allowed[e.Section] {
			def.Enabled = false
		} else if mode == "monitoring" {
			// Linha-base BNG: tentar todos os escalares configurados de sistema,
			// saúde e totais uma vez por ciclo. Walks de interfaces/sessões ficam fora.
			def.Enabled = true
		}
		p.Metrics[e.Key] = def
	}
	return p
}

func sectionsForCollectMode(mode string) map[string]bool {
	switch mode {
	case "monitoring":
		return map[string]bool{"system": true, "health": true, "subscribers": true}
	case "totals", "subscribers":
		return map[string]bool{"subscribers": true}
	case "health":
		return map[string]bool{"health": true, "system": true}
	case "system":
		return map[string]bool{"system": true}
	default:
		return nil
	}
}
