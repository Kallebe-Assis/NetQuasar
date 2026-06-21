package monitorworker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/zteparse"
)

type oltWorkerExecState struct {
	Pool      *pgxpool.Pool
	DeviceID  uuid.UUID
	Host      string
	Community string
	Brand     string
	Model     string
	DevDesc   string
	MaxPons   int
	Profile   oltcollect.Profile
	Summary   map[string]any
	Pons      []map[string]any
	VsolPre   *vsolWorkerPreload
	SkipStab  bool
	TelnetTO  time.Duration
	// StepsOverride passos do perfil antes de StepsForScope (coleta periódica simplificada).
	StepsOverride []oltcollect.Step
}

type vsolWorkerPreload struct {
	Pons []map[string]any
	Refs []vsolparse.OnuRef
	Meta map[string]any
}

// runOltProfileStepsWorker executa os passos do perfil OLT (mesma lógica do refresh manual).
func runOltProfileStepsWorker(ctx context.Context, st *oltWorkerExecState, scope string) error {
	rawSteps := st.StepsOverride
	if len(rawSteps) == 0 {
		rawSteps = oltcollect.EffectiveCollectionSteps(st.Profile)
	}
	steps := oltcollect.StepsForScope(rawSteps, scope)
	if len(steps) == 0 {
		st.Summary["olt_profile_error"] = "perfil sem passos de coleta activos"
		return fmt.Errorf("perfil sem passos de coleta")
	}
	st.Summary["olt_collection_mode"] = "profile_periodic"
	st.Summary["olt_profile"] = map[string]any{
		"brand": st.Profile.Brand, "model": st.Profile.Model, "steps_count": len(steps),
	}
	if scope != "" {
		st.Summary["olt_refresh_scope"] = scope
	}

	for _, step := range steps {
		if ctx.Err() != nil {
			st.Summary["olt_refresh_cancelled"] = ctx.Err().Error()
			break
		}
		if err := runOltWorkerStep(ctx, st, step); err != nil {
			return err
		}
	}
	return nil
}

func runOltWorkerStep(ctx context.Context, st *oltWorkerExecState, step oltcollect.Step) error {
	switch step.Method {
	case oltcollect.MethodIfMibRefresh:
		return oltWorkerIfMibRefresh(ctx, st)
	case oltcollect.MethodIfMibSnapshot:
		return oltWorkerIfMibSnapshot(ctx, st)
	case oltcollect.MethodVsolOnuCollect:
		return oltWorkerVsolOnuCollect(ctx, st, step)
	case oltcollect.MethodOnuMetricsCollect:
		return oltWorkerOnuMetricsCollect(ctx, st)
	case oltcollect.MethodOnuSNMPWalk:
		return oltWorkerOnuSNMPWalk(ctx, st, step)
	case oltcollect.MethodSNMPWalk:
		return oltWorkerSNMPWalk(ctx, st, step)
	case oltcollect.MethodTelnet:
		return oltWorkerTelnet(ctx, st, step)
	case oltcollect.MethodDatacomBuildPons:
		return oltWorkerDatacomBuildPons(st)
	case oltcollect.MethodIfMibMergePons:
		return oltWorkerIfMibMergePons(ctx, st)
	case oltcollect.MethodStabilizePons:
		return oltWorkerStabilizePons(ctx, st)
	default:
		return nil
	}
}

func oltWorkerIfMibRefresh(ctx context.Context, st *oltWorkerExecState) error {
	if st.Pool == nil || st.Host == "" || st.Community == "" {
		return fmt.Errorf("host ou community SNMP em falta")
	}
	budget := 120 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - 2*time.Second; left > 15*time.Second {
			budget = left
		}
	}
	ds := oltifderive.LoadIFMibDataset(ctx, st.Host, st.Community, budget, nil)
	arr := make([]map[string]any, 0, len(ds.Vars))
	for _, v := range ds.Vars {
		arr = append(arr, map[string]any{"oid": v.OID, "value": v.Value, "type": v.Type})
	}
	b, _ := json.Marshal(arr)
	_, err := st.Pool.Exec(ctx, `INSERT INTO interface_snapshots (device_id, interfaces) VALUES ($1, $2::jsonb)`, st.DeviceID, b)
	if err != nil {
		return err
	}
	st.Summary["if_mib_refresh_rows"] = len(ds.Vars)
	st.Summary["if_mib_refresh_truncated"] = ds.Truncated
	if ds.Note != "" {
		st.Summary["if_mib_refresh_note"] = ds.Note
	}
	oltWorkerPrimeVsolPreload(ctx, st, 55*time.Second)
	return nil
}

func oltWorkerIfMibSnapshot(ctx context.Context, st *oltWorkerExecState) error {
	pons, meta, refs, ok := oltWorkerVsolPonFromIfMIB(ctx, st, 55*time.Second, true)
	for k, v := range meta {
		st.Summary[k] = v
	}
	st.VsolPre = &vsolWorkerPreload{Pons: pons, Refs: refs, Meta: meta}
	if ok {
		st.Summary["if_mib_snapshot_ok"] = true
	}
	return nil
}

func oltWorkerPrimeVsolPreload(ctx context.Context, st *oltWorkerExecState, budget time.Duration) {
	pons, meta, refs, ok := oltWorkerVsolPonFromIfMIB(ctx, st, budget, true)
	if !ok || len(refs) == 0 {
		return
	}
	st.VsolPre = &vsolWorkerPreload{Pons: pons, Refs: refs, Meta: meta}
}

func oltWorkerVsolPonFromIfMIB(ctx context.Context, st *oltWorkerExecState, budget time.Duration, snapshotOnly bool) ([]map[string]any, map[string]any, []vsolparse.OnuRef, bool) {
	var raw []byte
	if st.Pool != nil {
		_ = st.Pool.QueryRow(ctx, `
			SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
		`, st.DeviceID).Scan(&raw)
	}
	var ds oltifderive.IfMibDataset
	if snapshotOnly && len(raw) > 0 {
		ds = oltifderive.DatasetFromSnapshot(raw)
		if len(vsolparse.OnuRefsFromIfRows(ds.Rows)) == 0 {
			ds = oltifderive.LoadIFMibDataset(ctx, st.Host, st.Community, budget, raw)
		}
	} else {
		ds = oltifderive.LoadIFMibDataset(ctx, st.Host, st.Community, budget, raw)
	}
	refs := vsolparse.OnuRefsFromIfRows(ds.Rows)
	meta := map[string]any{
		"if_mib_source": ds.Source, "if_mib_onu_ifaces": ds.OnuIfaces,
		"if_mib_pon_with_onu": ds.PonWithOnu, "vsol_onu_refs": len(refs),
	}
	if ds.Note != "" {
		meta["if_mib_note"] = ds.Note
	}
	if len(ds.Rows) == 0 || (ds.OnuIfaces == 0 && ds.PonWithOnu == 0) {
		return nil, meta, refs, false
	}
	opt := snmpmikrotik.OpticalPowerByIfIndex(ds.Rows, ds.Vars)
	rows := oltifderive.BuildPonSnapshotFromIfMIB(ds.Rows, opt)
	meta["if_mib_pon_rows"] = len(rows)
	return rows, meta, refs, len(rows) > 0
}

func oltWorkerOnuMetricsCollect(ctx context.Context, st *oltWorkerExecState) error {
	metrics := st.Profile.OnuMetrics
	if !metrics.HasAnyEnabled() {
		return fmt.Errorf("nenhuma MIB SNMP configurada para monitoramento deste modelo")
	}
	budget := 240 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - 2*time.Second; left > 30*time.Second {
			budget = left
		}
	}
	mctx, mcancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	sum, pons, _, err := oltcollect.CollectOnuMetrics(mctx, st.Host, st.Community, metrics, budget, st.MaxPons)
	mcancel()
	if err != nil {
		return err
	}
	for k, v := range sum {
		st.Summary[k] = v
	}
	if len(pons) > 0 {
		st.Pons = pons
	} else if len(st.Pons) == 0 && strings.Contains(strings.ToLower(strings.TrimSpace(st.Brand)), "vsol") {
		// Métricas vazias: não apagar PONs já obtidas pelo passo onu_snmp_walk anterior.
		oid := strings.TrimSpace(st.Profile.OnuOnlineOID)
		if oid == "" {
			oid = vsolparse.DefaultVSOLOnuWalkOID
		}
		walkBudget := 90 * time.Second
		if dl, ok := ctx.Deadline(); ok {
			if left := time.Until(dl) - 2*time.Second; left > 25*time.Second {
				walkBudget = left
			}
		}
		if walkBudget >= 25*time.Second {
			wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), walkBudget)
			wsum, wpons, _, _, _, werr := vsolparse.WalkOnuTable(wctx, st.Host, st.Community, oid, walkBudget)
			wcancel()
			if werr == nil && len(wpons) > 0 {
				for k, v := range wsum {
					st.Summary[k] = v
				}
				st.Summary["onu_metrics_fallback"] = "vsol_onu_snmp_walk"
				st.Pons = wpons
			}
		}
	}
	st.SkipStab = true
	return nil
}

func oltWorkerOnuSNMPWalk(ctx context.Context, st *oltWorkerExecState, step oltcollect.Step) error {
	oid := cleanOltSNMPOID(st.Profile.ResolveWalkOID(step))
	if oid == "" && strings.EqualFold(st.Brand, "vsol") {
		oid = vsolparse.DefaultVSOLOnuWalkOID
	}
	if oid == "" {
		return fmt.Errorf("onu_snmp_walk: defina onu_online_oid no perfil OLT")
	}
	budget := 98 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - 2*time.Second; left > 10*time.Second && left < budget {
			budget = left
		}
	}
	sum, pons, _, _, _, err := vsolparse.WalkOnuTable(ctx, st.Host, st.Community, oid, budget)
	if err != nil {
		return err
	}
	for k, v := range sum {
		st.Summary[k] = v
	}
	if len(pons) > 0 {
		st.Pons = pons
	}
	st.SkipStab = true
	return nil
}

func oltWorkerVsolOnuCollect(ctx context.Context, st *oltWorkerExecState, step oltcollect.Step) error {
	includeIF := oltWorkerBoolParam(step.Params, "include_if_mib", true)
	var refs []vsolparse.OnuRef
	var vPons []map[string]any
	sum := map[string]any{}

	if !includeIF && st.VsolPre != nil && len(st.VsolPre.Refs) > 0 {
		vPons = st.VsolPre.Pons
		refs = st.VsolPre.Refs
		for k, v := range st.VsolPre.Meta {
			sum[k] = v
		}
	} else {
		ponsIF, meta, r, ok := oltWorkerVsolPonFromIfMIB(ctx, st, 55*time.Second, false)
		if ok {
			vPons = ponsIF
			refs = r
			for k, v := range meta {
				sum[k] = v
			}
		}
	}
	if len(refs) == 0 {
		return fmt.Errorf("vsol_onu_collect: sem ONUs no IF-MIB")
	}
	mibTO := vsolparse.CollectTimeout(len(refs), false)
	mibCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mibTO)
	defer cancel()
	coll := vsolparse.CollectOLT(mibCtx, st.Host, st.Community, refs, false)
	onBy, offBy := vsolparse.OnlineOfflineByPon(coll.Vars)
	if len(vPons) > 0 && vsolparse.OnlineStepComplete(coll) {
		vPons = vsolparse.AttachOnlineOfflineToIfPons(vPons, onBy, offBy)
		st.Pons = vPons
	}
	for k, v := range sum {
		st.Summary[k] = v
	}
	st.SkipStab = true
	return nil
}

func oltWorkerSNMPWalk(ctx context.Context, st *oltWorkerExecState, step oltcollect.Step) error {
	oid := strings.TrimSpace(step.OID)
	if oid == "" && step.OIDField != "" {
		oid = st.Profile.OIDForField(step.OIDField)
	}
	oid = cleanOltSNMPOID(oid)
	if oid == "" {
		return fmt.Errorf("OID em falta no passo snmp_walk")
	}
	rows, trunc, note := oltWorkerWalkRoot(ctx, st.Host, st.Community, oid)
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

func oltWorkerTelnet(ctx context.Context, st *oltWorkerExecState, step oltcollect.Step) error {
	cmd := strings.TrimSpace(step.Command)
	if cmd == "" {
		return fmt.Errorf("comando telnet em falta")
	}
	tu, tp, te := "", "", ""
	if st.Pool != nil {
		var telUser, telPass, telEnable *string
		_ = st.Pool.QueryRow(ctx, `SELECT telnet_user, telnet_password, telnet_enable FROM settings_connection_defaults WHERE id=1`).
			Scan(&telUser, &telPass, &telEnable)
		if telUser != nil {
			tu = strings.TrimSpace(*telUser)
		}
		if telPass != nil {
			tp = strings.TrimSpace(*telPass)
		}
		if telEnable != nil {
			te = strings.TrimSpace(*telEnable)
		}
	}
	if tu == "" || tp == "" {
		st.Summary["telnet_note"] = "credenciais telnet não configuradas em Definições → Ligação"
		return fmt.Errorf("credenciais telnet não configuradas")
	}
	telTO := st.TelnetTO
	if telTO <= 0 {
		telTO = 90 * time.Second
	}
	tel := probing.TelnetRunCommand(ctx, probing.TelnetRunParams{
		Host: st.Host, Port: "23", Timeout: telTO,
		User: tu, Password: tp, Enable: te,
		Command: cmd, PreCommands: step.PreCommands, MaxReadBytes: 220000,
	})
	if !tel.OK {
		return fmt.Errorf("telnet: %s", tel.Error)
	}
	if strings.TrimSpace(step.Parser) != oltcollect.ParserZteGponOnuState {
		return nil
	}
	rows := zteparse.ParseShowGponOnuState(tel.Output)
	st.Summary["zte_telnet_onu_state_count"] = len(rows)
	if len(rows) == 0 {
		return nil
	}
	var ponRows []map[string]any
	if t, ok := st.Summary["zte_pon_status_table"].([]map[string]any); ok {
		ponRows = t
	}
	statusByIf := oltWorkerZTEPonStatusByIfIndex(ponRows)
	ifNameByIndex := map[int]string{}
	if st.Pool != nil {
		var ifRaw []byte
		if err := st.Pool.QueryRow(ctx, `SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1`, st.DeviceID).Scan(&ifRaw); err == nil && len(ifRaw) > 0 {
			ifRows := snmpifparse.BuildIfTable(oltWorkerWalkJSONToVars(ifRaw))
			for _, r := range ifRows {
				lb := strings.TrimSpace(r.DisplayName)
				if lb == "" {
					lb = strings.TrimSpace(r.IfName)
				}
				ifNameByIndex[r.IfIndex] = lb
			}
		}
	}
	st.Pons = make([]map[string]any, 0, len(rows))
	for _, pr := range rows {
		ponSt := "unknown"
		for ifIdx, lb := range ifNameByIndex {
			if oltWorkerZTEPonIDFromIfLabel(lb) == strings.TrimSpace(pr.Pon) {
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
	st.SkipStab = true
	return nil
}

func oltWorkerDatacomBuildPons(st *oltWorkerExecState) error {
	var onuRows []map[string]any
	if t, ok := st.Summary["datacom_onu_online_table"].([]map[string]any); ok {
		onuRows = t
	}
	if len(onuRows) == 0 {
		return fmt.Errorf("datacom_build_pons: tabela datacom_onu_online_table vazia")
	}
	dRows := oltWorkerDatacomPonRows(onuRows)
	if len(dRows) == 0 {
		return fmt.Errorf("datacom_build_pons: nenhuma PON derivada")
	}
	st.Pons = dRows
	st.Summary["datacom_pon_rows_count"] = len(dRows)
	st.SkipStab = true
	return nil
}

func oltWorkerIfMibMergePons(ctx context.Context, st *oltWorkerExecState) error {
	if st.Pool == nil {
		return fmt.Errorf("base de dados indisponível")
	}
	var ifRaw []byte
	if err := st.Pool.QueryRow(ctx, `SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1`, st.DeviceID).Scan(&ifRaw); err != nil || len(ifRaw) == 0 {
		return fmt.Errorf("sem snapshot IF-MIB")
	}
	ifRows := snmpifparse.BuildIfTable(oltWorkerWalkJSONToVars(ifRaw))
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, oltWorkerWalkJSONToVars(ifRaw))
	derivedPons, sumPatch := oltifderive.DeriveFromIfRows(ifRows, optMap)
	if len(derivedPons) == 0 {
		return fmt.Errorf("IF-MIB não produziu PONs")
	}
	merged := oltifderive.MergePonRowsForIfaceRefresh(st.Pons, derivedPons)
	st.Pons = merged
	st.Summary["if_mib_merge_applied"] = true
	for k, v := range sumPatch {
		st.Summary[k] = v
	}
	return nil
}

func oltWorkerStabilizePons(ctx context.Context, st *oltWorkerExecState) error {
	if st.SkipStab || st.Pool == nil {
		return nil
	}
	var prevSnapPons, prevSnapSum []byte
	_ = st.Pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]'), COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, st.DeviceID).
		Scan(&prevSnapPons, &prevSnapSum)
	prevMaps := oltifderive.PonsJSONToMaps(prevSnapPons)
	prevSumm := oltifderive.SummaryJSONBytesToMap(prevSnapSum)
	incomplete := len(st.Pons) < len(prevMaps) && len(prevMaps) > 0
	stabMaps, stabPatch := oltifderive.StabilizePonSnapshotRows(prevMaps, st.Pons, prevSumm, incomplete)
	st.Pons = stabMaps
	for k, v := range stabPatch {
		st.Summary[k] = v
	}
	return nil
}

func cleanOltSNMPOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

func oltWorkerBoolParam(params map[string]any, key string, def bool) bool {
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
	default:
		return def
	}
}

func oltWorkerWalkRoot(ctx context.Context, host, community, root string) ([]map[string]any, bool, string) {
	root = cleanOltSNMPOID(root)
	if root == "" {
		return nil, false, ""
	}
	walk, trunc, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: host, Port: 161, Community: community, RootOID: root,
		Version: "2c", Timeout: 22 * time.Second, Retries: 0, MaxRows: 24000,
	})
	out := make([]map[string]any, 0, len(walk))
	for _, v := range walk {
		oid := cleanOltSNMPOID(v.OID)
		suffix := strings.TrimPrefix(strings.TrimPrefix(oid, root), ".")
		row := map[string]any{"oid": oid, "suffix": suffix, "type": v.Type, "value": v.Value}
		if n, err := strconv.Atoi(strings.TrimSpace(v.Value)); err == nil {
			row["value_int"] = n
		}
		out = append(out, row)
	}
	return out, trunc, note
}

func oltWorkerWalkJSONToVars(raw []byte) []probing.SNMPVar {
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	out := make([]probing.SNMPVar, 0, len(arr))
	for _, row := range arr {
		oid, _ := row["oid"].(string)
		if strings.HasPrefix(oid, "__netquasar.") {
			continue
		}
		val := fmt.Sprint(row["value"])
		typ, _ := row["type"].(string)
		out = append(out, probing.SNMPVar{OID: oid, Value: val, Type: typ})
	}
	return out
}

func oltWorkerZTEPonStatusByIfIndex(ponRows []map[string]any) map[int]string {
	out := map[int]string{}
	for _, r := range ponRows {
		sfx := oltWorkerAnyString(r["suffix"])
		parts := strings.Split(sfx, ".")
		if len(parts) == 0 {
			continue
		}
		ifIdx, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		if err != nil || ifIdx <= 0 {
			continue
		}
		if v, ok := r["value_int"].(int); ok {
			switch v {
			case 1:
				out[ifIdx] = "up"
			case 2:
				out[ifIdx] = "down"
			default:
				out[ifIdx] = strconv.Itoa(v)
			}
		}
	}
	return out
}

func oltWorkerZTEPonIDFromIfLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if c := oltifderive.PonCompactFromPhy(s, s); c != "" {
		return c
	}
	return ""
}

func oltWorkerDatacomPonRows(rows []map[string]any) []map[string]any {
	type agg struct {
		key, name, status string
		total, online, offline int
	}
	byKey := map[string]*agg{}
	for _, r := range rows {
		sfx := strings.TrimSpace(oltWorkerAnyString(r["suffix"]))
		if sfx == "" {
			continue
		}
		parts := strings.Split(sfx, ".")
		col, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		key := strings.Join(parts[1:], ".")
		if key == "" {
			key = parts[0]
		}
		a := byKey[key]
		if a == nil {
			a = &agg{key: key}
			byKey[key] = a
		}
		switch col {
		case 2:
			a.name = strings.TrimSpace(oltWorkerAnyString(r["value"]))
		case 3:
			if v, ok := r["value_int"].(int); ok {
				a.total = v
			}
		case 5:
			if v, ok := r["value_int"].(int); ok {
				a.online = v
			}
		case 6:
			if v, ok := r["value_int"].(int); ok {
				a.offline = v
			}
		}
	}
	out := make([]map[string]any, 0, len(byKey))
	for _, a := range byKey {
		if a.total <= 0 && (a.online > 0 || a.offline > 0) {
			a.total = a.online + a.offline
		}
		if a.offline == 0 && a.total > a.online {
			a.offline = a.total - a.online
		}
		name := strings.TrimSpace(a.name)
		if name == "" {
			name = "PON-" + a.key
		}
		out = append(out, map[string]any{
			"id": a.key, "name": name,
			"onu_total": a.total, "onu_online": a.online, "onu_offline": a.offline,
			"status": a.status, "source_slice": "datacom_snmp_table",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return oltWorkerAnyString(out[i]["name"]) < oltWorkerAnyString(out[j]["name"])
	})
	return out
}

func oltWorkerAnyString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
