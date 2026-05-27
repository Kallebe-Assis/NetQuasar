package vsolparse

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// Campos MIB (sufixo após OIDGOnuAuthList).
const (
	FieldOnline = "4.1.8" // 1=online; 0, 2 ou outro=offline
	FieldModel  = "2.1.6"
	FieldSerial = "2.1.5"
	FieldRx     = "3.1.7"
	FieldTemp   = "3.1.3"
	FieldVolt   = "3.1.4"
)

const snmpWalkMaxRows = 35000

// StepLog resultado de um passo de coleta.
type StepLog struct {
	Name          string        `json:"name"`
	OIDsRequested int           `json:"oids_requested"`
	OIDsAnswered  int           `json:"oids_answered"`
	Elapsed       time.Duration `json:"-"`
	ElapsedMs     int64         `json:"elapsed_ms"`
	Error         string        `json:"error,omitempty"`
}

// OLTCollect resultado agregado da coleta VSOL V1600.
type OLTCollect struct {
	Vars      []probing.SNMPVar
	Steps     []StepLog
	Note      string
	Failed    bool
	Truncated bool
}

// CollectTimeout orçamento HTTP para um snmpwalk na tabela inteira.
func CollectTimeout(nOnu int, _ bool) time.Duration {
	sec := 120
	if nOnu > 150 {
		sec = 180
	}
	if nOnu > 400 {
		sec = 300
	}
	if sec > 600 {
		sec = 600
	}
	return time.Duration(sec) * time.Second
}

// CollectOLT faz snmpwalk em OIDGOnuAuthList (tabela MIB completa) e parseia no fim.
func CollectOLT(ctx context.Context, host, community string, refs []OnuRef, _ bool) OLTCollect {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" {
		return OLTCollect{Note: "host ou community vazio", Failed: true}
	}
	if len(refs) == 0 {
		return OLTCollect{Note: "sem_indices_onu (IF-MIB sem GPONxxONUyy)", Failed: true}
	}

	t0 := time.Now()
	walkTO := walkTimeoutFromCtx(ctx)
	vars, trunc, errNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host:      host,
		Community: community,
		RootOID:   OIDGOnuAuthList,
		Version:   "2c",
		Timeout:   walkTO,
		Retries:   1,
		MaxRows:   snmpWalkMaxRows,
	})
	vars = normalizeWalkVars(vars)

	onlineN := len(OnlineStaByRef(refs, vars))
	st := StepLog{
		Name:          "snmpwalk_gOnuAuthList",
		OIDsRequested: len(refs),
		OIDsAnswered:  onlineN,
		Elapsed:       time.Since(t0),
	}
	st.ElapsedMs = st.Elapsed.Milliseconds()

	var notes []string
	if errNote != "" {
		st.Error = errNote
	}
	if trunc {
		notes = append(notes, "walk_truncado")
	}
	if onlineN < len(refs) {
		notes = append(notes, fmt.Sprintf("online_4.1.8:%d/%d", onlineN, len(refs)))
	}
	notes = append(notes, fmt.Sprintf("vars_walk:%d", len(vars)))

	label := "snmpwalk_completo"
	if len(notes) > 0 {
		label += ";" + strings.Join(notes, ";")
	}
	if st.Error != "" {
		label += ";" + st.Error
	}

	coll := finishCollect(vars, []StepLog{st}, label)
	coll.Truncated = trunc
	return coll
}

func walkTimeoutFromCtx(ctx context.Context) time.Duration {
	const max = 300 * time.Second
	if ctx == nil {
		return max
	}
	if dl, ok := ctx.Deadline(); ok {
		left := time.Until(dl) - 2*time.Second
		if left < 30*time.Second {
			return 30 * time.Second
		}
		if left > max {
			return max
		}
		return left
	}
	return max
}

func normalizeWalkVars(vars []probing.SNMPVar) []probing.SNMPVar {
	out := make([]probing.SNMPVar, 0, len(vars))
	for _, v := range vars {
		oid := probing.NormalizeSNMPOID(v.OID)
		if oid == "" || !strings.HasPrefix(oid, OIDGOnuAuthList) {
			continue
		}
		if !probing.SNMPValueUsable(v.Value) {
			continue
		}
		out = append(out, probing.SNMPVar{OID: oid, Type: v.Type, Value: v.Value})
	}
	return out
}

// OnlineStepComplete true quando o walk trouxe 4.1.8 para todas as ONUs do IF-MIB.
func OnlineStepComplete(coll OLTCollect) bool {
	if len(coll.Steps) == 0 {
		return false
	}
	s := coll.Steps[0]
	if coll.Truncated {
		return false
	}
	return s.OIDsRequested > 0 && s.OIDsAnswered >= s.OIDsRequested
}

func finishCollect(vars []probing.SNMPVar, steps []StepLog, label string) OLTCollect {
	failed := false
	var notes []string
	for _, s := range steps {
		if s.Error != "" {
			failed = true
			notes = append(notes, s.Name+":"+s.Error)
		}
		if s.OIDsRequested > 0 && s.OIDsAnswered < s.OIDsRequested {
			failed = true
			notes = append(notes, fmt.Sprintf("%s:faltam_%d_de_%d", s.Name, s.OIDsRequested-s.OIDsAnswered, s.OIDsRequested))
		}
	}
	note := label
	if len(notes) > 0 {
		note = label + ";" + strings.Join(notes, ";")
	}
	return OLTCollect{Vars: vars, Steps: steps, Note: note, Failed: failed}
}

// BuildOnuTable monta uma linha por ref IF-MIB; online só com 4.1.8 válido.
func BuildOnuTable(refs []OnuRef, vars []probing.SNMPVar, prev []map[string]any, mergePrev bool) []map[string]any {
	onMap := OnlineStaByRef(refs, vars)
	_, _, parsed := FromSNMPWalk(vars, false)
	byKey := map[string]map[string]any{}
	for _, r := range parsed {
		if k := onuRowKey(r); k != "" {
			byKey[k] = r
		}
	}
	rows := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		k := fmt.Sprintf("%d.%d", ref.Pon, ref.Onu)
		row := scaffoldOnuRows([]OnuRef{ref})[0]
		if p, ok := byKey[k]; ok {
			row = copyOnuRow(p)
		}
		if sta, ok := onMap[refKey(ref.Pon, ref.Onu)]; ok {
			row["onu_online_sta"] = sta
			row["online"] = OnuOnlineFromSta(sta)
		} else {
			row["onu_online_sta"] = fieldUnset
			row["online"] = false
		}
		rows = append(rows, row)
	}
	if mergePrev && len(prev) > 0 {
		rows = MergeOnuRowsTelemetry(prev, rows)
	}
	return rows
}

func copyOnuRow(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// CountTelemetryVars contagens por coluna MIB (diagnóstico).
func CountTelemetryVars(vars []probing.SNMPVar) map[string]int {
	out := map[string]int{"online": 0, "model": 0, "serial": 0, "rx": 0, "volt": 0, "temp": 0}
	for _, v := range vars {
		t, col, _, _, ok := parseSuffix(v.OID)
		if !ok {
			continue
		}
		switch t {
		case 2:
			if col == 6 {
				out["model"]++
			}
			if col == 5 {
				out["serial"]++
			}
		case 3:
			if col == 7 {
				out["rx"]++
			}
			if col == 4 {
				out["volt"]++
			}
			if col == 3 {
				out["temp"]++
			}
		case 4:
			if col == 8 {
				out["online"]++
			}
		}
	}
	return out
}

// OnuRowsToJSON serializa para o snapshot.
func OnuRowsToJSON(rows []map[string]any) []any {
	return onuRowsToAny(rows)
}

func scaffoldOnuRows(refs []OnuRef) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		out = append(out, map[string]any{
			"pon": r.Pon, "onu": r.Onu, "phase_sta": "—", "online": false, "onu_online_sta": -1,
		})
	}
	return out
}

func rowsFromVars(vars []probing.SNMPVar) []map[string]any {
	acc := map[string]*onuAcc{}
	get := func(pon, onu int) *onuAcc {
		k := fmt.Sprintf("%d.%d", pon, onu)
		if acc[k] == nil {
			acc[k] = newOnuAcc(pon, onu)
		}
		return acc[k]
	}
	for _, v := range vars {
		t, col, pon, onu, ok := parseSuffix(v.OID)
		if !ok {
			continue
		}
		row := get(pon, onu)
		val := normalizeVSOLString(v.Value)
		switch t {
		case 2:
			switch col {
			case 5:
				row.sn = val
			case 6:
				row.model = val
			}
		case 3:
			switch col {
			case 3:
				row.temp = val
			case 4:
				row.volt = val
			case 7:
				row.rx = val
			}
		case 4:
			if col == 8 {
				row.onuOnlineSta = intFromVal(val)
			}
		}
	}
	return accToOnuRows(acc)
}

func accToOnuRows(acc map[string]*onuAcc) []map[string]any {
	keys := make([]string, 0, len(acc))
	for k := range acc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		o := acc[k]
		if o == nil {
			continue
		}
		out = append(out, map[string]any{
			"pon": o.pon, "onu": o.onu,
			"phase_sta": onuPhaseLabel(o), "online": onuIsOnline(o),
			"onu_online_sta": o.onuOnlineSta,
			"temp": o.temp, "voltage": o.volt, "rx_pwr": o.rx,
			"serial": o.sn, "model": o.model,
		})
	}
	return out
}

func onuRowsToAny(rows []map[string]any) []any {
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil
	}
	var arr []any
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	return arr
}

// ReconcileSummaryFromPons alinha totais do summary com a soma das linhas PON.
func ReconcileSummaryFromPons(summary map[string]any, pons []map[string]any) {
	if summary == nil {
		return
	}
	var tot, on, off int
	for _, p := range pons {
		tot += intVal(p["onu_total"])
		on += intVal(p["onu_online"])
		off += intVal(p["onu_offline"])
	}
	summary["vsol_pon_count"] = len(pons)
	summary["vsol_onu_count"] = tot
	summary["vsol_onu_online"] = on
	summary["vsol_onu_offline"] = off
}

func intVal(v any) int {
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
