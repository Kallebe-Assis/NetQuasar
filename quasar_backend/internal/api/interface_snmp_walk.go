package api

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

// Limites de linhas SNMP para walks de interface (IF-MIB ~22 colunas × N interfaces).
const (
	snmpIFMibMaxRows     = 42_000
	snmpIFXMibMaxRows    = 48_000
	snmpMkOpticalMaxRows = 24_000
	snmpMkIfStatsMaxRows = 16_000
	snmpIfSensorsMaxRows = 2_000
)

type interfaceSNMPWalkResult struct {
	Merged    []probing.SNMPVar
	Truncated bool
	Note      string
}

func walkShareTimeout(total time.Duration, frac float64, min, cap time.Duration) time.Duration {
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

// collectInterfaceSNMPWalks executa walks IF-MIB (+ Mikrotik opcional) com limites altos para centenas de interfaces.
func collectInterfaceSNMPWalks(ctx context.Context, pool *pgxpool.Pool, host, community string, total time.Duration, isMikrotik bool) interfaceSNMPWalkResult {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	out := interfaceSNMPWalkResult{}
	if host == "" || community == "" {
		return out
	}

	if isMikrotik && pool != nil {
		profile := mikrotikcollect.LoadGlobalProfile(ctx, pool)
		roots := mikrotikcollect.InterfaceWalkOIDs(profile.Metrics)
		if len(roots) > 0 {
			var merged []probing.SNMPVar
			trunc := false
			var notes []string
			perRoot := total / time.Duration(len(roots)+1)
			if perRoot < 10*time.Second {
				perRoot = 10 * time.Second
			}
			for _, root := range roots {
				walk, t, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
					Host: host, Port: 161, Community: community, RootOID: root,
					Version: "2c", Timeout: perRoot, Retries: 0, MaxRows: snmpIFMibMaxRows,
				})
				merged = append(merged, walk...)
				trunc = trunc || t
				if note != "" {
					notes = append(notes, note)
				}
			}
			walkSen, truncSen, noteSen := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.99.1.1.1.4",
				Version: "2c", Timeout: walkShareTimeout(total, 0.08, 6*time.Second, 25*time.Second),
				Retries: 0, MaxRows: snmpIfSensorsMaxRows,
			})
			out.Merged = append(merged, walkSen...)
			out.Truncated = trunc || truncSen
			out.Note = strings.TrimSpace(strings.Join(append(notes, noteSen), " "))
			if out.Truncated {
				out.Note = strings.TrimSpace(out.Note + " (walk truncado — aumente interface_snapshot_timeout_ms nas configurações)")
			}
			return out
		}
	}

	walkIF, truncIF, noteIF := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.2.2.1",
		Version: "2c", Timeout: walkShareTimeout(total, 0.36, 15*time.Second, 100*time.Second),
		Retries: 0, MaxRows: snmpIFMibMaxRows,
	})
	walkX, truncX, noteX := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.31.1.1.1",
		Version: "2c", Timeout: walkShareTimeout(total, 0.30, 12*time.Second, 90*time.Second),
		Retries: 0, MaxRows: snmpIFXMibMaxRows,
	})
	var walkMk, walkMkIf []probing.SNMPVar
	var truncMk, truncMkIf bool
	var noteMk, noteMkIf string
	if isMikrotik {
		walkMk, truncMk, noteMk = probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultOpticalWalkRoot,
			Version: "2c", Timeout: walkShareTimeout(total, 0.14, 10*time.Second, 45*time.Second),
			Retries: 0, MaxRows: snmpMkOpticalMaxRows,
		})
		walkMkIf, truncMkIf, noteMkIf = probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: snmpmikrotik.DefaultInterfaceStatsNameWalkRoot,
			Version: "2c", Timeout: walkShareTimeout(total, 0.12, 8*time.Second, 40*time.Second),
			Retries: 0, MaxRows: snmpMkIfStatsMaxRows,
		})
	}
	walkSen, truncSen, noteSen := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: "1.3.6.1.2.1.99.1.1.1.4",
		Version: "2c", Timeout: walkShareTimeout(total, 0.08, 6*time.Second, 25*time.Second),
		Retries: 0, MaxRows: snmpIfSensorsMaxRows,
	})

	out.Merged = append(append(append(append(append([]probing.SNMPVar{}, walkIF...), walkX...), walkMk...), walkMkIf...), walkSen...)
	out.Truncated = truncIF || truncX || truncMk || truncMkIf || truncSen
	out.Note = strings.TrimSpace(strings.Join([]string{noteIF, noteX, noteMk, noteMkIf, noteSen}, " "))
	if out.Truncated {
		out.Note = strings.TrimSpace(out.Note + " (walk truncado — aumente interface_snapshot_timeout_ms nas configurações)")
	}
	return out
}
