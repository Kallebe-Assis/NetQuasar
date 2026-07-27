package mikrotikcollect

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

const pppoeSessionsMetricKey = "pppoe_active_sessions"

func uint64FromInt64(v int64) uint64 {
	if v <= 0 {
		return 0
	}
	return uint64(v)
}

// PPPoESessionsFromIfTable deriva sessões PPPoE de um walk de interfaces já feito,
// aceitando o nome vindo de ifDescr (IF-MIB) ou ifName (IF-X-MIB).
func PPPoESessionsFromIfTable(vars []probing.SNMPVar) []PPPoESessionRow {
	rows := snmpifparse.BuildIfTable(vars)
	out := make([]PPPoESessionRow, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.DisplayName)
		if name == "" {
			name = strings.TrimSpace(r.Descr)
		}
		if name == "" || !isPPPoEInterfaceName(name) {
			continue
		}
		out = append(out, PPPoESessionRow{
			IfIndex:         r.IfIndex,
			Name:            name,
			OperStatus:      r.OperStatus,
			OperStatusLabel: ifOperStatusLabel(r.OperStatus),
			InOctets:        uint64FromInt64(r.InOctets),
			OutOctets:       uint64FromInt64(r.OutOctets),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ifTableWalkHasNames indica se o walk trouxe nomes de interface (ifDescr/ifName).
// Sem nomes não é possível concluir «zero sessões PPPoE».
func ifTableWalkHasNames(vars []probing.SNMPVar) bool {
	for _, v := range vars {
		oid := strings.TrimPrefix(strings.TrimSpace(v.OID), ".")
		if strings.HasPrefix(oid, "1.3.6.1.2.1.2.2.1.2.") || strings.HasPrefix(oid, "1.3.6.1.2.1.31.1.1.1.1.") {
			return true
		}
	}
	return false
}

// SyncPPPoEFromInterfaceWalk actualiza o campo pppoe_active_sessions da telemetria mais
// recente a partir de um walk de interfaces (refresh manual ou ciclo de monitoramento),
// para a aba PPPoE acompanhar a coleta de interfaces sem SNMP extra. Só o campo PPPoE é
// reescrito — CPU, memória e restantes métricas da amostra ficam intactas.
func SyncPPPoEFromInterfaceWalk(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, vars []probing.SNMPVar) (int, error) {
	if pool == nil || len(vars) == 0 {
		return 0, nil
	}
	profile := LoadGlobalProfile(ctx, pool)
	if def, ok := profile.Metrics[pppoeSessionsMetricKey]; ok && !def.Enabled {
		return 0, nil
	}
	sessions := PPPoESessionsFromIfTable(vars)
	if len(sessions) == 0 && !ifTableWalkHasNames(vars) {
		return 0, nil
	}
	label := "Sessões PPPoE activas (IF-MIB)"
	for _, e := range profile.CatalogEntries() {
		if e.Key == pppoeSessionsMetricKey && strings.TrimSpace(e.Label) != "" {
			label = e.Label
			break
		}
	}
	b, err := json.Marshal(FieldResult{
		Key:           pppoeSessionsMetricKey,
		Label:         label,
		OK:            true,
		CollectMode:   ModeIFMibPPPoE,
		OID:           IFTableBaseOID,
		Value:         len(sessions),
		PPPoESessions: sessions,
	})
	if err != nil {
		return 0, err
	}
	tag, err := pool.Exec(ctx, `
		UPDATE telemetry_samples SET metrics =
			coalesce(metrics, '{}'::jsonb) || jsonb_build_object(
				'mikrotik_collection',
				coalesce(metrics->'mikrotik_collection', '{}'::jsonb) || jsonb_build_object(
					'fields',
					coalesce(metrics->'mikrotik_collection'->'fields', '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb)
				)
			)
		WHERE id = (
			SELECT id FROM telemetry_samples WHERE device_id = $1 ORDER BY collected_at DESC LIMIT 1
		)
	`, deviceID, pppoeSessionsMetricKey, b)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		// Equipamento sem telemetria persistida: cria amostra só com o campo PPPoE.
		_, err = pool.Exec(ctx, `
			INSERT INTO telemetry_samples (device_id, collected_at, metrics)
			VALUES ($1, now(), jsonb_build_object(
				'mikrotik_collection', jsonb_build_object('fields', jsonb_build_object($2::text, $3::jsonb))
			))
		`, deviceID, pppoeSessionsMetricKey, b)
		if err != nil {
			return 0, err
		}
	}
	return len(sessions), nil
}
