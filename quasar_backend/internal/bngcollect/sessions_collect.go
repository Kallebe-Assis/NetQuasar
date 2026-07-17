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
	applyVar := func(v probing.SNMPVar) {
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
			return
		}
		val := strings.TrimSpace(v.Value)
		if !probing.SNMPValueUsable(val) {
			return
		}
		if out[key] == nil {
			out[key] = make(map[string]string)
		}
		out[key][idx] = val
	}
	for _, v := range vars {
		applyVar(v)
	}

	// GET misto pode omitir IPV6-TC; pedir WAN/PD isolados quando faltarem.
	var ipv6Retry []string
	for _, key := range []string{"access_ipv6", "access_ipv6_pd"} {
		if m := out[key]; m != nil && probing.SNMPValueUsable(m[idx]) {
			continue
		}
		base := baseByKey[key]
		if base == "" {
			continue
		}
		ipv6Retry = append(ipv6Retry, base+"."+idx)
	}
	if len(ipv6Retry) > 0 {
		retryVars, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, ipv6Retry, len(ipv6Retry))
		for _, v := range retryVars {
			applyVar(v)
		}
	}
	return out
}

// FetchSessionsByIndices obtém detalhes SNMP em tempo real para índices hwAccess específicos.
func FetchSessionsByIndices(ctx context.Context, host, community string, profile Profile, indices []string, timeout time.Duration) []SessionRow {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" || len(indices) == 0 {
		return nil
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	profile = profileWithSessionWalksEnabled(profile)
	stripSuffix := profile.Options.PPPoELoginStripSuffix
	out := make([]SessionRow, 0, len(indices))
	for _, idx := range indices {
		idx = strings.TrimSpace(idx)
		if idx == "" {
			continue
		}
		columnMaps := map[string]map[string]string{}
		loginBase := metricBaseOID(profile, "access_login")
		vars, _ := probing.SNMPGetMany(ctx, host, community, "2c", 8*time.Second, 1, []string{loginBase + "." + idx}, 1)
		loginVal := ""
		for _, v := range vars {
			if strings.TrimSpace(v.Value) != "" {
				loginVal = strings.TrimSpace(v.Value)
				break
			}
		}
		if loginVal != "" {
			columnMaps["access_login"] = map[string]string{idx: NormalizeSNMPLoginValue(loginVal, stripSuffix)}
		}
		for key, m := range FetchSessionDetailMaps(ctx, host, community, profile, idx, timeout) {
			columnMaps[key] = m
		}
		merged := mergeSessionMaps(columnMaps, true)
		if len(merged) > 0 {
			rows := ApplyLoginStripToSessions(merged, stripSuffix)
			out = append(out, rows[0])
			continue
		}
		if loginVal != "" {
			out = append(out, SessionRow{
				Index:  idx,
				Login:  NormalizeSNMPLoginValue(loginVal, stripSuffix),
				Status: "Up",
			})
		}
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

	// Huawei BNGs podem omitir colunas IPV6-TC em GETs grandes/mistos, embora o
	// snmpwalk da mesma coluna funcione. Recolher WAN/PD uma vez por consulta
	// completa garante IPv6 sem multiplicar pedidos por login.
	for _, key := range []string{"access_ipv6", "access_ipv6_pd"} {
		if ctx.Err() != nil {
			break
		}
		base := metricBaseOID(profile, key)
		vars, truncated, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Community: community, RootOID: base, Version: "2c",
			Timeout: 30 * time.Second, Retries: 1, MaxRows: maxSessionWalkRows,
			MaxRepetitions: 10,
		})
		matched := mergeSessionColumnWalk(columnMaps, key, base, vars, idxToLogin)
		out.Fields[key] = FieldResult{
			Key: key, Label: key, CollectMode: ModeSNMPWalk, OID: base,
			OK: matched > 0, RowCount: len(vars), Truncated: truncated, Error: note,
		}
		if matched > 0 {
			out.Status.Collected++
		} else if note != "" {
			out.Status.Failed++
		}
	}

	sessions := mergeSessionMaps(columnMaps)
	sessions = ApplyLoginStripToSessions(sessions, stripSuffix)
	out.Status.Collected += len(detailKeys)
	return out, sessions
}

func mergeSessionColumnWalk(
	columnMaps map[string]map[string]string,
	key, base string,
	vars []probing.SNMPVar,
	knownIndices map[string]string,
) int {
	if columnMaps[key] == nil {
		columnMaps[key] = make(map[string]string)
	}
	matched := 0
	for _, v := range vars {
		idx := extractIndexFromOID(v.OID, base)
		if idx == "" {
			continue
		}
		if len(knownIndices) > 0 {
			if _, ok := knownIndices[idx]; !ok {
				continue
			}
		}
		value := strings.TrimSpace(v.Value)
		if !probing.SNMPValueUsable(value) {
			continue
		}
		columnMaps[key][idx] = value
		matched++
	}
	return matched
}
