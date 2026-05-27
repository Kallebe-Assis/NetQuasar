package vsolparse

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// DefaultVSOLOnuWalkOID raiz gOnuAuthList (configurável no perfil OLT).
const DefaultVSOLOnuWalkOID = OIDGOnuAuthList

// WalkOnuTable faz um único snmpwalk na raiz OID e conta ONUs a partir da resposta.
func WalkOnuTable(ctx context.Context, host, community, rootOID string, budget time.Duration) (
	summary map[string]any, pons []map[string]any, vars []probing.SNMPVar, truncated bool, note string, err error,
) {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	rootOID = strings.TrimSpace(rootOID)
	if rootOID == "" {
		rootOID = DefaultVSOLOnuWalkOID
	}
	if host == "" || community == "" {
		return nil, nil, nil, false, "", fmt.Errorf("host ou community SNMP em falta")
	}
	walkTO := budget
	if walkTO <= 0 {
		walkTO = 90 * time.Second
	}
	if walkTO > 98*time.Second {
		walkTO = 98 * time.Second
	}

	t0 := time.Now()
	walkVars, truncated, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: rootOID,
		Version: "2c", Timeout: walkTO, Retries: 1, MaxRows: snmpWalkMaxRows,
	})
	vars = normalizeWalkVarsForBase(walkVars, rootOID)
	elapsed := time.Since(t0).Milliseconds()

	sum, _, onuRows := FromSNMPWalk(vars, truncated)
	if sum == nil {
		sum = map[string]any{}
	}
	sum["vsol_snmp_mode"] = "onu_snmp_walk"
	sum["vsol_walk_oid"] = rootOID
	sum["vsol_snmp_var_count"] = len(vars)
	sum["vsol_walk_elapsed_ms"] = elapsed
	sum["olt_collection_mode"] = "onu_snmp_walk"
	if note != "" {
		sum["vsol_get_note"] = note
	}
	if truncated {
		sum["vsol_walk_truncated"] = true
	}
	pons = PonsFromOnuRows(onuRows)
	return sum, pons, vars, truncated, note, nil
}

func normalizeWalkVarsForBase(vars []probing.SNMPVar, base string) []probing.SNMPVar {
	base = strings.TrimSuffix(strings.TrimSpace(probing.NormalizeSNMPOID(base)), ".")
	out := make([]probing.SNMPVar, 0, len(vars))
	for _, v := range vars {
		oid := probing.NormalizeSNMPOID(v.OID)
		if oid == "" {
			continue
		}
		if base != "" && !strings.HasPrefix(oid, base) {
			continue
		}
		if !probing.SNMPValueUsable(v.Value) {
			continue
		}
		out = append(out, probing.SNMPVar{OID: oid, Type: v.Type, Value: v.Value})
	}
	return out
}

// PonsFromOnuRows agrega contagens por porta PON a partir das linhas ONU.
func PonsFromOnuRows(onuRows []map[string]any) []map[string]any {
	type agg struct {
		total, online, offline int
	}
	byPon := map[int]*agg{}
	for _, r := range onuRows {
		pon := intValAny(r["pon"])
		if pon <= 0 {
			continue
		}
		a := byPon[pon]
		if a == nil {
			a = &agg{}
			byPon[pon] = a
		}
		a.total++
		if on, ok := r["online"].(bool); ok && on {
			a.online++
		} else {
			a.offline++
		}
	}
	keys := make([]int, 0, len(byPon))
	for p := range byPon {
		keys = append(keys, p)
	}
	sort.Ints(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, pon := range keys {
		a := byPon[pon]
		id := fmt.Sprintf("%02d", pon)
		out = append(out, map[string]any{
			"id": id, "name": "GPON0/" + id,
			"onu_total": a.total, "onu_online": a.online, "onu_offline": a.offline,
			"status": "vsol_snmp_walk", "source_slice": "onu_snmp_walk",
		})
	}
	return out
}

func intValAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
