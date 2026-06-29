package bngcollect

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const (
	onlineFailTableBase    = "1.3.6.1.4.1.2011.5.2.1.48.1"
	offlineRecordBase      = "1.3.6.1.4.1.2011.5.2.1.52.1"
	accessOnlineTimeBase   = "1.3.6.1.4.1.2011.5.2.1.16.1.18"
	pppoeOnlineScalarOID   = "1.3.6.1.4.1.2011.5.2.1.14.1.2.0"
	authFailPollSize       = 100
	authFailIndexOverlap   = 30
	authSNMPCollectTimeout = 18 * time.Second
	authScalarTimeout      = 4 * time.Second
	authSessionRecentSec   = 180
	authBootstrapOnlineSec = 900
)

// AuthAttemptLog registo de tentativa/sessão de autenticação para UI.
type AuthAttemptLog struct {
	Kind    string `json:"kind"`
	Time    string `json:"time,omitempty"`
	Login   string `json:"login,omitempty"`
	MAC     string `json:"mac,omitempty"`
	Port    string `json:"port,omitempty"`
	Seq     string `json:"seq,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Message string `json:"message,omitempty"`
}

const (
	maxAuthRecordsPerLogin = 25
	maxAuthRecordsGeneral  = 50
)

func fetchRecentBngAuthRecordsFast(ctx context.Context, host, community string, limit int, stripSuffix string) []AuthAttemptLog {
	if limit <= 0 || limit > maxAuthRecordsGeneral {
		limit = maxAuthRecordsGeneral
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.TrimSpace(community) == "" {
		return nil
	}
	timeout := authSNMPCollectTimeout

	userMap := snmpWalkTopColumn(ctx, host, community, timeout, onlineFailTableBase+".2", authFailPollSize)
	failLogs := buildFailAuthLogs(ctx, host, community, userMap, nil, authFailPollSize, timeout, stripSuffix)

	onlineMap := snmpWalkColumnInt(ctx, host, community, timeout, accessOnlineTimeBase)
	okLogs := buildSuccessLogsFromOnlineMap(ctx, host, community, onlineMap, authBootstrapOnlineSec, timeout, nil, stripSuffix)

	out := append(failLogs, okLogs...)
	return sortLimitAuthRecords(out, limit)
}

// pollAuthEvents — arquitectura A+B+C: falhas incrementais, escalar PPPoE, walk sessões sob demanda.
func pollAuthEvents(ctx context.Context, host, community string, st *authPollState, stripSuffix string) []AuthAttemptLog {
	if st == nil {
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.TrimSpace(community) == "" {
		return nil
	}
	now := time.Now()
	timeout := authSNMPCollectTimeout

	// B: escalar PPPoE online (GET rápido, cada poll).
	pppoeCount, pppoeOK := snmpGetScalarInt(ctx, host, community, pppoeOnlineScalarOID, authScalarTimeout)

	// A: falhas incrementais (walk top + GET só índices novos).
	failLogs := pollIncrementalFailLogs(ctx, host, community, st, timeout, stripSuffix)

	// C: walk sessões só quando PPPoE subiu ou intervalo periódico.
	var okLogs []AuthAttemptLog
	if st.shouldWalkSessions(pppoeCount, pppoeOK, now) {
		maxSec := authSessionRecentSec
		if !st.bootstrapped {
			maxSec = authBootstrapOnlineSec
		}
		onlineMap := snmpWalkColumnInt(ctx, host, community, timeout, accessOnlineTimeBase)
		okLogs = buildSuccessLogsFromOnlineMap(ctx, host, community, onlineMap, maxSec, timeout, st, stripSuffix)
		st.noteSessionWalk(now)
	}
	st.notePPPoECount(pppoeCount, pppoeOK)
	st.bootstrapped = true

	var fresh []AuthAttemptLog
	for _, rec := range append(failLogs, okLogs...) {
		if st.markSeen(rec) {
			fresh = append(fresh, rec)
		}
	}
	return fresh
}

func pollIncrementalFailLogs(ctx context.Context, host, community string, st *authPollState, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	userMap := snmpWalkTopColumn(ctx, host, community, timeout, onlineFailTableBase+".2", authFailPollSize)
	if len(userMap) == 0 {
		return nil
	}

	threshold := st.failIndexThreshold()
	if len(st.ring) == 0 {
		threshold = 0
	}
	var newIndices []string
	allIndices := make([]string, 0, len(userMap))
	for idx := range userMap {
		allIndices = append(allIndices, idx)
		if authIndexNum(idx) > threshold {
			newIndices = append(newIndices, idx)
		}
	}
	st.updateMaxFailIndex(allIndices)

	if len(newIndices) == 0 {
		return nil
	}
	sort.Slice(newIndices, func(i, j int) bool {
		return authIndexNum(newIndices[i]) > authIndexNum(newIndices[j])
	})
	return buildFailAuthLogsForIndices(ctx, host, community, userMap, newIndices, timeout, stripSuffix)
}

// FetchRecentBngAuthRecords obtém os registos AAA mais recentes (todos os logins).
func FetchRecentBngAuthRecords(ctx context.Context, host, community string, limit int, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	_ = timeout
	return fetchRecentBngAuthRecordsFast(ctx, host, community, limit, stripSuffix)
}

// FetchAuthAttemptsForLogin obtém falhas e sessões offline filtradas por login (walk mínimo).
func FetchAuthAttemptsForLogin(ctx context.Context, host, community, login string, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	login = strings.TrimSpace(login)
	if login == "" || strings.TrimSpace(host) == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	target := strings.ToLower(NormalizeSNMPLoginValue(login, stripSuffix))
	if target == "" {
		target = strings.ToLower(login)
	}

	filter := func(user string) bool {
		return matchLoginAttempt(user, target, login, stripSuffix)
	}

	var failLogs, offLogs []AuthAttemptLog
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		userMap := snmpWalkTopColumn(ctx, host, community, timeout, onlineFailTableBase+".2", 500)
		failLogs = buildFailAuthLogs(ctx, host, community, userMap, filter, maxAuthRecordsPerLogin, timeout, stripSuffix)
	}()
	go func() {
		defer wg.Done()
		userMap := snmpWalkTopColumn(ctx, host, community, timeout, offlineRecordBase+".2", 500)
		offLogs = buildOfflineAuthLogs(ctx, host, community, userMap, filter, maxAuthRecordsPerLogin, timeout, stripSuffix)
	}()
	wg.Wait()

	out := append(failLogs, offLogs...)
	return filterSortLimitAuthRecords(out, maxAuthRecordsPerLogin)
}

func buildFailAuthLogs(ctx context.Context, host, community string, userMap map[string]string, filter func(string) bool, limit int, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	if len(userMap) == 0 {
		return nil
	}
	matched := pickIndices(userMap, filter, limit)
	if len(matched) == 0 {
		return nil
	}
	return buildFailAuthLogsForIndices(ctx, host, community, userMap, matched, timeout, stripSuffix)
}

func buildFailAuthLogsForIndices(ctx context.Context, host, community string, userMap map[string]string, indices []string, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	if len(indices) == 0 {
		return nil
	}

	fieldSuffix := map[string]string{
		"time":      ".19",
		"reason":    ".20",
		"reply":     ".21",
		"mac":       ".4",
		"interface": ".6",
	}
	byIdx := fetchAuthFieldsByIndex(ctx, host, community, onlineFailTableBase, indices, fieldSuffix, timeout)

	var out []AuthAttemptLog
	for _, idx := range indices {
		f := byIdx[idx]
		reason := strings.TrimSpace(f["reason"])
		reply := strings.TrimSpace(f["reply"])
		if reason == "" {
			reason = reply
		}
		detail := reason
		if reply != "" && reply != reason {
			detail = reason + " · " + reply
		}
		loginTime := normalizeAuthRecordTime(f["time"])
		if loginTime == "" {
			loginTime = time.Now().Format("2006-01-02 15:04:05")
		}
		rec := AuthAttemptLog{
			Kind:   "failure",
			Login:  strings.TrimSpace(userMap[idx]),
			Time:   loginTime,
			MAC:    f["mac"],
			Port:   strings.TrimSpace(f["interface"]),
			Seq:    idx,
			Reason: reason,
			Detail: detail,
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	}
	return out
}

func buildSuccessLogsFromOnlineMap(ctx context.Context, host, community string, onlineMap map[string]int, maxOnlineSec int, timeout time.Duration, st *authPollState, stripSuffix string) []AuthAttemptLog {
	if len(onlineMap) == 0 {
		return nil
	}

	type candidate struct {
		idx string
		sec int
	}
	var candidates []candidate
	now := time.Now()

	for idx, sec := range onlineMap {
		if sec < 0 || sec > maxOnlineSec {
			if st != nil {
				st.prevOnline[idx] = sec
			}
			continue
		}
		if st != nil {
			st.prevOnline[idx] = sec
		}
		candidates = append(candidates, candidate{idx: idx, sec: sec})
	}
	if len(candidates) == 0 {
		return nil
	}

	indices := make([]string, len(candidates))
	for i, c := range candidates {
		indices[i] = c.idx
	}

	fieldSuffix := map[string]string{
		"login": ".3",
		"mac":   ".17",
		"port":  ".10",
		"start": ".25",
	}
	byIdx := fetchAuthFieldsByIndexMultiBase(ctx, host, community, map[string]map[string]string{
		"15.1": fieldSuffix,
	}, indices, timeout)

	var out []AuthAttemptLog
	for _, c := range candidates {
		f := byIdx[c.idx]
		login := NormalizeSNMPLoginValue(f["login"], stripSuffix)
		if login == "" || looksLikeIPAddressLogin(login) {
			continue
		}
		loginTime := normalizeAuthRecordTime(f["start"])
		if loginTime == "" && c.sec > 0 {
			loginTime = now.Add(-time.Duration(c.sec) * time.Second).Format("2006-01-02 15:04:05")
		}
		port := strings.TrimSpace(f["port"])
		rec := AuthAttemptLog{
			Kind:   "success",
			Login:  login,
			Time:   loginTime,
			MAC:    f["mac"],
			Port:   port,
			Reason: "Login OK",
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	}
	return out
}

func buildOfflineAuthLogs(ctx context.Context, host, community string, userMap map[string]string, filter func(string) bool, limit int, timeout time.Duration, stripSuffix string) []AuthAttemptLog {
	if len(userMap) == 0 {
		return nil
	}
	matched := pickIndices(userMap, filter, limit)
	if len(matched) == 0 {
		return nil
	}

	fieldSuffix := map[string]string{
		"login":  ".11",
		"reason": ".13",
		"authen": ".19",
		"mac":    ".4",
	}
	byIdx := fetchAuthFieldsByIndex(ctx, host, community, offlineRecordBase, matched, fieldSuffix, timeout)

	var out []AuthAttemptLog
	for _, idx := range matched {
		f := byIdx[idx]
		authen := strings.TrimSpace(f["authen"])
		if authen != "" && authen != "2" && authen != "authed" {
			continue
		}
		rec := AuthAttemptLog{
			Kind:   "success",
			Login:  strings.TrimSpace(userMap[idx]),
			Time:   normalizeAuthRecordTime(f["login"]),
			MAC:    f["mac"],
			Reason: "Sessão autenticada",
			Detail: strings.TrimSpace(f["reason"]),
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	}
	return out
}

func snmpGetScalarInt(ctx context.Context, host, community, oid string, timeout time.Duration) (int, bool) {
	res := probing.SNMPGet(ctx, probing.SNMPGetParams{
		Host:      host,
		Community: community,
		Version:   "2c",
		OIDs:      []string{oid},
		Timeout:   timeout,
	})
	if !res.OK || len(res.Vars) == 0 {
		return 0, false
	}
	return parseIntMetric(strings.TrimSpace(res.Vars[0].Value))
}

func looksLikeIPAddressLogin(s string) bool {
	s = strings.TrimSpace(strings.TrimSuffix(s, "/"))
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func pickIndices(userMap map[string]string, filter func(string) bool, limit int) []string {
	var matched []string
	for idx, user := range userMap {
		user = strings.TrimSpace(user)
		if user == "" {
			continue
		}
		if filter != nil && !filter(user) {
			continue
		}
		matched = append(matched, idx)
	}
	if len(matched) == 0 {
		return nil
	}
	sort.Slice(matched, func(i, j int) bool {
		return authIndexNum(matched[i]) > authIndexNum(matched[j])
	})
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched
}

func snmpWalkTopColumn(ctx context.Context, host, community string, timeout time.Duration, rootOID string, keep int) map[string]string {
	if keep <= 0 {
		keep = authFailPollSize
	}
	vars, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: rootOID, Version: "2c",
		Timeout: timeout, MaxRows: 5000,
	})
	type row struct {
		idx string
		val string
		num int64
	}
	rows := make([]row, 0, len(vars))
	for _, v := range vars {
		idx := extractIndexFromOID(v.OID, rootOID)
		if idx == "" {
			continue
		}
		rows = append(rows, row{idx: idx, val: strings.TrimSpace(v.Value), num: authIndexNum(idx)})
	}
	if len(rows) == 0 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].num > rows[j].num })
	if len(rows) > keep {
		rows = rows[:keep]
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.idx] = r.val
	}
	return out
}

func snmpWalkColumnInt(ctx context.Context, host, community string, timeout time.Duration, rootOID string) map[string]int {
	vars, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: rootOID, Version: "2c",
		Timeout: timeout, MaxRows: 8000,
	})
	out := make(map[string]int, len(vars))
	for _, v := range vars {
		idx := extractIndexFromOID(v.OID, rootOID)
		if idx == "" {
			continue
		}
		sec, ok := parseIntMetric(strings.TrimSpace(v.Value))
		if !ok {
			continue
		}
		out[idx] = sec
	}
	return out
}

func authIndexNum(idx string) int64 {
	idx = strings.TrimSpace(idx)
	if idx == "" {
		return 0
	}
	n, err := strconv.ParseInt(idx, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func fetchAuthFieldsByIndex(ctx context.Context, host, community, tableBase string, indices []string, fieldSuffix map[string]string, timeout time.Duration) map[string]map[string]string {
	return fetchAuthFieldsByIndexMultiBase(ctx, host, community, map[string]map[string]string{
		tableSuffix(tableBase): fieldSuffix,
	}, indices, timeout)
}

func tableSuffix(tableBase string) string {
	parts := strings.Split(strings.Trim(tableBase, "."), ".")
	if len(parts) < 2 {
		return tableBase
	}
	return parts[len(parts)-2] + "." + parts[len(parts)-1]
}

func fetchAuthFieldsByIndexMultiBase(ctx context.Context, host, community string, tables map[string]map[string]string, indices []string, timeout time.Duration) map[string]map[string]string {
	var oids []string
	for _, idx := range indices {
		for suffix, fields := range tables {
			base := "1.3.6.1.4.1.2011.5.2.1." + suffix
			for _, col := range fields {
				oids = append(oids, base+col+"."+idx)
			}
		}
	}
	vals, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, oids, 40)
	byIdx := make(map[string]map[string]string, len(indices))
	for _, v := range vals {
		oid := probing.NormalizeSNMPOID(v.OID)
		for _, idx := range indices {
			for suffix, fields := range tables {
				base := "1.3.6.1.4.1.2011.5.2.1." + suffix
				for field, col := range fields {
					if strings.HasSuffix(oid, col+"."+idx) && strings.Contains(oid, base) {
						if byIdx[idx] == nil {
							byIdx[idx] = map[string]string{}
						}
						byIdx[idx][field] = strings.TrimSpace(v.Value)
					}
				}
			}
		}
	}
	return byIdx
}

func matchLoginAttempt(user, target, rawLogin, stripSuffix string) bool {
	if user == "" {
		return false
	}
	user = strings.ToLower(NormalizeSNMPLoginValue(user, stripSuffix))
	if user == target {
		return true
	}
	return strings.Contains(user, target) || strings.Contains(target, user) ||
		strings.EqualFold(user, NormalizeSNMPLoginValue(rawLogin, stripSuffix))
}

// AuthLogsFromSession gera entradas a partir do estado actual da sessão online.
func AuthLogsFromSession(row SessionRow, stripSuffix string) []AuthAttemptLog {
	var out []AuthAttemptLog
	auth := strings.ToLower(strings.TrimSpace(row.AuthState))
	author := strings.ToLower(strings.TrimSpace(row.AuthorState))
	loginTime := ""
	if sec, ok := parseIntMetric(row.OnlineTimeSec); ok && sec > 0 {
		loginTime = ApproxLoginTimeFromOnlineSec(sec)
	}
	if strings.Contains(auth, "autenticado") || row.AuthStateRaw == "3" {
		rec := AuthAttemptLog{
			Kind:   "success",
			Login:  strings.TrimSpace(row.Login),
			MAC:    strings.TrimSpace(row.MAC),
			Time:   loginTime,
			Port:   strings.TrimSpace(row.Interface),
			Reason: "Autenticado (sessão online)",
			Detail: row.AuthState,
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	} else if row.AuthState != "" {
		rec := AuthAttemptLog{
			Kind:   "failure",
			Login:  strings.TrimSpace(row.Login),
			MAC:    strings.TrimSpace(row.MAC),
			Time:   loginTime,
			Port:   strings.TrimSpace(row.Interface),
			Reason: row.AuthState,
			Detail: "Estado actual da autenticação",
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	}
	if strings.Contains(author, "autorizado") || row.AuthorStateRaw == "3" {
		rec := AuthAttemptLog{
			Kind:   "success",
			Login:  strings.TrimSpace(row.Login),
			MAC:    strings.TrimSpace(row.MAC),
			Time:   loginTime,
			Port:   strings.TrimSpace(row.Interface),
			Reason: "Autorizado (sessão online)",
			Detail: row.AuthorState,
		}
		finalizeAuthRecord(&rec, stripSuffix)
		out = append(out, rec)
	}
	return out
}

func authEventDedupeKey(rec AuthAttemptLog) string {
	if rec.Seq != "" && rec.Kind == "failure" {
		return fmt.Sprintf("fail:%s", rec.Seq)
	}
	return fmt.Sprintf("%s|%s|%s|%s", rec.Kind, strings.ToLower(strings.TrimSpace(rec.Login)), strings.ToUpper(strings.TrimSpace(rec.MAC)), strings.TrimSpace(rec.Time))
}
