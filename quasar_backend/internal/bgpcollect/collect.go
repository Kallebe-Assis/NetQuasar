package bgpcollect

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// FieldResult resultado de uma métrica (GET escalar ou WALK de tabela).
type FieldResult struct {
	Key         string `json:"key"`
	Section     string `json:"section"`
	OK          bool   `json:"ok"`
	CollectMode string `json:"collect_mode"`
	OID         string `json:"oid"`
	Value       any    `json:"value,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CollectOutput resultado de uma coleta BGP (todas as métricas activas do perfil).
type CollectOutput struct {
	Fields         map[string]FieldResult `json:"fields"`
	CollectedCount int                    `json:"collected_count"`
	FailedCount    int                    `json:"failed_count"`
	Message        string                 `json:"message,omitempty"`
}

func catalogEntry(key string) (CatalogEntry, bool) {
	for _, e := range MetricCatalog {
		if e.Key == key {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// CollectPeriodic coleta as métricas activas do perfil contra host/community: GET para
// escalares, WALK para tabelas (interfaces/peers) — mesmo motor SNMP (internal/probing) já
// usado por bngcollect/mikrotikcollect, sem reimplementar transporte.
func CollectPeriodic(ctx context.Context, host, community string, profile Profile, timeout time.Duration) CollectOutput {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	out := CollectOutput{Fields: make(map[string]FieldResult)}
	if host == "" || community == "" {
		out.Message = "host ou community SNMP em falta"
		return out
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	var getOIDs []string
	getKeyByOID := make(map[string]string)
	for _, e := range MetricCatalog {
		def := profile.Metrics[e.Key]
		if !def.Enabled || strings.TrimSpace(def.OID) == "" {
			continue
		}
		if def.CollectMode == ModeSNMPWalk {
			vars, _, errMsg := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: host, Port: 161, Community: community, RootOID: def.OID,
				Version: "2c", Timeout: timeout, MaxRows: 2000,
			})
			fr := FieldResult{Key: e.Key, Section: e.Section, CollectMode: def.CollectMode, OID: def.OID}
			if len(vars) == 0 {
				fr.Error = errMsg
				if fr.Error == "" {
					fr.Error = "sem resultados"
				}
				out.FailedCount++
			} else {
				fr.OK = true
				fr.Value = vars
				out.CollectedCount++
			}
			out.Fields[e.Key] = fr
			continue
		}
		getOIDs = append(getOIDs, def.OID)
		getKeyByOID[def.OID] = e.Key
	}

	if len(getOIDs) > 0 {
		vars, errMsg := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, getOIDs, 20)
		gotByOID := make(map[string]string)
		for oid, key := range getKeyByOID {
			gotByOID[probing.NormalizeSNMPOID(oid)] = key
		}
		gotVal := make(map[string]string)
		for _, v := range vars {
			if key, ok := getKeyByOID[v.OID]; ok {
				gotVal[key] = v.Value
				continue
			}
			if key, ok := gotByOID[probing.NormalizeSNMPOID(v.OID)]; ok {
				gotVal[key] = v.Value
			}
		}
		for oid, key := range getKeyByOID {
			e, _ := catalogEntry(key)
			fr := FieldResult{Key: key, Section: e.Section, CollectMode: ModeSNMPGet, OID: oid}
			if v, ok := gotVal[key]; ok && probing.SNMPValueUsable(v) {
				fr.OK = true
				fr.Value = v
				out.CollectedCount++
			} else {
				fr.Error = errMsg
				if fr.Error == "" {
					fr.Error = "sem resposta"
				}
				out.FailedCount++
			}
			out.Fields[key] = fr
		}
	}
	if out.CollectedCount == 0 && out.FailedCount > 0 && out.Message == "" {
		out.Message = "nenhuma métrica respondeu"
	}
	return out
}

// CollectAndStoreForDevice coleta com o perfil dado e grava em telemetry_samples — mesmo
// destino/formato que bngcollect.CollectAndStorePeriodicMode já usa para BNG, para reaproveitar
// qualquer leitura futura de telemetria genérica por equipamento.
func CollectAndStoreForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration, profile Profile) (CollectOutput, error) {
	out := CollectPeriodic(ctx, host, community, profile, timeout)
	metrics := map[string]any{
		"bgp_collection": out,
		"profile_id":     profile.ID,
		"profile_name":   profile.Name,
	}
	b, err := json.Marshal(metrics)
	if err != nil {
		return out, err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO telemetry_samples (device_id, collected_at, metrics) VALUES ($1, now(), $2::jsonb)
	`, deviceID, b)
	return out, err
}
