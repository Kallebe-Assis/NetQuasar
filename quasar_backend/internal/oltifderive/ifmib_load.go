package oltifderive

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

// Limites alinhados ao refresh de interfaces (handlers interface_snmp_walk).
const (
	IFMibWalkMaxRows  = 42_000
	IFXMibWalkMaxRows = 48_000
)

// IfMibDataset resultado de walk ao vivo e/ou snapshot em BD.
type IfMibDataset struct {
	Rows       []snmpifparse.IfRow
	Vars       []probing.SNMPVar
	OnuIfaces  int
	PonWithOnu int
	Truncated  bool
	Source     string // live | snapshot | live+snapshot
	Note       string
}

// OIDs mínimos para inventário OLT (evita walk completo ifTable ~22 col × N ifIndex).
const (
	oidIfDescr      = "1.3.6.1.2.1.2.2.1.2"
	oidIfOperStatus = "1.3.6.1.2.1.2.2.1.8"
	oidIfName       = "1.3.6.1.2.1.31.1.1.1.1"
)

// WalkIFMib walk direcionado ifDescr + ifOperStatus + ifName (completa todas as ONUs com poucas linhas SNMP).
func WalkIFMib(ctx context.Context, host, community string, budget time.Duration) ([]probing.SNMPVar, bool, string) {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" {
		return nil, false, ""
	}
	if budget <= 0 {
		budget = 120 * time.Second
	}
	if budget > 240*time.Second {
		budget = 240 * time.Second
	}
	perWalk := budget / 3
	if perWalk < 18*time.Second {
		perWalk = 18 * time.Second
	}
	if perWalk > 60*time.Second {
		perWalk = 60 * time.Second
	}

	var merged []probing.SNMPVar
	trunc := false
	var notes []string
	roots := []struct {
		oid   string
		max   int
		label string
	}{
		{oidIfDescr, 25000, "ifDescr"},
		{oidIfOperStatus, 25000, "ifOperStatus"},
		{oidIfName, 25000, "ifName"},
	}
	for _, w := range roots {
		wCtx, cancel := context.WithTimeout(ctx, perWalk)
		v, t, n := probing.SNMPWalk(wCtx, probing.SNMPWalkParams{
			Host: host, Port: 161, Community: community, RootOID: w.oid,
			Version: "2c", Timeout: perWalk, Retries: 1, MaxRows: w.max,
		})
		cancel()
		merged = append(merged, v...)
		if t {
			trunc = true
			notes = append(notes, w.label+":trunc")
		}
		if strings.TrimSpace(n) != "" {
			notes = append(notes, w.label+":"+n)
		}
	}
	return merged, trunc, strings.TrimSpace(strings.Join(notes, "; "))
}

// DatasetFromSnapshot usa apenas interface_snapshots em BD (sem SNMP ao vivo).
func DatasetFromSnapshot(snapshotRaw []byte) IfMibDataset {
	ds := parseIfSnapshotJSON(snapshotRaw)
	if ds.Source == "" && len(ds.Rows) > 0 {
		ds.Source = "snapshot"
	}
	ds.PonWithOnu = CountPonWithOnuFromRows(ds.Rows)
	return ds
}

// LoadIFMibDataset walk ao vivo + união com snapshot (nunca descarta o conjunto mais completo).
func LoadIFMibDataset(ctx context.Context, host, community string, budget time.Duration, snapshotRaw []byte) IfMibDataset {
	var live, snap IfMibDataset
	if host != "" && community != "" {
		vars, trunc, note := WalkIFMib(ctx, host, community, budget)
		if len(vars) > 0 {
			live.Rows = snmpifparse.BuildIfTable(vars)
			live.Vars = vars
			live.OnuIfaces = CountOnuIfaceRows(live.Rows)
			live.PonWithOnu = CountPonWithOnuFromRows(live.Rows)
			live.Truncated = trunc
			live.Source = "live"
			live.Note = note
		}
	}
	if len(snapshotRaw) > 0 {
		snap = parseIfSnapshotJSON(snapshotRaw)
		if snap.Source == "" && len(snap.Rows) > 0 {
			snap.Source = "snapshot"
		}
		snap.PonWithOnu = CountPonWithOnuFromRows(snap.Rows)
	}
	if len(live.Rows) == 0 {
		return snap
	}
	if len(snap.Rows) == 0 {
		return live
	}
	mergedRows := MergeIfRowSets(live.Rows, snap.Rows)
	out := IfMibDataset{
		Rows:       mergedRows,
		Vars:       live.Vars,
		OnuIfaces:  CountOnuIfaceRows(mergedRows),
		PonWithOnu: CountPonWithOnuFromRows(mergedRows),
		Truncated:  live.Truncated && snap.Truncated,
		Source:     "live+snapshot",
		Note:       strings.TrimSpace(live.Note + "; merged_snapshot"),
	}
	if len(live.Vars) == 0 {
		out.Vars = snap.Vars
	}
	return out
}

// CountPonWithOnuFromRows número de PONs distintas com pelo menos uma interface ONU no IF-MIB.
func CountPonWithOnuFromRows(rows []snmpifparse.IfRow) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		c, _, ok := PonCompactFromOnuIface(ifaceLabel(r.IfName, r.Descr), r.Descr)
		if ok && c != "" {
			seen[c] = struct{}{}
		}
	}
	return len(seen)
}

// MergeIfRowSets une linhas por ifIndex (prefere nome GPON e operStatus não-zero).
func MergeIfRowSets(primary, secondary []snmpifparse.IfRow) []snmpifparse.IfRow {
	byIdx := map[int]snmpifparse.IfRow{}
	order := make([]int, 0, len(primary)+len(secondary))
	add := func(r snmpifparse.IfRow) {
		if r.IfIndex <= 0 {
			return
		}
		prev, ok := byIdx[r.IfIndex]
		if !ok {
			byIdx[r.IfIndex] = r
			order = append(order, r.IfIndex)
			return
		}
		byIdx[r.IfIndex] = mergeIfRowPair(prev, r)
	}
	for _, r := range primary {
		add(r)
	}
	for _, r := range secondary {
		add(r)
	}
	out := make([]snmpifparse.IfRow, 0, len(order))
	for _, ix := range order {
		out = append(out, byIdx[ix])
	}
	return out
}

func mergeIfRowPair(a, b snmpifparse.IfRow) snmpifparse.IfRow {
	out := a
	if strings.TrimSpace(b.IfName) != "" {
		out.IfName = b.IfName
	}
	if strings.TrimSpace(b.Descr) != "" {
		out.Descr = b.Descr
	}
	if strings.TrimSpace(b.DisplayName) != "" {
		if strings.TrimSpace(out.DisplayName) == "" || strings.Contains(strings.ToUpper(b.DisplayName), "GPON") {
			out.DisplayName = b.DisplayName
		}
	}
	if b.OperStatus != 0 {
		out.OperStatus = b.OperStatus
	}
	if b.AdminStatus != 0 {
		out.AdminStatus = b.AdminStatus
	}
	if out.DisplayName == "" {
		out.DisplayName = ifaceLabel(out.IfName, out.Descr)
	}
	return out
}

// CountOnuIfaceRows conta interfaces GPONxxONUyy no IF-MIB.
func CountOnuIfaceRows(rows []snmpifparse.IfRow) int {
	n := 0
	for _, r := range rows {
		if _, _, ok := PonCompactFromOnuIface(ifaceLabel(r.IfName, r.Descr), r.Descr); ok {
			n++
		}
	}
	return n
}

func parseIfSnapshotJSON(raw []byte) IfMibDataset {
	var out IfMibDataset
	vars := walkJSONVars(raw)
	if len(vars) > 0 {
		out.Rows = snmpifparse.BuildIfTable(vars)
		out.Vars = vars
		out.Truncated = snapshotWalkTruncated(raw)
	} else {
		out.Rows = ifRowsFromInterfaceTableJSON(raw)
	}
	out.OnuIfaces = CountOnuIfaceRows(out.Rows)
	return out
}

func walkJSONVars(raw []byte) []probing.SNMPVar {
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
		Type  string `json:"type"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	out := make([]probing.SNMPVar, 0, len(arr))
	for _, v := range arr {
		oid := strings.TrimSpace(v.OID)
		if strings.HasPrefix(oid, "__netquasar.") {
			continue
		}
		out = append(out, probing.SNMPVar{OID: v.OID, Value: v.Value, Type: v.Type})
	}
	return out
}

func snapshotWalkTruncated(raw []byte) bool {
	var arr []struct {
		OID   string `json:"oid"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return false
	}
	for _, v := range arr {
		if strings.TrimSpace(v.OID) == "__netquasar.walk" && strings.TrimSpace(v.Value) == "truncated" {
			return true
		}
	}
	return false
}

func ifRowsFromInterfaceTableJSON(raw []byte) []snmpifparse.IfRow {
	var wrap map[string]json.RawMessage
	if json.Unmarshal(raw, &wrap) != nil {
		return nil
	}
	tab, ok := wrap["interface_table"]
	if !ok {
		return nil
	}
	var arr []map[string]any
	if json.Unmarshal(tab, &arr) != nil {
		return nil
	}
	return ifRowsFromInterfaceTable(arr)
}

func ifRowsFromInterfaceTable(tab []map[string]any) []snmpifparse.IfRow {
	out := make([]snmpifparse.IfRow, 0, len(tab))
	for _, m := range tab {
		idx := jsonInt(m["if_index"])
		if idx <= 0 {
			continue
		}
		oper := jsonInt(m["oper_status_n"])
		if oper == 0 {
			if s, ok := m["oper_status"].(string); ok && strings.EqualFold(strings.TrimSpace(s), "up") {
				oper = 1
			}
		}
		descr, _ := m["descr"].(string)
		name, _ := m["if_name"].(string)
		disp, _ := m["display_name"].(string)
		if strings.TrimSpace(disp) == "" {
			disp = strings.TrimSpace(name)
			if disp == "" {
				disp = descr
			}
		}
		out = append(out, snmpifparse.IfRow{
			IfIndex: idx, Descr: descr, IfName: name, DisplayName: disp,
			OperStatus: oper, AdminStatus: jsonInt(m["admin_status_n"]),
		})
	}
	return out
}

func jsonInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}
