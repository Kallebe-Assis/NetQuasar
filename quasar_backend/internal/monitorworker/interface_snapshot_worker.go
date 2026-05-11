package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
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
	walkIF, _, noteIF := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: "1.3.6.1.2.1.2.2.1",
		Version: "2c", Timeout: 34 * time.Second, Retries: 0, MaxRows: 14000,
	})
	walkX, _, noteX := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: "1.3.6.1.2.1.31.1.1.1",
		Version: "2c", Timeout: 28 * time.Second, Retries: 0, MaxRows: 20000,
	})
	var merged []probing.SNMPVar
	merged = append(merged, walkIF...)
	merged = append(merged, walkX...)
	if workerLikelyMikrotik(cat, brand, model, description) {
		walkMk, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: h, Port: 161, Community: c, RootOID: snmpmikrotik.DefaultOpticalWalkRoot,
			Version: "2c", Timeout: 20 * time.Second, Retries: 0, MaxRows: 12000,
		})
		walkMkIf, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: h, Port: 161, Community: c, RootOID: snmpmikrotik.DefaultInterfaceStatsNameWalkRoot,
			Version: "2c", Timeout: 14 * time.Second, Retries: 0, MaxRows: 4000,
		})
		merged = append(merged, walkMk...)
		merged = append(merged, walkMkIf...)
	}
	walkSen, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: h, Port: 161, Community: c, RootOID: "1.3.6.1.2.1.99.1.1.1.4",
		Version: "2c", Timeout: 14 * time.Second, Retries: 0, MaxRows: 800,
	})
	merged = append(merged, walkSen...)
	if len(merged) == 0 {
		if log != nil {
			log.Debug().Str("device", deviceID.String()).Str("host", h).Msg("interface snapshot worker: walk vazio")
		}
		return
	}
	arr := make([]map[string]any, 0, len(merged))
	for _, v := range merged {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
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
			log.Warn().Err(err).Str("device", deviceID.String()).Str("notes", strings.TrimSpace(noteIF+" "+noteX)).Msg("interface_snapshots insert (worker)")
		}
		return
	}
}
