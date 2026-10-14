package bngcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// LookupOutcome é o resultado detalhado de uma procura de login no BNG.
type LookupOutcome struct {
	Row   SessionRow
	Found bool
	// RowsSeen é quantas linhas da coluna de login o walk percorreu até parar. Um «não encontrado» com
	// RowsSeen = 0 (ou muito abaixo do esperado) NÃO prova que o login está offline: o BNG pode ter devolvido
	// uma tabela vazia/parcial. Quem chama decide se a resposta é conclusiva.
	RowsSeen int
	// Loose indica que o login só foi achado pela comparação tolerante (sem domínio/@, sem espaços/controle).
	Loose bool
}

// looseLoginKey — chave de comparação tolerante: minúsculas, sem controle/espaços e sem «@domínio».
func looseLoginKey(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == ' ' {
			return -1
		}
		return r
	}, strings.ToLower(s))
	if i := strings.Index(s, "@"); i > 0 {
		s = s[:i]
	}
	return s
}

// ipv4AliasOfASCII4 — a leitura genérica de OCTET STRING mostra um login de exatamente 4 caracteres sem letras
// (ex.: «1234») como IPv4 («49.50.51.52»). Devolve essa forma para o login pesquisado, ou "".
func ipv4AliasOfASCII4(s string) string {
	if len(s) != 4 {
		return ""
	}
	for i := 0; i < 4; i++ {
		c := s[i]
		if c < 32 || c >= 127 || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return ""
		}
	}
	return fmt.Sprintf("%d.%d.%d.%d", s[0], s[1], s[2], s[3])
}

// LookupSessionByLoginDetailed é LookupSessionByLogin com os dados para decidir se um «não encontrado» é confiável.
func LookupSessionByLoginDetailed(ctx context.Context, host, community, login string, profile Profile, timeout time.Duration, hintIndex string) (LookupOutcome, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return LookupOutcome{}, fmt.Errorf("login vazio")
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
	if alias := ipv4AliasOfASCII4(displayLogin); alias != "" {
		targets = append(targets, alias)
	}
	looseWant := looseLoginKey(NormalizeSNMPLoginValue(login, stripSuffix))

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
			return LookupOutcome{Row: row, Found: true}, nil
		}
	}

	var idx, looseIdx string
	var matchedLogin, looseLogin string
	rows := 0
	_, ok, walkErr := probing.SNMPWalkUntil(ctx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: baseOID, Version: "2c",
		Timeout: timeout, MaxRows: maxSessionWalkRows,
	}, func(v probing.SNMPVar) bool {
		rows++
		val := strings.TrimSpace(v.Value)
		if !matchFn(val) {
			if looseIdx == "" && looseWant != "" && looseLoginKey(NormalizeSNMPLoginValue(val, stripSuffix)) == looseWant {
				if c := extractIndexFromOID(v.OID, baseOID); c != "" {
					looseIdx, looseLogin = c, NormalizeSNMPLoginValue(val, stripSuffix)
				}
			}
			return false
		}
		candidate := extractIndexFromOID(v.OID, baseOID)
		if candidate == "" {
			return false
		}
		idx = candidate
		matchedLogin = NormalizeSNMPLoginValue(val, stripSuffix)
		return true
	})
	if walkErr != "" {
		return LookupOutcome{RowsSeen: rows}, fmt.Errorf("%s", walkErr)
	}
	loose := false
	if (!ok || idx == "") && looseIdx != "" {
		idx, matchedLogin, loose = looseIdx, looseLogin, true
	} else if !ok || idx == "" {
		return LookupOutcome{Row: SessionRow{Status: "Down", Login: displayLogin}, RowsSeen: rows}, nil
	}
	if matchedLogin == "" {
		matchedLogin = displayLogin
	}

	columnMaps := map[string]map[string]string{
		"access_login": {idx: matchedLogin},
	}
	for key, m := range FetchSessionDetailMaps(ctx, host, community, profile, idx, 12*time.Second) {
		columnMaps[key] = m
	}

	merged := mergeSessionMaps(columnMaps, true)
	if len(merged) > 0 {
		out := ApplyLoginStripToSessions(merged, stripSuffix)
		return LookupOutcome{Row: out[0], Found: true, RowsSeen: rows, Loose: loose}, nil
	}
	return LookupOutcome{Row: SessionRow{Index: idx, Login: displayLogin, Status: "Up"}, Found: true, RowsSeen: rows, Loose: loose}, nil
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

