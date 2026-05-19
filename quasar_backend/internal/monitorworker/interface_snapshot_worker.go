package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func workerLikelyMikrotik(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") || strings.Contains(hay, "chr")
}

// CollectInterfaceSnapshotWorker grava uma linha em interface_snapshots (walk IF+X, opcionalmente ópticas Mikrotik).
func CollectInterfaceSnapshotWorker(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, host, community string, cat, brand, model, description string) {
	if pool == nil || strings.TrimSpace(host) == "" || strings.TrimSpace(community) == "" {
		return
	}
	h := strings.TrimSpace(host)
	c := strings.TrimSpace(community)
	total := 120 * time.Second
	if cfg, err := loadClampMonitoringIntervals(ctx, pool); err == nil {
		total = cfg.interfaceTimeout(false)
	}
	walkRes := collectWorkerInterfaceSNMPWalks(ctx, h, c, total, workerLikelyMikrotik(cat, brand, model, description))
	if len(walkRes.Merged) == 0 {
		if log != nil {
			log.Debug().Str("device", deviceID.String()).Str("host", h).Msg("interface snapshot worker: walk vazio")
		}
		return
	}
	arr := make([]map[string]any, 0, len(walkRes.Merged)+1)
	for _, v := range walkRes.Merged {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	if walkRes.Truncated {
		arr = append(arr, map[string]any{"oid": "__netquasar.walk", "value": "truncated", "type": "meta"})
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO interface_snapshots (device_id, interfaces) VALUES ($1, $2::jsonb)
	`, deviceID, b)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Str("device", deviceID.String()).Msg("interface_snapshots insert (worker)")
		}
		return
	}
}
