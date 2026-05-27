package api

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

func loadOltCollectionProfile(ctx context.Context, pool *pgxpool.Pool, brand, model string) (oltcollect.Profile, error) {
	var p oltcollect.Profile
	var stepsRaw, metricsRaw []byte
	var onu, pon, trx, base *string
	err := pool.QueryRow(ctx, `
		SELECT brand, model, onu_online_oid, pon_status_oid, transceiver_oid, snmp_base_oid,
			coalesce(collection_steps::text, '[]'),
			coalesce(onu_metrics::text, '{}')
		FROM olt_vendor_models
		WHERE upper(trim(brand)) = upper(trim($1)) AND upper(trim(model)) = upper(trim($2))
	`, brand, model).Scan(&p.Brand, &p.Model, &onu, &pon, &trx, &base, &stepsRaw, &metricsRaw)
	if err == pgx.ErrNoRows {
		return p, err
	}
	if err != nil {
		return p, err
	}
	if onu != nil {
		p.OnuOnlineOID = *onu
	}
	if pon != nil {
		p.PonStatusOID = *pon
	}
	if trx != nil {
		p.TransceiverOID = *trx
	}
	if base != nil {
		p.SNMPBaseOID = *base
	}
	p.Steps = oltcollect.ParseSteps(stepsRaw)
	p.OnuMetrics = oltcollect.ParseOnuMetrics(metricsRaw)
	return p, nil
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
