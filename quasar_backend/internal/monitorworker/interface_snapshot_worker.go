package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/interfacealerts"
	"github.com/rs/zerolog"
)

func workerLikelyMikrotik(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") || strings.Contains(hay, "chr")
}

// CollectInterfaceSnapshotWorker grava interface_snapshots e avalia alertas SFP / interface DOWN.
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

	var prevRaw []byte
	_ = pool.QueryRow(ctx, `
		SELECT interfaces FROM interface_snapshots
		WHERE device_id = $1
		ORDER BY collected_at DESC
		LIMIT 1
	`, deviceID).Scan(&prevRaw)

	walkRes := collectWorkerInterfaceSNMPWalks(ctx, h, c, total, workerLikelyMikrotik(cat, brand, model, description), pool)
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
	currRaw, err := json.Marshal(arr)
	if err != nil {
		return
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO interface_snapshots (device_id, interfaces) VALUES ($1, $2::jsonb)
	`, deviceID, currRaw)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Str("device", deviceID.String()).Msg("interface_snapshots insert (worker)")
		}
		return
	}

	interfacealerts.EvaluateAfterSnapshot(ctx, pool, log, interfacealerts.Params{
		DeviceID:   deviceID,
		Host:       h,
		DeviceDesc: description,
		Category:   cat,
		Brand:      brand,
		Model:      model,
		Source:     "monitor_worker",
		PrevJSON:   prevRaw,
		CurrJSON:   currRaw,
	})
}
