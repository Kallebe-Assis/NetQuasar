package bngcollect

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	return collectInternal(ctx, host, community, profile, timeout, false)
}

// CollectSessionsWalk coleta colunas de sessão PPPoE (vários walks — operação pesada).
func CollectSessionsWalk(ctx context.Context, host, community string, profile Profile, timeout time.Duration) (CollectOutput, []SessionRow) {
	profile = profileWithSessionWalksEnabled(profile)
	out := collectInternal(ctx, host, community, profile, timeout, true)
	sessions := buildSessionsFromOutput(profile, out)
	return out, sessions
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

func collectInternal(ctx context.Context, host, community string, profile Profile, timeout time.Duration, sessionsOnly bool) CollectOutput {
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
			if isSession {
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

		switch mode {
		case ModeSNMPWalk, ModeAccessSessions:
			vars, truncated, walkErr := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: host, Community: community, RootOID: oid, Version: "2c",
				Timeout: walkTimeout, MaxRows: maxSessionWalkRows,
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
	return mergeSessionMaps(columnMaps)
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

// CollectAndStorePeriodic coleta periódica + grava telemetria e stats.
func CollectAndStorePeriodic(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration) (CollectOutput, error) {
	profile := LoadGlobalProfile(ctx, pool)
	out := CollectPeriodic(ctx, host, community, profile, timeout)
	st := ExtractStatsTotals(out)
	_ = StoreStatsSample(ctx, pool, deviceID, st)

	metrics := map[string]any{
		"bng_collection": out,
		"profile_source": "settings_bng_collection",
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
