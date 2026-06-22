package api

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

func loadOltCollectionProfile(ctx context.Context, pool *pgxpool.Pool, brand, model string) (oltcollect.Profile, error) {
	return oltcollect.LoadVendorProfile(ctx, pool, brand, model)
}

func collectionStepsJSON(steps []oltcollect.Step) json.RawMessage {
	if len(steps) == 0 {
		return json.RawMessage("[]")
	}
	b, _ := json.Marshal(steps)
	return b
}

func onuMetricsJSON(m oltcollect.OnuMetricsConfig) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	b, _ := json.Marshal(m)
	return b
}

func onuReportCommandsJSON(c oltcollect.OnuReportConfig) json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}

func ponTelnetCommandsJSON(c oltcollect.PonTelnetConfig) json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}
