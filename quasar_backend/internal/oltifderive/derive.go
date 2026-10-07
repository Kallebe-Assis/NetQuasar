package oltifderive

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

func ponSortParts(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := make([]int, 0, 4)
	n := 0
	inNum := false
	for _, r := range s {
		if unicode.IsDigit(r) {
			n = n*10 + int(r-'0')
			inNum = true
			continue
		}
		if inNum {
			parts = append(parts, n)
			n = 0
			inNum = false
		}
	}
	if inNum {
		parts = append(parts, n)
	}
	return parts
}

func lessPonKey(a, b string) bool {
	pa, pb := ponSortParts(a), ponSortParts(b)
	if len(pa) > 0 && len(pb) > 0 {
		n := len(pa)
		if len(pb) < n {
			n = len(pb)
		}
		for i := 0; i < n; i++ {
			if pa[i] == pb[i] {
				continue
			}
			return pa[i] < pb[i]
		}
		if len(pa) != len(pb) {
			return len(pa) < len(pb)
		}
	}
	return strings.ToLower(strings.TrimSpace(a)) < strings.ToLower(strings.TrimSpace(b))
}

type PonAgg struct {
	Compact  string
	Name     string
	Total    int
	Online   int
	Offline  int
	PonAdmin int
	PonOper  int
	PonIfIdx int
}

func ifaceLabel(ifName, descr string) string {
	if strings.TrimSpace(ifName) != "" {
		return strings.TrimSpace(ifName)
	}
	return strings.TrimSpace(descr)
}

// DeriveFromIfRows classifica interfaces OLT e agrega ONUs por PON (chave compacta GPON0/1 → "01").
func DeriveFromIfRows(rows []snmpifparse.IfRow, optByIdx map[int]snmpmikrotik.OpticalPower) ([]map[string]any, map[string]any) {
	byPon := map[string]*PonAgg{}
	ensure := func(compact string) *PonAgg {
		if byPon[compact] == nil {
			byPon[compact] = &PonAgg{Compact: compact, Name: "PON " + compact}
		}
		return byPon[compact]
	}

	for _, r := range rows {
		disp := ifaceLabel(r.IfName, r.Descr)
		k := ClassifyKind(disp, r.Descr)
		switch k {
		case KindPON:
			c := PonCompactFromPhy(disp, r.Descr)
			if c == "" {
				continue
			}
			p := ensure(c)
			p.Name = disp
			p.PonIfIdx = r.IfIndex
			p.PonAdmin = r.AdminStatus
			p.PonOper = r.OperStatus
		case KindONU:
			c, onuN, ok := PonCompactFromOnuIface(disp, r.Descr)
			if !ok || c == "" {
				continue
			}
			p := ensure(c)
			_ = onuN
			p.Total++
			if r.OperStatus == 1 {
				p.Online++
			} else {
				p.Offline++
			}
		default:
			_ = disp
		}
	}

	var keys []string
	for k := range byPon {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessPonKey(keys[i], keys[j]) })

	ponsOut := make([]map[string]any, 0, len(keys))
	onuTotal, onuOn, onuOff := 0, 0, 0
	for _, key := range keys {
		p := byPon[key]
		if p.PonIfIdx == 0 && p.Total == 0 {
			continue
		}
		onuTotal += p.Total
		onuOn += p.Online
		onuOff += p.Offline
		st := "if_mib"
		if p.PonOper == 1 {
			st = "pon_up"
		} else if p.PonIfIdx > 0 {
			st = "pon_down"
		}
		op := optByIdx[p.PonIfIdx]
		row := map[string]any{
			"id":           p.Compact,
			"name":         p.Name,
			"onu_total":    p.Total,
			"onu_online":   p.Online,
			"onu_offline":  p.Offline,
			"status":       st,
			"source_slice": "if_mib_onu",
		}
		if op.TxDBm != nil {
			row["tx_dbm"] = *op.TxDBm
		}
		if op.RxDBm != nil {
			row["rx_dbm"] = *op.RxDBm
		}
		ponsOut = append(ponsOut, row)
	}

	summary := map[string]any{
		"onu_total_if_mib":   onuTotal,
		"onu_online_if_mib":  onuOn,
		"onu_offline_if_mib": onuOff,
		"if_mib_derived":     true,
	}
	return ponsOut, summary
}

// CountOnuPerPonFromIfRows agrega ONUs por PON a partir de ifDescr/ifName «GPON01ONU2» (operStatus 1 = online).
func CountOnuPerPonFromIfRows(rows []snmpifparse.IfRow) []map[string]any {
	byPon := map[string]*PonAgg{}
	ensure := func(compact string) *PonAgg {
		if byPon[compact] == nil {
			byPon[compact] = &PonAgg{Compact: compact, Name: "GPON0/" + trimLeadingZero(compact)}
		}
		return byPon[compact]
	}
	for _, r := range rows {
		disp := ifaceLabel(r.IfName, r.Descr)
		c, _, ok := PonCompactFromOnuIface(disp, r.Descr)
		if !ok || c == "" {
			continue
		}
		p := ensure(c)
		p.Total++
		if r.OperStatus == 1 {
			p.Online++
		} else {
			p.Offline++
		}
	}
	var keys []string
	for k := range byPon {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessPonKey(keys[i], keys[j]) })
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		p := byPon[key]
		out = append(out, map[string]any{
			"id":           p.Compact,
			"name":         p.Name,
			"onu_total":    p.Total,
			"onu_online":   p.Online,
			"onu_offline":  p.Offline,
			"status":       "if_mib_onu",
			"source_slice": "if_mib_onu_count",
		})
	}
	return out
}

func trimLeadingZero(s string) string {
	s = strings.TrimSpace(s)
	for len(s) > 1 && s[0] == '0' {
		s = s[1:]
	}
	return s
}

// BuildPonSnapshotFromIfMIB inventário de portas GPON0/N + contagens ONU por interface ONU.
func BuildPonSnapshotFromIfMIB(rows []snmpifparse.IfRow, optByIdx map[int]snmpmikrotik.OpticalPower) []map[string]any {
	counts := CountOnuPerPonFromIfRows(rows)
	phy := ListPonPhysicalPortsFromIfRows(rows, optByIdx)
	var merged []map[string]any
	if len(counts) == 0 {
		merged = phy
	} else {
		// Contagens primeiro; portas físicas só preenchem PONs sem ONU (evita pon_down zerar totais).
		merged = MergePonRowsForIfaceRefresh(counts, phy)
	}
	out := make([]map[string]any, 0, len(merged))
	for _, row := range merged {
		out = append(out, FinalizePonRowFromIfMIB(row))
	}
	return DedupePonMaps(out)
}

// FinalizePonRowFromIfMIB normaliza totais e estado quando há ONUs contadas via IF-MIB.
func FinalizePonRowFromIfMIB(row map[string]any) map[string]any {
	r := cloneMap(row)
	if rowPickInt(r, "onu_total", "total_onu", "onus", "onus_total", "onu_count") > 0 {
		r["status"] = "if_mib_onu"
		if fmt.Sprint(r["source_slice"]) == "" || fmt.Sprint(r["source_slice"]) == "if_mib_pon_port" {
			r["source_slice"] = "if_mib_onu_count"
		}
	}
	NormalizePonONUCounts(r)
	return r
}

// ListPonPhysicalPortsFromIfRows devolve só interfaces PON físicas (GPON0/N), sem contagem por IF de ONU.
// Usado em OLT VSOL para completar portas sem ONU no MIB enterprise (ex.: 8 PONs, 5 com ONUs).
func ListPonPhysicalPortsFromIfRows(rows []snmpifparse.IfRow, optByIdx map[int]snmpmikrotik.OpticalPower) []map[string]any {
	var out []map[string]any
	for _, r := range rows {
		disp := ifaceLabel(r.IfName, r.Descr)
		if ClassifyKind(disp, r.Descr) != KindPON {
			continue
		}
		c := PonCompactFromPhy(disp, r.Descr)
		if c == "" {
			continue
		}
		st := "pon_down"
		if r.OperStatus == 1 {
			st = "pon_up"
		}
		row := map[string]any{
			"id":           c,
			"name":         disp,
			"onu_total":    0,
			"onu_online":   0,
			"onu_offline":  0,
			"status":       st,
			"source_slice": "if_mib_pon_port",
		}
		if op, ok := optByIdx[r.IfIndex]; ok {
			if op.TxDBm != nil {
				row["tx_dbm"] = *op.TxDBm
			}
			if op.RxDBm != nil {
				row["rx_dbm"] = *op.RxDBm
			}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, _ := out[i]["id"].(string)
		aj, _ := out[j]["id"].(string)
		return lessPonKey(ai, aj)
	})
	return out
}

// PonPortsFromSNMPVars lista portas PON a partir de walk IF-MIB (2.2 + 31).
func PonPortsFromSNMPVars(vars []probing.SNMPVar) []map[string]any {
	if len(vars) == 0 {
		return nil
	}
	ifRows := snmpifparse.BuildIfTable(vars)
	opt := snmpmikrotik.OpticalPowerByIfIndex(ifRows, vars)
	return ListPonPhysicalPortsFromIfRows(ifRows, opt)
}

// AnnotateInterfaceTable adiciona olt_iface_kind e sanitiza octets saturados (32-bit).
func AnnotateInterfaceTable(tab []map[string]any) {
	for _, row := range tab {
		disp, _ := row["display_name"].(string)
		descr, _ := row["descr"].(string)
		k := ClassifyKind(disp, descr)
		row["olt_iface_kind"] = string(k)

		inV, ok1 := row["in_octets"]
		outV, ok2 := row["out_octets"]
		var inI, outI int64
		if ok1 {
			inI = toInt64(inV)
		}
		if ok2 {
			outI = toInt64(outV)
		}
		if Saturated32Counters(inI, outI) {
			row["in_octets"] = nil
			row["out_octets"] = nil
			row["octets_saturated_32bit"] = true
		}
	}
}

// ponKeyForIfaceRow calcula a mesma chave canónica de PON (ver CanonicalPonRowKey) a partir de uma
// linha da tabela GENÉRICA de interfaces (campos display_name/descr, não name/id) — usado por
// DedupeOltInterfaceTablePonRows. Devolve "" para qualquer linha que não seja reconhecida como porta
// PON física em nenhum dos formatos conhecidos (GPON0/N, ZTE, Datacom, ou o nome sintético VSOL
// "PON N").
func ponKeyForIfaceRow(row map[string]any) string {
	disp := strings.TrimSpace(fmt.Sprint(row["display_name"]))
	descr := strings.TrimSpace(fmt.Sprint(row["descr"]))
	if disp == "<nil>" {
		disp = ""
	}
	if descr == "<nil>" {
		descr = ""
	}
	if c := PonCompactFromPhy(disp, descr); c != "" {
		return c
	}
	for _, s := range []string{disp, descr} {
		if sm := reVsolPonName.FindStringSubmatch(s); len(sm) == 2 {
			if n, err := strconv.Atoi(sm[1]); err == nil && n > 0 {
				return VsolMibPonCompactID(n)
			}
		}
	}
	return ""
}

func isEmptyNumeric(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case float64:
		return x == 0
	case int:
		return x == 0
	case int64:
		return x == 0
	case string:
		return strings.TrimSpace(x) == ""
	default:
		return false
	}
}

// mergeOltIfaceRowPair funde duas linhas da tabela genérica de interfaces que representam a MESMA
// porta PON física (ver DedupeOltInterfaceTablePonRows) — prefere o nome estilo "GPONx/y" para
// exibição (mesma regra de preferPonDisplayName) e preenche contadores/estado vazios de uma linha
// com os valores da outra, já que normalmente só uma das duas entradas duplicadas no ifTable do
// equipamento carrega os contadores de tráfego/estado reais.
func mergeOltIfaceRowPair(a, b map[string]any) map[string]any {
	nameA := strings.TrimSpace(fmt.Sprint(a["display_name"]))
	nameB := strings.TrimSpace(fmt.Sprint(b["display_name"]))
	primary, secondary := a, b
	if ponDisplayNameRank(nameB) > ponDisplayNameRank(nameA) {
		primary, secondary = b, a
	}
	out := cloneMap(primary)
	for _, k := range []string{"in_octets", "out_octets", "in_bps", "out_bps", "tx_dbm", "rx_dbm", "speed_bps"} {
		if isEmptyNumeric(out[k]) && !isEmptyNumeric(secondary[k]) {
			out[k] = secondary[k]
		}
	}
	if rowToInt(out["oper_status_n"]) == 0 && rowToInt(secondary["oper_status_n"]) != 0 {
		out["oper_status"] = secondary["oper_status"]
		out["oper_status_n"] = secondary["oper_status_n"]
	}
	if rowToInt(out["admin_status_n"]) == 0 && rowToInt(secondary["admin_status_n"]) != 0 {
		out["admin_status"] = secondary["admin_status"]
		out["admin_status_n"] = secondary["admin_status_n"]
	}
	return out
}

// DedupeOltInterfaceTablePonRows funde linhas duplicadas da MESMA porta PON física na tabela
// genérica de interfaces (usada em Equipamentos > OLT > Interfaces, distinta do resumo PON/ONU que
// já tinha esse dedup via DedupePonMaps). Bug real observado: algumas OLTs (confirmado numa VSOL
// V1600G1) expõem cada porta PON física duas vezes no próprio ifTable SNMP — uma entrada estilo
// "GPON0/1" e outra com o nome sintético "PON 01" — fazendo uma OLT de 8 PONs aparecer com 16
// interfaces. Linhas que não são PON (GE, VLAN, ONU, outras) passam intactas.
func DedupeOltInterfaceTablePonRows(tab []map[string]any) []map[string]any {
	if len(tab) == 0 {
		return tab
	}
	out := make([]map[string]any, 0, len(tab))
	keyToIdx := map[string]int{}
	for _, row := range tab {
		key := ponKeyForIfaceRow(row)
		if key == "" {
			out = append(out, row)
			continue
		}
		if i, ok := keyToIdx[key]; ok {
			out[i] = mergeOltIfaceRowPair(out[i], row)
			continue
		}
		keyToIdx[key] = len(out)
		out = append(out, row)
	}
	return out
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}
