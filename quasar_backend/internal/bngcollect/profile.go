package bngcollect

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile perfil global de coleta BNG.
type Profile struct {
	Metrics MetricsConfig `json:"metrics"`
}

// LoadGlobalProfile carrega perfil da BD (id=1) com merge de defaults.
func LoadGlobalProfile(ctx context.Context, pool *pgxpool.Pool) Profile {
	var metricsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT coalesce(metrics::text, '{}')
		FROM settings_bng_collection WHERE id = 1
	`).Scan(&metricsRaw)
	p := Profile{Metrics: DefaultMetrics()}
	if err != nil {
		return p
	}
	if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	return p
}
