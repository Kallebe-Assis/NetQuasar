package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/zteparse"
)

type oltCollectExecState struct {
	DeviceID       uuid.UUID
	Host           string
	Community      string
	Brand          string
	Model          string
	DevDesc        string
	MaxPons        int
	Profile        oltcollect.Profile
	Summary        map[string]any
	Pons           []any
	StepLog        []map[string]any
	VsolPreload    *vsolIfPreload
	SkipStabilize  bool
	FullTelemetry  bool
	TelnetTO       time.Duration
	Scope          string
}

func (s *Server) executeOltProfile(ctx context.Context, st *oltCollectExecState) error {
	steps := oltcollect.EnabledSteps(st.Profile.Steps)
	steps = oltcollect.StepsForScope(steps, st.Scope)
	if len(steps) == 0 {
		st.Summary["olt_profile_error"] = "perfil sem passos de coleta — configure em Definições → OLT vendors"
		return fmt.Errorf("perfil sem passos de coleta")
	}
	st.Summary["olt_profile"] = map[string]any{
		"brand": st.Profile.Brand, "model": st.Profile.Model,
		"steps_count": len(steps),
	}
	st.Summary["olt_collection_mode"] = "profile_manual"
	if scope := strings.TrimSpace(st.Scope); scope != "" {
		st.Summary["olt_refresh_scope"] = scope
	}

	for _, step := range steps {
		t0 := time.Now()
		entry := map[string]any{
			"id": step.ID, "method": step.Method, "status": "ok",
		}
		if step.ID == "" {
			entry["id"] = step.Method
		}
		err := s.runOltCollectStep(ctx, st, step)
		entry["elapsed_ms"] = time.Since(t0).Milliseconds()
		if err != nil {
			entry["status"] = "error"
			entry["error"] = err.Error()
		}
		enrichOltStepLogEntry(entry, step.Method, st.Summary)
		st.StepLog = append(st.StepLog, entry)
		if ctx.Err() != nil {
			st.Summary["olt_refresh_cancelled"] = ctx.Err().Error()
			break
		}
	}
	st.Summary["olt_profile_steps"] = st.StepLog
	st.Summary["collection_log"] = buildOltCollectionLog(st.Summary)
	return nil
}

func (s *Server) runOltCollectStep(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	switch step.Method {
	case oltcollect.MethodIfMibRefresh:
		return s.oltStepIfMibRefresh(ctx, st)
	case oltcollect.MethodIfMibSnapshot:
		return s.oltStepIfMibSnapshot(ctx, st)
	case oltcollect.MethodVsolOnuCollect:
		return s.oltStepVsolOnuCollect(ctx, st, step)
	case oltcollect.MethodOnuMetricsCollect:
		return s.oltStepOnuMetricsCollect(ctx, st)
	case oltcollect.MethodOnuSNMPWalk:
		return s.oltStepOnuSNMPWalk(ctx, st, step)
	case oltcollect.MethodSNMPWalk:
		return s.oltStepSNMPWalk(ctx, st, step)
	case oltcollect.MethodSNMPGet:
		return s.oltStepSNMPGet(ctx, st, step)
	case oltcollect.MethodTelnet:
		return s.oltStepTelnet(ctx, st, step)
	case oltcollect.MethodDatacomBuildPons:
		return s.oltStepDatacomBuildPons(ctx, st)
	case oltcollect.MethodIfMibMergePons:
		return s.oltStepIfMibMergePons(ctx, st)
	case oltcollect.MethodStabilizePons:
		return s.oltStepStabilizePons(ctx, st)
	default:
		return fmt.Errorf("método desconhecido: %s", step.Method)
	}
}

func (s *Server) oltStepIfMibRefresh(ctx context.Context, st *oltCollectExecState) error {
	if st.Host == "" || st.Community == "" {
		return fmt.Errorf("host ou community SNMP em falta")
	}
	ifRefreshTO := s.loadCollectionTimeouts(ctx).InterfaceRefreshTotal()
	walkRes := collectInterfaceSNMPWalks(ctx, s.DB(), st.Host, st.Community, ifRefreshTO, false)
	merged := walkRes.Merged
	arr := make([]map[string]any, 0, len(merged)+1)
	for _, v := range merged {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	if walkRes.Truncated {
		arr = append(arr, map[string]any{"oid": "__netquasar.walk", "value": "truncated", "type": "meta"})
	}
	b, _ := json.Marshal(arr)
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base de dados indisponível")
	}
	_, err := pool.Exec(ctx, `INSERT INTO interface_snapshots (device_id, interfaces) VALUES ($1, $2::jsonb)`, st.DeviceID, b)
	if err != nil {
		return err
	}
	st.Summary["if_mib_refresh_rows"] = len(merged)
	st.Summary["if_mib_refresh_truncated"] = walkRes.Truncated
	if walkRes.Note != "" {
		st.Summary["if_mib_refresh_note"] = walkRes.Note
	}
	s.primeVsolPreloadFromSnapshot(ctx, st, 55*time.Second)
	return nil
}

func (s *Server) primeVsolPreloadFromSnapshot(ctx context.Context, st *oltCollectExecState, budget time.Duration) {
	if st == nil || st.Host == "" || st.Community == "" {
		return
	}
	pons, meta, refs, ok := s.vsolPonRowsFromIfMIB(ctx, st.DeviceID, st.Host, st.Community, budget, true)
	if !ok || len(refs) == 0 {
		return
	}
	st.VsolPreload = &vsolIfPreload{Pons: pons, Refs: refs, Meta: meta}
}

func (s *Server) oltStepIfMibSnapshot(ctx context.Context, st *oltCollectExecState) error {
	if st.Host == "" || st.Community == "" {
		return fmt.Errorf("host ou community SNMP em falta")
	}
	budget := 55 * time.Second
	snapOnly := strings.TrimSpace(st.Scope) == oltcollect.ScopeOnu
	pons, meta, refs, ok := s.vsolPonRowsFromIfMIB(ctx, st.DeviceID, st.Host, st.Community, budget, snapOnly)
	for k, v := range meta {
		st.Summary[k] = v
	}
	st.VsolPreload = &vsolIfPreload{Pons: pons, Refs: refs, Meta: meta}
	if ok {
		st.Summary["if_mib_snapshot_ok"] = true
	}
	return nil
}

func (s *Server) oltStepOnuMetricsCollect(ctx context.Context, st *oltCollectExecState) error {
	metrics := st.Profile.OnuMetrics
	if !metrics.HasAnyEnabled() {
		st.Summary["onu_metrics_missing"] = true
		st.Summary["onu_metrics_note"] = "Nenhuma MIB SNMP configurada para monitoramento deste modelo"
		return fmt.Errorf("nenhuma MIB SNMP configurada para monitoramento deste modelo")
	}
	budget := 240 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - 2*time.Second; left > 10*time.Second && left < budget {
			budget = left
		}
	}
	sum, pons, _, err := oltcollect.CollectOnuMetrics(ctx, st.Host, st.Community, metrics, budget, st.MaxPons)
	if err != nil {
		return err
	}
	for k, v := range sum {
		st.Summary[k] = v
	}
	if len(pons) > 0 {
		st.Pons = make([]any, 0, len(pons))
		for _, p := range pons {
			st.Pons = append(st.Pons, p)
		}
	}
	st.SkipStabilize = true
	return nil
}

func (s *Server) oltStepOnuSNMPWalk(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	oid := cleanSNMPOID(st.Profile.ResolveWalkOID(step))
	if oid == "" && strings.EqualFold(strings.TrimSpace(st.Brand), "vsol") {
		oid = vsolparse.DefaultVSOLOnuWalkOID
	}
	if oid == "" {
		return fmt.Errorf("onu_snmp_walk: defina onu_online_oid no perfil OLT (ex.: 1.3.6.1.4.1.37950.1.1.6.1.1)")
	}
	budget := 98 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - 2*time.Second; left > 10*time.Second && left < budget {
			budget = left
		}
	}
	sum, pons, vars, trunc, note, err := vsolparse.WalkOnuTable(ctx, st.Host, st.Community, oid, budget)
	if err != nil {
		return err
	}
	for k, v := range sum {
		st.Summary[k] = v
	}
	st.Summary["onu_walk_oid"] = oid
	st.Summary["onu_walk_var_count"] = len(vars)
	if trunc {
		st.Summary["onu_walk_truncated"] = true
	}
	if note != "" {
		st.Summary["onu_walk_note"] = note
	}
	if len(pons) > 0 {
		st.Pons = make([]any, 0, len(pons))
		for _, p := range pons {
			st.Pons = append(st.Pons, p)
		}
	}
	st.SkipStabilize = true
	return nil
}

func (s *Server) oltStepVsolOnuCollect(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	includeIF := stepBoolParam(step.Params, "include_if_mib", true)
	var preload *vsolIfPreload
	if !includeIF {
		preload = st.VsolPreload
		if preload == nil || len(preload.Refs) == 0 {
			return fmt.Errorf("vsol_onu_collect sem IF-MIB: execute if_mib_snapshot ou if_mib_refresh antes")
		}
	}
	vPons, _, vsolSum := s.collectVsolOLTWithPreload(ctx, st.DeviceID, st.Host, st.Community, st.FullTelemetry, preload)
	for k, v := range vsolSum {
		st.Summary[k] = v
	}
	if len(vPons) > 0 {
		st.Pons = make([]any, 0, len(vPons))
		for _, p := range vPons {
			st.Pons = append(st.Pons, p)
		}
	}
	st.SkipStabilize = true
	return nil
}

func (s *Server) oltStepSNMPWalk(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	oid := strings.TrimSpace(step.OID)
	if oid == "" && step.OIDField != "" {
		oid = st.Profile.OIDForField(step.OIDField)
	}
	oid = cleanSNMPOID(oid)
	if oid == "" {
		return fmt.Errorf("OID em falta no passo snmp_walk")
	}
	rows, trunc, note := walkRootRows(ctx, st.Host, st.Community, oid)
	key := strings.TrimSpace(step.StoreAs)
	if key == "" {
		key = "snmp_walk_" + strings.ReplaceAll(step.ID, "-", "_")
	}
	st.Summary[key] = rows
	if trunc {
		st.Summary[key+"_truncated"] = true
	}
	if note != "" {
		st.Summary[key+"_note"] = note
	}
	return nil
}

func (s *Server) oltStepSNMPGet(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	oids := make([]string, 0, len(step.OIDs)+1)
	if o := cleanSNMPOID(step.OID); o != "" {
		oids = append(oids, o)
	}
	for _, raw := range step.OIDs {
		if o := cleanSNMPOID(raw); o != "" {
			oids = append(oids, o)
		}
	}
	if len(oids) == 0 && step.OIDField != "" {
		if o := cleanSNMPOID(st.Profile.OIDForField(step.OIDField)); o != "" {
			oids = append(oids, o)
		}
	}
	if len(oids) == 0 {
		return fmt.Errorf("nenhum OID no passo snmp_get")
	}
	vars, note := probing.SNMPGetMany(ctx, st.Host, st.Community, "2c", 12*time.Second, 1, oids, 32)
	out := make([]map[string]any, 0, len(vars))
	for _, v := range vars {
		out = append(out, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	key := strings.TrimSpace(step.StoreAs)
	if key == "" {
		key = "snmp_get_" + strings.ReplaceAll(step.ID, "-", "_")
	}
	st.Summary[key] = out
	if note != "" {
		st.Summary[key+"_note"] = note
	}
	return nil
}

func (s *Server) oltStepTelnet(ctx context.Context, st *oltCollectExecState, step oltcollect.Step) error {
	cmd := strings.TrimSpace(step.Command)
	if cmd == "" {
		return fmt.Errorf("comando telnet em falta")
	}
	var telUser, telPass, telEnable *string
	_ = s.DB().QueryRow(ctx, `SELECT telnet_user, telnet_password, telnet_enable FROM settings_connection_defaults WHERE id=1`).Scan(&telUser, &telPass, &telEnable)
	tu, tp, te := "", "", ""
	if telUser != nil {
		tu = strings.TrimSpace(*telUser)
	}
	if telPass != nil {
		tp = strings.TrimSpace(*telPass)
	}
	if telEnable != nil {
		te = strings.TrimSpace(*telEnable)
	}
	if tu == "" || tp == "" {
		st.Summary["telnet_note"] = "credenciais telnet não configuradas em Definições → Ligação"
		return nil
	}
	tel := probing.TelnetRunCommand(ctx, probing.TelnetRunParams{
		Host: st.Host, Port: "23", Timeout: st.TelnetTO,
		User: tu, Password: tp, Enable: te,
		Command: cmd, PreCommands: step.PreCommands, MaxReadBytes: 220000,
	})
	key := strings.TrimSpace(step.StoreAs)
	if key == "" {
		key = "telnet_output"
	}
	st.Summary[key+"_note"] = tel.Note
	st.Summary[key+"_raw_snippet"] = trimForSummary(tel.Output, 2400)
	if !tel.OK {
		st.Summary[key+"_error"] = tel.Error
		return fmt.Errorf("telnet: %s", tel.Error)
	}
	switch strings.TrimSpace(step.Parser) {
	case oltcollect.ParserZteGponOnuState:
		rows := zteparse.ParseShowGponOnuState(tel.Output)
		st.Summary["zte_telnet_onu_state_rows"] = rows
		st.Summary["zte_telnet_onu_state_count"] = len(rows)
		if len(rows) == 0 {
			return nil
		}
		var ponRows []map[string]any
		if t, ok := st.Summary["zte_pon_status_table"].([]map[string]any); ok {
			ponRows = t
		} else if t, ok := st.Summary["zte_pon_status_table"].([]any); ok {
			for _, it := range t {
				if m, ok := it.(map[string]any); ok {
					ponRows = append(ponRows, m)
				}
			}
		}
		statusByIf := buildZTEPonStatusByIfIndex(ponRows)
		ifNameByIndex := map[int]string{}
		if pool := s.DB(); pool != nil {
			var ifRaw []byte
			if err := pool.QueryRow(ctx, `SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1`, st.DeviceID).Scan(&ifRaw); err == nil && len(ifRaw) > 0 {
				ifRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(ifRaw))
				for _, r := range ifRows {
					lb := strings.TrimSpace(r.DisplayName)
					if lb == "" {
						lb = strings.TrimSpace(r.IfName)
					}
					if lb == "" {
						lb = strings.TrimSpace(r.Descr)
					}
					ifNameByIndex[r.IfIndex] = lb
				}
			}
		}
		st.Pons = make([]any, 0, len(rows))
		for _, pr := range rows {
			ponSt := "unknown"
			for ifIdx, lb := range ifNameByIndex {
				if ztePonIDFromIfLabel(lb) == strings.TrimSpace(pr.Pon) {
					if x := strings.TrimSpace(statusByIf[ifIdx]); x != "" {
						ponSt = x
					}
					break
				}
			}
			st.Pons = append(st.Pons, map[string]any{
				"id": pr.Pon, "name": "PON-" + pr.Pon,
				"onu_total": pr.OnuTotal, "onu_online": pr.OnuOnline, "onu_offline": pr.OnuOffline,
				"status": ponSt, "source_slice": "zte_telnet_show_gpon_onu_state",
			})
		}
		st.Summary["zte_telnet_applied"] = true
	default:
		st.Summary[key+"_output"] = trimForSummary(tel.Output, 8000)
	}
	return nil
}

func (s *Server) oltStepDatacomBuildPons(ctx context.Context, st *oltCollectExecState) error {
	var onuRows []map[string]any
	if t, ok := st.Summary["datacom_onu_online_table"].([]map[string]any); ok {
		onuRows = t
	} else if t, ok := st.Summary["datacom_onu_online_table"].([]any); ok {
		for _, it := range t {
			if m, ok := it.(map[string]any); ok {
				onuRows = append(onuRows, m)
			}
		}
	}
	if len(onuRows) == 0 {
		return fmt.Errorf("datacom_build_pons: tabela datacom_onu_online_table vazia")
	}
	dRows := buildDatacomPonRowsFromTable(onuRows)
	if len(dRows) == 0 {
		return fmt.Errorf("datacom_build_pons: nenhuma PON derivada")
	}
	st.Pons = make([]any, 0, len(dRows))
	for _, pr := range dRows {
		st.Pons = append(st.Pons, pr)
	}
	st.Summary["datacom_pon_rows_count"] = len(dRows)
	return nil
}

func (s *Server) oltStepIfMibMergePons(ctx context.Context, st *oltCollectExecState) error {
	pool := s.DB()
	if pool == nil {
		return fmt.Errorf("base de dados indisponível")
	}
	var ifRaw []byte
	if err := pool.QueryRow(ctx, `SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1`, st.DeviceID).Scan(&ifRaw); err != nil || len(ifRaw) == 0 {
		return fmt.Errorf("sem snapshot IF-MIB — execute if_mib_refresh")
	}
	ifRows := snmpifparse.BuildIfTable(walkJSONToSNMPVars(ifRaw))
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, walkJSONToSNMPVars(ifRaw))
	derivedPons, sumPatch := oltifderive.DeriveFromIfRows(ifRows, optMap)
	if len(derivedPons) == 0 {
		return fmt.Errorf("IF-MIB não produziu PONs")
	}
	existing := oltifderive.PonsAnySliceToMaps(st.Pons)
	merged := oltifderive.MergePonRowsForIfaceRefresh(existing, derivedPons)
	st.Pons = oltifderive.PonsMapsToAny(merged)
	st.Summary["if_mib_merge_applied"] = true
	for k, v := range sumPatch {
		st.Summary[k] = v
	}
	return nil
}

func (s *Server) oltStepStabilizePons(ctx context.Context, st *oltCollectExecState) error {
	if st.SkipStabilize {
		st.Summary["stabilize_skipped"] = "vsol"
		return nil
	}
	pool := s.DB()
	if pool == nil {
		return nil
	}
	var prevSnapPons, prevSnapSum []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]'), COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, st.DeviceID).Scan(&prevSnapPons, &prevSnapSum)
	prevMaps := oltifderive.PonsJSONToMaps(prevSnapPons)
	prevSumm := oltifderive.SummaryJSONBytesToMap(prevSnapSum)
	newMaps := oltifderive.PonsAnySliceToMaps(st.Pons)
	incomplete := len(newMaps) < len(prevMaps) && len(prevMaps) > 0
	stabMaps, stabPatch := oltifderive.StabilizePonSnapshotRows(prevMaps, newMaps, prevSumm, incomplete)
	st.Pons = oltifderive.PonsMapsToAny(stabMaps)
	for k, v := range stabPatch {
		st.Summary[k] = v
	}
	return nil
}

func stepBoolParam(params map[string]any, key string, def bool) bool {
	if params == nil {
		return def
	}
	v, ok := params[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1"
	case float64:
		return x != 0
	default:
		return def
	}
}
