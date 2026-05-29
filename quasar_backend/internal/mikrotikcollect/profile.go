package mikrotikcollect

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile perfil global de coleta MikroTik.
type Profile struct {
	Metrics         MetricsConfig `json:"metrics"`
	CollectionSteps []Step        `json:"collection_steps"`
}

// Step passo extra de coleta (walk/get manual).
type Step struct {
	ID          string         `json:"id,omitempty"`
	Method      string         `json:"method"`
	Enabled     *bool          `json:"enabled,omitempty"`
	OID         string         `json:"oid,omitempty"`
	StoreAs     string         `json:"store_as,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

func (s Step) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

const (
	MethodSNMPWalk = "snmp_walk"
	MethodSNMPGet  = "snmp_get"
)

// LoadGlobalProfile carrega perfil da BD (id=1) com merge de defaults.
func LoadGlobalProfile(ctx context.Context, pool *pgxpool.Pool) Profile {
	var metricsRaw, stepsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT coalesce(metrics::text, '{}'), coalesce(collection_steps::text, '[]')
		FROM settings_mikrotik_collection WHERE id = 1
	`).Scan(&metricsRaw, &stepsRaw)
	p := Profile{
		Metrics:         DefaultMetrics(),
		CollectionSteps: ParseSteps(stepsRaw),
	}
	if err != nil {
		return p
	}
	if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	return p
}

func ParseSteps(raw []byte) []Step {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return nil
	}
	var steps []Step
	if json.Unmarshal(raw, &steps) != nil {
		return nil
	}
	out := make([]Step, 0, len(steps))
	for _, s := range steps {
		s.Method = strings.TrimSpace(strings.ToLower(s.Method))
		if s.Method == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IsMikrotikDevice heurística de identificação MikroTik.
func IsMikrotikDevice(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") || strings.Contains(hay, "chr")
}
