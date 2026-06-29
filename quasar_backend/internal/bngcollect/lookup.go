package bngcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// LookupSessionByLogin procura login via índice em cache (GET) ou walk da coluna username.
func LookupSessionByLogin(ctx context.Context, host, community, login string, profile Profile, timeout time.Duration, hintIndex string) (SessionRow, bool, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return SessionRow{}, false, fmt.Errorf("login vazio")
	}
	stripSuffix := profile.Options.PPPoELoginStripSuffix
	profile = profileWithSessionWalksEnabled(profile)
	def := profile.Metrics["access_login"]
	baseOID := strings.TrimSpace(def.OID)
	if baseOID == "" {
		baseOID = catalogPlaceholder("access_login")
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}

	targets := PPPoELoginLookupTargets(login, stripSuffix)
	displayLogin := NormalizePPPoELogin(login, stripSuffix)

	matchFn := func(val string) bool {
		for _, t := range targets {
			if MatchPPPoELogin(t, val, stripSuffix) {
				return true
			}
		}
		return false
	}

	if hint := strings.TrimSpace(hintIndex); hint != "" {
		if row, ok := lookupSessionByIndex(ctx, host, community, profile, hint, displayLogin, matchFn, timeout); ok {
			return row, true, nil
		}
	}

	var idx string
	matched, ok, walkErr := probing.SNMPWalkUntil(ctx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: baseOID, Version: "2c",
		Timeout: timeout, MaxRows: maxSessionWalkRows,
	}, func(v probing.SNMPVar) bool {
		if !matchFn(strings.TrimSpace(v.Value)) {
			return false
		}
		candidate := extractIndexFromOID(v.OID, baseOID)
		if candidate == "" {
			return false
		}
		idx = candidate
		return true
	})
	if walkErr != "" {
		return SessionRow{}, false, fmt.Errorf("%s", walkErr)
	}
	if !ok || idx == "" {
		_ = matched
		return SessionRow{Status: "Down", Login: displayLogin}, false, nil
	}

	columnMaps := map[string]map[string]string{
		"access_login": {idx: displayLogin},
	}
	for key, m := range FetchSessionDetailMaps(ctx, host, community, profile, idx, 12*time.Second) {
		columnMaps[key] = m
	}

	merged := mergeSessionMaps(columnMaps, true)
	if len(merged) > 0 {
		out := ApplyLoginStripToSessions(merged, stripSuffix)
		return out[0], true, nil
	}
	return SessionRow{Index: idx, Login: displayLogin, Status: "Up"}, true, nil
}

func lookupSessionByIndex(ctx context.Context, host, community string, profile Profile, idx, displayLogin string, matchFn func(string) bool, timeout time.Duration) (SessionRow, bool) {
	loginBase := metricBaseOID(profile, "access_login")
	vars, errMsg := probing.SNMPGetMany(ctx, host, community, "2c", 8*time.Second, 1, []string{loginBase + "." + idx}, 1)
	if errMsg != "" && len(vars) == 0 {
		return SessionRow{}, false
	}
	loginVal := ""
	for _, v := range vars {
		if strings.TrimSpace(v.Value) != "" {
			loginVal = strings.TrimSpace(v.Value)
			break
		}
	}
	if loginVal == "" || !matchFn(loginVal) {
		return SessionRow{}, false
	}

	columnMaps := map[string]map[string]string{
		"access_login": {idx: NormalizeSNMPLoginValue(loginVal, profile.Options.PPPoELoginStripSuffix)},
	}
	for key, m := range FetchSessionDetailMaps(ctx, host, community, profile, idx, 12*time.Second) {
		columnMaps[key] = m
	}
	merged := mergeSessionMaps(columnMaps, true)
	if len(merged) == 0 {
		return SessionRow{Index: idx, Login: displayLogin, Status: "Up"}, true
	}
	out := ApplyLoginStripToSessions(merged, profile.Options.PPPoELoginStripSuffix)
	return out[0], true
}

