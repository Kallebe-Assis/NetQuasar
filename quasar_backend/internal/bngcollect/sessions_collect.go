package bngcollect

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const (
	sessionDetailBatchSize = 50
	sessionGetOIDBatch     = 40
)

// SessionDetailGetKeys colunas obtidas por GET após walk de logins (exclui access_login).
func SessionDetailGetKeys() []string {
	keys := make([]string, 0, len(SessionWalkKeys()))
	for _, key := range SessionWalkKeys() {
		if key == "access_login" {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// FetchSessionDetailMaps obtém colunas de sessão por índice hwAccess (GET SNMP).
func FetchSessionDetailMaps(ctx context.Context, host, community string, profile Profile, idx string, timeout time.Duration) map[string]map[string]string {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	idx = strings.TrimSpace(idx)
	out := make(map[string]map[string]string)
	if host == "" || community == "" || idx == "" {
		return out
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}

	getKeys := SessionDetailGetKeys()
	baseByKey := make(map[string]string, len(getKeys))
	var oids []string
	keyByOID := make(map[string]string, len(getKeys)*2)

	registerOID := func(full, key string) {
		full = strings.TrimSpace(full)
		if full == "" {
			return
		}
		oids = append(oids, full)
		keyByOID[full] = key
		keyByOID[probing.NormalizeSNMPOID(full)] = key
	}

	for _, key := range getKeys {
		base := metricBaseOID(profile, key)
		baseByKey[key] = base
		registerOID(base+"."+idx, key)
	}

	vars, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, oids, sessionGetOIDBatch)
	suffix := "." + idx
	for _, v := range vars {
		key := keyByOID[v.OID]
		if key == "" {
			key = keyByOID[probing.NormalizeSNMPOID(v.OID)]
		}
		if key == "" {
			oid := probing.NormalizeSNMPOID(v.OID)
			if strings.HasSuffix(oid, suffix) {
				base := strings.TrimSuffix(oid, suffix)
				for k, b := range baseByKey {
					if probing.NormalizeSNMPOID(b) == base {
						key = k
						break
					}
				}
			}
		}
		if key == "" {
			continue
		}
		if out[key] == nil {
			out[key] = make(map[string]string)
		}
		out[key][idx] = strings.TrimSpace(v.Value)
	}
	return out
}

func metricBaseOID(profile Profile, key string) string {
	def := profile.Metrics[key]
	base := strings.TrimSpace(def.OID)
	if base == "" {
		base = catalogPlaceholder(key)
	}
	return base
}

type oidRef struct {
	key string
	idx string
}

// collectSessionsByIndex: 1× walk de logins + GET por índice (rápido em ~3000 sessões).
func collectSessionsByIndex(ctx context.Context, host, community string, profile Profile, timeout time.Duration, report *CollectProgressReporter) (CollectOutput, []SessionRow) {
	out := CollectOutput{
		Fields: make(map[string]FieldResult),
		Status: CollectionStatus{Enabled: 1},
	}
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" {
		out.Status.Message = "host ou community SNMP em falta"
		return out, nil
	}

	walkTimeout := timeout
	if walkTimeout <= 0 {
		walkTimeout = 300 * time.Second
	}
	_ = walkTimeout // prazo total via ctx; Timeout por pedido SNMP = 30s
	getTimeout := 45 * time.Second

	loginBase := metricBaseOID(profile, "access_login")
	if report != nil && report.OnPhase != nil {
		report.OnPhase("access_login", "A carregar logins PPPoE (walk)…")
	}

	var onProgress func(int)
	if report != nil && report.OnLoginsLoaded != nil {
		onProgress = func(n int) {
			if n <= 50 || n%50 == 0 || n%500 == 0 {
				report.OnLoginsLoaded(n)
			}
		}
	}

	loginVars, truncated, walkErr := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: loginBase, Version: "2c",
		Timeout: 30 * time.Second, MaxRows: maxSessionWalkRows,
		OnProgress: onProgress,
	})
	loginFR := FieldResult{
		Key: "access_login", Label: "Login", CollectMode: ModeSNMPWalk, OID: loginBase,
		RowCount: len(loginVars), Truncated: truncated,
	}
	if walkErr != "" {
		loginFR.Error = walkErr
		out.Status.Failed++
		out.Fields["access_login"] = loginFR
		out.Status.Message = walkErr
		return out, nil
	}
	loginFR.OK = true
	loginFR.Value = loginVars
	out.Fields["access_login"] = loginFR
	out.Status.Collected++

	stripSuffix := profile.Options.PPPoELoginStripSuffix
	idxToLogin := make(map[string]string, len(loginVars))
	for _, v := range loginVars {
		idx := extractIndexFromOID(v.OID, loginBase)
		if idx == "" {
			continue
		}
		raw := strings.TrimSpace(v.Value)
		idxToLogin[idx] = NormalizeSNMPLoginValue(raw, stripSuffix)
		if idxToLogin[idx] == "" {
			idxToLogin[idx] = raw
		}
	}

	total := len(idxToLogin)
	if report != nil && report.OnLoginsLoaded != nil && total > 0 {
		report.OnLoginsLoaded(total)
	}
	if total == 0 {
		return out, nil
	}

	indices := make([]string, 0, total)
	for idx := range idxToLogin {
		indices = append(indices, idx)
	}
	sort.Strings(indices)

	columnMaps := map[string]map[string]string{
		"access_login": idxToLogin,
	}
	detailKeys := SessionDetailGetKeys()

	if report != nil && report.OnPhase != nil {
		report.OnPhase("details", fmt.Sprintf("A obter detalhes de %d sessões por índice…", total))
	}

	enriched := 0
	for start := 0; start < len(indices); start += sessionDetailBatchSize {
		if ctx.Err() != nil {
			out.Status.Message = ctx.Err().Error()
			break
		}
		end := start + sessionDetailBatchSize
		if end > len(indices) {
			end = len(indices)
		}
		batchIdx := indices[start:end]

		var oids []string
		keyByOID := make(map[string]oidRef, len(batchIdx)*len(detailKeys))
		for _, idx := range batchIdx {
			for _, key := range detailKeys {
				base := metricBaseOID(profile, key)
				full := base + "." + idx
				oids = append(oids, full)
				keyByOID[full] = oidRef{key: key, idx: idx}
			}
		}

		vars, getErr := probing.SNMPGetMany(ctx, host, community, "2c", getTimeout, 1, oids, sessionGetOIDBatch)
		for _, v := range vars {
			ref, ok := keyByOID[v.OID]
			if !ok {
				ref, ok = keyByOID[probing.NormalizeSNMPOID(v.OID)]
			}
			if !ok {
				continue
			}
			if columnMaps[ref.key] == nil {
				columnMaps[ref.key] = make(map[string]string)
			}
			columnMaps[ref.key][ref.idx] = strings.TrimSpace(v.Value)
		}
		if getErr != "" && len(vars) == 0 {
			out.Status.Failed++
			out.Status.Message = getErr
		}

		enriched += len(batchIdx)
		if report != nil && report.OnSessionsLoaded != nil {
			if enriched == len(batchIdx) || enriched%500 == 0 || enriched == total {
				report.OnSessionsLoaded(enriched, total)
			}
		}
	}

	sessions := mergeSessionMaps(columnMaps)
	sessions = ApplyLoginStripToSessions(sessions, stripSuffix)
	out.Status.Collected += len(detailKeys)
	return out, sessions
}
