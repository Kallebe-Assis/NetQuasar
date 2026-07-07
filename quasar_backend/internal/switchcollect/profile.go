package switchcollect

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

// Profile perfil global de coleta Switch (mesma estrutura SNMP que MikroTik).
type Profile = mikrotikcollect.Profile

// LoadGlobalProfile carrega perfil da BD (settings_switch_collection).
func LoadGlobalProfile(ctx context.Context, pool *pgxpool.Pool) Profile {
	var metricsRaw, stepsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT coalesce(metrics::text, '{}'), coalesce(collection_steps::text, '[]')
		FROM settings_switch_collection WHERE id = 1
	`).Scan(&metricsRaw, &stepsRaw)
	p := Profile{
		Metrics:         DefaultSwitchMetrics(),
		CollectionSteps: mikrotikcollect.ParseSteps(stepsRaw),
		Catalog:         SwitchMetricCatalog,
	}
	if err != nil {
		return p
	}
	if parsed := mikrotikcollect.ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithCatalog(SwitchMetricCatalog, DefaultSwitchMetrics())
	}
	return p
}

// IsSwitchDevice identifica equipamentos da categoria Switch.
func IsSwitchDevice(category, brand, model, description string) bool {
	return strings.EqualFold(strings.TrimSpace(category), "switch")
}
