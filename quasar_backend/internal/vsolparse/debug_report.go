package vsolparse

import (
	"sort"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// SnmpDebugReport painel de diagnóstico.
type SnmpDebugReport struct {
	GeneratedAt string           `json:"generated_at"`
	Host        string           `json:"host,omitempty"`
	WalkNote    string           `json:"walk_note,omitempty"`
	Sections    []DebugSection   `json:"sections"`
	FinalPons   []map[string]any `json:"final_pons,omitempty"`
	IFMeta      map[string]any   `json:"if_mib_meta,omitempty"`
}

type DebugSection struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	OIDRoot  string           `json:"oid_root"`
	RowCount int              `json:"row_count"`
	Stats    map[string]any   `json:"stats,omitempty"`
	Rows     []map[string]any `json:"rows"`
}

// BuildSnmpDebugReport a partir da coleta sequencial.
func BuildSnmpDebugReport(host string, coll OLTCollect, ifMeta map[string]any, ifPons []map[string]any, finalPons []map[string]any) SnmpDebugReport {
	onBy, offBy := OnlineOfflineByPon(coll.Vars)
	rep := SnmpDebugReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        host,
		WalkNote:    coll.Note,
		IFMeta:      ifMeta,
		FinalPons:   finalPons,
	}
	stepRows := make([]map[string]any, len(coll.Steps))
	for i, s := range coll.Steps {
		stepRows[i] = map[string]any{
			"passo": s.Name, "pedidos": s.OIDsRequested, "respostas": s.OIDsAnswered,
			"elapsed_ms": s.ElapsedMs, "erro": s.Error,
		}
	}
	rep.Sections = append(rep.Sections, DebugSection{
		ID: "passos", Title: "Coleta snmpwalk (gOnuAuthList)", OIDRoot: OIDGOnuAuthList,
		RowCount: len(coll.Vars), Stats: map[string]any{"vars_total": len(coll.Vars)}, Rows: stepRows,
	})
	rep.Sections = append(rep.Sections,
		debugSection(coll.Vars, "onu_online", "Online / Offline (4.1.8)", OIDGOnuAuthList+"."+FieldOnline,
			func(t, c int) bool { return t == 4 && c == 8 }, 400,
			func(pon, onu int, val string) map[string]any {
				st := intFromVal(val)
				lbl := "offline"
				if OnuOnlineFromSta(st) {
					lbl = "online"
				}
				return map[string]any{"pon": pon, "onu": onu, "valor": st, "estado": lbl}
			}),
		debugSection(coll.Vars, "model", "Modelo (2.1.6)", OIDGOnuAuthList+"."+FieldModel,
			func(t, c int) bool { return t == 2 && c == 6 }, 200,
			func(pon, onu int, val string) map[string]any {
				return map[string]any{"pon": pon, "onu": onu, "modelo": val}
			}),
		debugSection(coll.Vars, "rx", "RX dBm (3.1.7)", OIDGOnuAuthList+"."+FieldRx,
			func(t, c int) bool { return t == 3 && c == 7 }, 200,
			func(pon, onu int, val string) map[string]any {
				return map[string]any{"pon": pon, "onu": onu, "rx_dbm": val}
			}),
	)
	rep.Sections = append(rep.Sections, DebugSection{
		ID: "merge_counts", Title: "Contagem 4.1.8 por PON", OIDRoot: OIDGOnuAuthList + "." + FieldOnline,
		RowCount: len(onBy) + len(offBy),
		Stats:    map[string]any{"online_by_pon": onBy, "offline_by_pon": offBy},
		Rows:     buildPonCountRows(onBy, offBy),
	})
	if len(ifPons) > 0 {
		rep.Sections = append(rep.Sections, DebugSection{
			ID: "if_mib_pons", Title: "IF-MIB por PON", OIDRoot: "IF-MIB",
			RowCount: len(ifPons), Rows: limitMaps(ifPons, 40),
		})
	}
	return rep
}

func debugSection(vars []probing.SNMPVar, id, title, root string, match func(int, int) bool, max int, mk func(int, int, string) map[string]any) DebugSection {
	rows := make([]map[string]any, 0, max)
	total := 0
	for _, v := range vars {
		t, col, pon, onu, ok := parseSuffix(v.OID)
		if !ok || !match(t, col) {
			continue
		}
		total++
		if len(rows) < max {
			rows = append(rows, mk(pon, onu, normalizeVSOLString(v.Value)))
		}
	}
	return DebugSection{ID: id, Title: title, OIDRoot: root, RowCount: total, Stats: map[string]any{"parsed_rows": total}, Rows: rows}
}

func buildPonCountRows(onBy, offBy map[int]int) []map[string]any {
	seen := map[int]struct{}{}
	for p := range onBy {
		seen[p] = struct{}{}
	}
	for p := range offBy {
		seen[p] = struct{}{}
	}
	keys := make([]int, 0, len(seen))
	for p := range seen {
		keys = append(keys, p)
	}
	sort.Ints(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, p := range keys {
		out = append(out, map[string]any{
			"pon": p, "online_4.1.8": onBy[p], "offline_4.1.8": offBy[p],
		})
	}
	return out
}

func limitMaps(in []map[string]any, max int) []map[string]any {
	if len(in) <= max {
		return in
	}
	out := make([]map[string]any, max)
	copy(out, in[:max])
	return out
}

func DebugReportToMap(rep SnmpDebugReport) map[string]any {
	secs := make([]any, len(rep.Sections))
	for i, s := range rep.Sections {
		secs[i] = map[string]any{
			"id": s.ID, "title": s.Title, "oid_root": s.OIDRoot,
			"row_count": s.RowCount, "stats": s.Stats, "rows": s.Rows,
		}
	}
	return map[string]any{
		"generated_at": rep.GeneratedAt, "host": rep.Host, "walk_note": rep.WalkNote,
		"sections": secs, "final_pons": rep.FinalPons, "if_mib_meta": rep.IFMeta,
	}
}
