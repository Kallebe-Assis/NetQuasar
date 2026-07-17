package bngcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const maxSessionWalkRows = 50000

// FieldResult resultado por métrica.
type FieldResult struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	OK          bool   `json:"ok"`
	CollectMode string `json:"collect_mode"`
	OID         string `json:"oid"`
	Value       any    `json:"value,omitempty"`
	RowCount    int    `json:"row_count,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CollectionStatus estado geral da coleta.
type CollectionStatus struct {
	MissingOID []string `json:"missing_oid"`
	Enabled    int      `json:"enabled"`
	Collected  int      `json:"collected"`
	Failed     int      `json:"failed"`
	Message    string   `json:"message,omitempty"`
}

// CollectOutput resultado completo.
type CollectOutput struct {
	Fields map[string]FieldResult `json:"fields"`
	Status CollectionStatus       `json:"status"`
}

// CollectPeriodic executa métricas leves (GET escalares + sistema) para ciclo de monitoramento.
func CollectPeriodic(ctx context.Context, host, community string, profile Profile, timeout time.Duration) CollectOutput {
	return collectInternal(ctx, host, community, profile, timeout, false, nil)
}

// CollectProgressReporter callbacks de progresso (consulta completa PPPoE).
type CollectProgressReporter struct {
	OnLoginsLoaded   func(count int)
	OnSessionsLoaded func(enriched, total int)
	OnPhase          func(key, label string)
}

// CollectSessionsWalk coleta sessões PPPoE: 1 walk de logins + GET por índice.
func CollectSessionsWalk(ctx context.Context, host, community string, profile Profile, timeout time.Duration, report *CollectProgressReporter) (CollectOutput, []SessionRow) {
	profile = profileWithSessionWalksEnabled(profile)
	return collectSessionsByIndex(ctx, host, community, profile, timeout, report)
}

func profileWithSessionWalksEnabled(p Profile) Profile {
	if p.Metrics == nil {
		p.Metrics = DefaultMetrics()
	}
	for _, key := range SessionWalkKeys() {
		def := p.Metrics[key]
		if def.OID == "" {
			def.OID = catalogPlaceholder(key)
		}
		if def.CollectMode == "" {
			def.CollectMode = ModeSNMPWalk
		}
		def.Enabled = true
		p.Metrics[key] = def
	}
	return p
}

func collectInternal(ctx context.Context, host, community string, profile Profile, timeout time.Duration, sessionsOnly bool, report *CollectProgressReporter) CollectOutput {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	out := CollectOutput{
		Fields: make(map[string]FieldResult),
		Status: CollectionStatus{},
	}
	if host == "" || community == "" {
		out.Status.Message = "host ou community SNMP em falta"
		return out
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	walkTimeout := timeout
	if sessionsOnly {
		walkTimeout = 180 * time.Second
		if walkTimeout < timeout {
			walkTimeout = timeout
		}
	}

	for _, entry := range MetricCatalog {
		if sessionsOnly {
			found := false
			for _, k := range SessionWalkKeys() {
				if k == entry.Key {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		} else {
			isSession := false
			for _, k := range SessionWalkKeys() {
				if k == entry.Key {
					isSession = true
					break
				}
			}
			if isSession || isInterfaceWalkKey(entry.Key) {
				// Sessões PPPoE e IF-MIB: só sob demanda (consulta / monitor de interfaces).
				continue
			}
		}

		def, ok := profile.Metrics[entry.Key]
		if !ok {
			def = DefaultMetrics()[entry.Key]
		}
		if !def.Enabled {
			continue
		}
		out.Status.Enabled++
		oid := strings.TrimSpace(def.OID)
		if oid == "" {
			out.Status.MissingOID = append(out.Status.MissingOID, entry.Label)
			continue
		}
		mode := strings.TrimSpace(def.CollectMode)
		if mode == "" {
			mode = entry.DefaultMode
		}

		fr := FieldResult{
			Key:         entry.Key,
			Label:       entry.Label,
			CollectMode: mode,
			OID:         oid,
		}

		if report != nil && report.OnPhase != nil {
			report.OnPhase(entry.Key, "A carregar "+entry.Label+"…")
		}

		switch mode {
		case ModeSNMPWalk, ModeAccessSessions:
			var onProgress func(int)
			if report != nil && report.OnLoginsLoaded != nil && entry.Key == "access_login" {
				onProgress = func(n int) {
					if n == 1 || n%500 == 0 {
						report.OnLoginsLoaded(n)
					}
				}
			}
			vars, truncated, walkErr := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: host, Community: community, RootOID: oid, Version: "2c",
				Timeout: walkTimeout, MaxRows: maxSessionWalkRows,
				OnProgress: onProgress,
			})
			fr.RowCount = len(vars)
			fr.Truncated = truncated
			if walkErr != "" {
				fr.Error = walkErr
				out.Status.Failed++
			} else {
				fr.OK = true
				out.Status.Collected++
				fr.Value = vars
				if report != nil && report.OnLoginsLoaded != nil && entry.Key == "access_login" && len(vars) > 0 {
					report.OnLoginsLoaded(len(vars))
				}
			}
		default:
			res := probing.SNMPGet(ctx, probing.SNMPGetParams{
				Host: host, Community: community, OIDs: []string{oid}, Version: "2c",
				Timeout: timeout,
			})
			if !res.OK || len(res.Vars) == 0 {
				fr.Error = res.Error
				if fr.Error == "" {
					fr.Error = "sem resposta SNMP"
				}
				out.Status.Failed++
			} else {
				fr.OK = true
				fr.Value = res.Vars[0].Value
				out.Status.Collected++
			}
		}
		out.Fields[entry.Key] = fr
	}
	return out
}

func buildSessionsFromOutput(profile Profile, out CollectOutput) []SessionRow {
	columnMaps := make(map[string]map[string]string)
	for _, key := range SessionWalkKeys() {
		fr, ok := out.Fields[key]
		if !ok || !fr.OK {
			continue
		}
		def := profile.Metrics[key]
		base := strings.TrimSpace(def.OID)
		if base == "" {
			base = catalogPlaceholder(key)
		}
		m := make(map[string]string)
		for _, v := range walkVarsToSlice(fr.Value) {
			idx := extractIndexFromOID(v.OID, base)
			if idx == "" {
				continue
			}
			m[idx] = strings.TrimSpace(v.Value)
		}
		columnMaps[key] = m
	}
	sessions := mergeSessionMaps(columnMaps)
	return ApplyLoginStripToSessions(sessions, profile.Options.PPPoELoginStripSuffix)
}

func catalogPlaceholder(key string) string {
	for _, e := range MetricCatalog {
		if e.Key == key {
			return e.Placeholder
		}
	}
	return ""
}

// StatsTotals totais extraídos da coleta periódica.
type StatsTotals struct {
	TotalOnline     *int
	PPPoEOnline     *int
	IPv4Online      *int
	IPv6Online      *int
	DualStackOnline *int
}

func ExtractStatsTotals(out CollectOutput) StatsTotals {
	var st StatsTotals
	for _, key := range PeriodicTotalKeys() {
		fr, ok := out.Fields[key]
		if !ok || !fr.OK {
			continue
		}
		val, _ := fr.Value.(string)
		n, ok := parseIntMetric(val)
		if !ok {
			continue
		}
		switch key {
		case "total_online":
			st.TotalOnline = &n
		case "pppoe_online":
			st.PPPoEOnline = &n
		case "ipv4_online":
			st.IPv4Online = &n
		case "ipv6_online":
			st.IPv6Online = &n
		case "dual_stack_online":
			st.DualStackOnline = &n
		}
	}
	return st
}

// StoreStatsSample grava amostra de totais para gráficos.
func StoreStatsSample(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, st StatsTotals) error {
	if st.TotalOnline == nil && st.PPPoEOnline == nil && st.IPv4Online == nil && st.IPv6Online == nil && st.DualStackOnline == nil {
		return nil
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO bng_stats_samples (device_id, collected_at, total_online, pppoe_online, ipv4_online, ipv6_online, dual_stack_online)
		VALUES ($1, now(), $2, $3, $4, $5, $6)
	`, deviceID, st.TotalOnline, st.PPPoEOnline, st.IPv4Online, st.IPv6Online, st.DualStackOnline)
	return err
}

// StoreSessionSnapshot persiste lista de sessões PPPoE.
func StoreSessionSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, sessions []SessionRow, source string) error {
	payload := map[string]any{
		"sessions": sessions,
		"source":   source,
		"count":    len(sessions),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO bng_session_snapshots (device_id, captured_at, label, data, session_count)
		VALUES ($1, now(), $2, $3::jsonb, $4)
	`, deviceID, source, b, len(sessions))
	return err
}

// UpsertSessionInLatestSnapshot actualiza ou insere uma sessão no snapshot mais recente do BNG.
func UpsertSessionInLatestSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, row SessionRow, stripSuffix string) error {
	if pool == nil {
		return fmt.Errorf("pool indisponível")
	}
	row.Login = strings.TrimSpace(NormalizeSNMPLoginValue(row.Login, stripSuffix))
	if row.Login == "" {
		row.Login = strings.TrimSpace(row.Login)
	}

	var snapID int64
	var label string
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, label, data::text FROM bng_session_snapshots
		WHERE device_id=$1 ORDER BY captured_at DESC LIMIT 1
	`, deviceID).Scan(&snapID, &label, &raw)

	sessions := parseSessionRowsFromSnapshotJSON(raw)
	idx := findSessionRowIndex(sessions, row, stripSuffix)
	if idx >= 0 {
		sessions[idx] = row
	} else {
		sessions = append(sessions, row)
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			return StoreSessionSnapshot(ctx, pool, deviceID, sessions, "snmp_login_lookup")
		}
		return err
	}

	source := strings.TrimSpace(label)
	if source == "" {
		source = "snmp_access_table"
	}
	payload := map[string]any{
		"sessions": sessions,
		"source":   source,
		"count":    len(sessions),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		UPDATE bng_session_snapshots
		SET data=$1::jsonb, session_count=$2, captured_at=now()
		WHERE id=$3
	`, b, len(sessions), snapID)
	return err
}

func parseSessionRowsFromSnapshotJSON(raw []byte) []SessionRow {
	if len(raw) == 0 {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	arr, ok := doc["sessions"].([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return nil
	}
	var sessions []SessionRow
	if json.Unmarshal(b, &sessions) != nil {
		return nil
	}
	return sessions
}

// FindSessionIndexInLatestSnapshot devolve índice SNMP da sessão no último snapshot (lookup rápido).
func FindSessionIndexInLatestSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, login, stripSuffix string) string {
	if pool == nil {
		return ""
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return ""
	}
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT data::text FROM bng_session_snapshots
		WHERE device_id=$1 ORDER BY captured_at DESC LIMIT 1
	`, deviceID).Scan(&raw)
	if err != nil {
		return ""
	}
	sessions := parseSessionRowsFromSnapshotJSON(raw)
	targets := PPPoELoginLookupTargets(login, stripSuffix)
	for _, s := range sessions {
		idx := strings.TrimSpace(s.Index)
		if idx == "" {
			continue
		}
		for _, t := range targets {
			if MatchPPPoELogin(t, s.Login, stripSuffix) {
				return idx
			}
		}
	}
	return ""
}

func findSessionRowIndex(sessions []SessionRow, row SessionRow, stripSuffix string) int {
	if row.Index != "" {
		want := strings.TrimSpace(row.Index)
		for i, s := range sessions {
			if strings.TrimSpace(s.Index) == want {
				return i
			}
		}
	}
	if row.Login != "" {
		for i, s := range sessions {
			if MatchPPPoELogin(row.Login, s.Login, stripSuffix) {
				return i
			}
		}
	}
	return -1
}

// CollectAndStorePeriodic coleta periódica + grava telemetria e stats.
func CollectAndStorePeriodic(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, error) {
	return CollectAndStorePeriodicMode(ctx, pool, deviceID, host, community, timeout, "full")
}

// CollectAndStorePeriodicMode coleta periódica filtrada por modo (totals, health, system, full).
func CollectAndStorePeriodicMode(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration, mode string) (CollectOutput, error) {
	profile := LoadGlobalProfile(ctx, pool)
	profile = ProfileWithCollectMode(profile, mode)
	out := CollectPeriodic(ctx, host, community, profile, timeout)
	st := ExtractStatsTotals(out)
	_ = StoreStatsSample(ctx, pool, deviceID, st)

	metrics := map[string]any{
		"bng_collection": out,
		"profile_source": "settings_bng_collection",
		"collect_mode":   strings.TrimSpace(mode),
	}
	mode = strings.TrimSpace(mode)
	if mode == "" || mode == "full" {
		_ = CollectAndStoreInfrastructure(ctx, pool, deviceID, host, community, timeout, profile.Options)
	}
	b, _ := json.Marshal(metrics)
	_, err := pool.Exec(ctx, `
		INSERT INTO telemetry_samples (device_id, collected_at, metrics) VALUES ($1, now(), $2::jsonb)
	`, deviceID, b)
	return out, err
}

func fmtIntAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func walkVarsToSlice(v any) []probing.SNMPVar {
	switch x := v.(type) {
	case []probing.SNMPVar:
		return x
	case []any:
		out := make([]probing.SNMPVar, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, probing.SNMPVar{
				OID:   stringField(m, "oid"),
				Type:  stringField(m, "type"),
				Value: stringField(m, "value"),
			})
		}
		return out
	default:
		return nil
	}
}

func stringField(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
