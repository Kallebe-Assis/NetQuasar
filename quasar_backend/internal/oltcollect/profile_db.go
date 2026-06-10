package oltcollect

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadVendorProfile carrega o perfil de coleta OLT (brand/model) a partir da BD.
func LoadVendorProfile(ctx context.Context, pool *pgxpool.Pool, brand, model string) (Profile, error) {
	var p Profile
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
	p.Steps = ParseSteps(stepsRaw)
	p.OnuMetrics = ParseOnuMetrics(metricsRaw)
	return p, nil
}
