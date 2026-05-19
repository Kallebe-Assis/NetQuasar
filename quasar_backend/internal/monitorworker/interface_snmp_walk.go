package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

const (
	workerIFMibMaxRows     = 42_000
	workerIFXMibMaxRows    = 48_000
	workerMkOpticalMaxRows = 24_000
	workerMkIfStatsMaxRows = 16_000
	workerIfSensorsMaxRows = 2_000
)

func workerWalkShareTimeout(total time.Duration, frac float64, min, cap time.Duration) time.Duration {
	if total <= 0 {
		total = 120 * time.Second
	}
	w := time.Duration(float64(total) * frac)
	if w < min {
		return min
	}
	if w > cap {
		return cap
	}
	return w
}

// WorkerInterfaceWalkResult resultado de walks IF-MIB no worker (alinhado ao refresh manual da API).
type WorkerInterfaceWalkResult struct {
	Merged    []probing.SNMPVar
	Truncated bool
}

// collectWorkerInterfaceSNMPWalks walks IF-MIB (+ Mikrotik) com limites altos para equipamentos com muitas interfaces.
func collectWorkerInterfaceSNMPWalks(ctx context.Context, host, community string, total time.Duration, isMikrotik bool) WorkerInterfaceWalkResult {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" {
		return WorkerInterfaceWalkResult{}
	}
	walkIF, truncIF, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.2.2.1",
		Version: "2c", Timeout: workerWalkShareTimeout(total, 0.40, 15*time.Second, 100*time.Second),
		Retries: 0, MaxRows: workerIFMibMaxRows,
	})
	walkX, truncX, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.31.1.1.1",
		Version: "2c", Timeout: workerWalkShareTimeout(total, 0.35, 12*time.Second, 90*time.Second),
		Retries: 0, MaxRows: workerIFXMibMaxRows,
	})
	merged := append([]probing.SNMPVar{}, walkIF...)
	merged = append(merged, walkX...)
	trunc := truncIF || truncX
	if isMikrotik {
		walkMk, truncMk, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultOpticalWalkRoot,
			Version: "2c", Timeout: workerWalkShareTimeout(total, 0.12, 8*time.Second, 40*time.Second),
			Retries: 0, MaxRows: workerMkOpticalMaxRows,
		})
		walkMkIf, truncMkIf, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultInterfaceStatsNameWalkRoot,
			Version: "2c", Timeout: workerWalkShareTimeout(total, 0.10, 6*time.Second, 35*time.Second),
			Retries: 0, MaxRows: workerMkIfStatsMaxRows,
		})
		merged = append(merged, walkMk...)
		merged = append(merged, walkMkIf...)
		trunc = trunc || truncMk || truncMkIf
	}
	walkSen, truncSen, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.99.1.1.1.4",
		Version: "2c", Timeout: workerWalkShareTimeout(total, 0.08, 5*time.Second, 20*time.Second),
		Retries: 0, MaxRows: workerIfSensorsMaxRows,
	})
	return WorkerInterfaceWalkResult{Merged: append(merged, walkSen...), Truncated: trunc || truncSen}
}
