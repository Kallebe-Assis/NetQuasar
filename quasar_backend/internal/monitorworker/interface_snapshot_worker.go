package monitorworker

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/ifaceoptical"
	"github.com/netquasar/netquasar/quasar_backend/internal/interfacealerts"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/rs/zerolog"
)

func workerLikelyMikrotik(category, brand, model, description string) bool {
	if strings.EqualFold(strings.TrimSpace(category), "switch") {
		return false
	}
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") || strings.Contains(hay, "chr")
}

func workerLikelySwitch(category string) bool {
	return strings.EqualFold(strings.TrimSpace(category), "switch")
}

// CollectInterfaceSnapshotWorker grava interface_snapshots (SNMP + Telnet óptico) e avalia alertas SFP / interface DOWN.
func CollectInterfaceSnapshotWorker(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, host, community string, cat, brand, model, description string) {
	if pool == nil || strings.TrimSpace(host) == "" || strings.TrimSpace(community) == "" {
		return
	}
	h := strings.TrimSpace(host)
	c := strings.TrimSpace(community)
	total := 120 * time.Second
	if cfg, err := loadClampMonitoringIntervals(ctx, pool); err == nil {
		total = cfg.interfaceTimeout(false, false)
	}

	var prevRaw, olderRaw []byte
	if rows, qerr := pool.Query(ctx, `
		SELECT interfaces FROM interface_snapshots
		WHERE device_id = $1
		ORDER BY collected_at DESC
		LIMIT 2
	`, deviceID); qerr == nil {
		n := 0
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				break
			}
			if n == 0 {
				prevRaw = raw
			} else {
				olderRaw = raw
			}
			n++
		}
		rows.Close()
	}

	isSwitch := workerLikelySwitch(cat)
	isMk := workerLikelyMikrotik(cat, brand, model, description)
	walkRes := collectWorkerInterfaceSNMPWalks(ctx, h, c, total, isMk || isSwitch, pool, isSwitch)
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
	if isMk || isSwitch {
		telnetTO := 60 * time.Second
		if total > 0 && total/3 < telnetTO {
			telnetTO = total / 3
			if telnetTO < 20*time.Second {
				telnetTO = 20 * time.Second
			}
		}
		arr = ifaceoptical.EnrichSnapshotArray(ctx, pool, deviceID, h, isSwitch, arr, telnetTO)
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

	if isMk {
		// Aba PPPoE lê da telemetria: manter alinhada com este walk de interfaces.
		if _, errPPP := mikrotikcollect.SyncPPPoEFromInterfaceWalk(ctx, pool, deviceID, walkRes.Merged); errPPP != nil && log != nil {
			log.Warn().Err(errPPP).Str("device", deviceID.String()).Msg("sync PPPoE no snapshot de interfaces")
		}
	}

	interfacealerts.EvaluateAfterSnapshot(ctx, pool, log, interfacealerts.Params{
		DeviceID:   deviceID,
		Host:       h,
		Community:  c,
		DeviceDesc: description,
		Category:   cat,
		Brand:      brand,
		Model:      model,
		Source:     "monitor_worker",
		PrevJSON:   prevRaw,
		OlderJSON:  olderRaw,
		CurrJSON:   currRaw,
	})
}
