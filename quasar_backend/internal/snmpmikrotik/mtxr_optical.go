package snmpmikrotik

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

// Referência: data/mibs/MIKROTIK/MIKROTIK.mib — mtxrOpticalTable (…14988.1.1.19.1.1),
// mtxrOpticalTxPower / mtxrOpticalRxPower = IDiv1000 (milésimos de dBm).
// ObjectIndex (índice da linha) «não está relacionado» com números da consola (MIB); por isso
// cruzamos com mtxrInterfaceStatsName (…14.1.1.2), que em RouterOS coincide com o ifName.

var reMtxrSfpDigits = regexp.MustCompile(`^sfp(\d+)$`)

const (
	// Walk direto na mtxrOpticalTable (evita truncar em subárvores irmãs antes de chegar TX/RX).
	DefaultOpticalWalkRoot = "1.3.6.1.4.1.14988.1.1.19.1.1"
	// Apenas a coluna mtxrInterfaceStatsName (…14.1.1.2.<instância>) — nomes de interface alinhados ao IF-MIB.
	DefaultInterfaceStatsNameWalkRoot = "1.3.6.1.4.1.14988.1.1.14.1.1.2"

	colOpticalIndex = 1
	colName         = 2
	colTx           = 9
	colRx           = 10
	colTemperature  = 6
	colSupplyV      = 7
	colTxBias       = 8
)

const mtxrOpticalMIBPrefix = "1.3.6.1.4.1.14988.1.1.19"

type OpticalPower struct {
	TxDBm          *float64
	RxDBm          *float64
	TemperatureC   *float64
	SupplyVoltageV *float64
	BiasCurrentMA  *float64
}

type mtxrModule struct {
	name            string
	objectIndexHint int
	tx              *float64
	rx              *float64
	temperatureC    *float64
	supplyVoltageV  *float64
	biasCurrentMA   *float64
}

// parseMtxrCell devolve (coluna, índice da linha mtxr) para OIDs ...19.1.1.<col>.<mtxIdx>.
func parseMtxrCell(oid string) (col, mtxrIdx int, ok bool) {
	oid = strings.TrimSpace(oid)
	oid = strings.TrimPrefix(oid, ".")
	if !strings.Contains(oid, mtxrOpticalMIBPrefix) {
		return 0, 0, false
	}
	parts := strings.Split(oid, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	n := len(parts)
	mtxrIdx, err1 := strconv.Atoi(parts[n-1])
	col, err2 := strconv.Atoi(parts[n-2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if col != colOpticalIndex && col != colName && col != colTx && col != colRx &&
		col != colTemperature && col != colSupplyV && col != colTxBias {
		return 0, 0, false
	}
	if mtxrIdx < 1 {
		return 0, 0, false
	}
	return col, mtxrIdx, true
}

const ifaceStatsNamePrefix = "1.3.6.1.4.1.14988.1.1.14.1.1.2."

// parseInterfaceStatsNameOID — mtxrInterfaceStatsName (.14.1.1.2.<inst>).
func parseInterfaceStatsNameOID(oid string) (instance int, ok bool) {
	oid = strings.TrimSpace(oid)
	oid = strings.TrimPrefix(oid, ".")
	if !strings.HasPrefix(oid, ifaceStatsNamePrefix) {
		return 0, false
	}
	suf := oid[len(ifaceStatsNamePrefix):]
	if suf == "" {
		return 0, false
	}
	// índice simples Integer32
	n, err := strconv.Atoi(suf)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func trimSNMPDisplayString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	return strings.TrimSpace(s)
}

func mtxrNameCandidates(label string) []string {
	label = strings.ToLower(trimSNMPDisplayString(label))
	if label == "" {
		return nil
	}
	var cands []string
	cands = append(cands, "sfp-"+label)
	cands = append(cands, "sfp-sfp"+strings.TrimPrefix(label, "sfp"))
	if m := reMtxrSfpDigits.FindStringSubmatch(label); m != nil {
		cands = append(cands, "sfp-sfpplus"+m[1])
	}
	seen := map[string]struct{}{}
	var uniq []string
	for _, c := range cands {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		k := strings.ToLower(c)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, c)
	}
	return uniq
}

func matchExactIfName(full string, rows []snmpifparse.IfRow) (int, bool) {
	full = strings.TrimSpace(full)
	if full == "" {
		return 0, false
	}
	for _, r := range rows {
		for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
			h = strings.TrimSpace(h)
			if h != "" && strings.EqualFold(h, full) {
				return r.IfIndex, true
			}
		}
	}
	return 0, false
}

// matchMtxrLabelToIfIndex associa mtxrOpticalName curto (ex.: sfpplus1) ao ifIndex.
func matchMtxrLabelToIfIndex(label string, rows []snmpifparse.IfRow) (int, bool) {
	lb := strings.ToLower(trimSNMPDisplayString(label))
	uniq := mtxrNameCandidates(lb)
	if len(uniq) == 0 {
		return 0, false
	}
	for _, r := range rows {
		for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
			hn := strings.ToLower(strings.TrimSpace(h))
			if hn == "" {
				continue
			}
			for _, c := range uniq {
				if hn == strings.ToLower(c) {
					return r.IfIndex, true
				}
			}
		}
	}
	if len(lb) >= 4 {
		for _, r := range rows {
			for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
				hn := strings.ToLower(strings.TrimSpace(h))
				if hn == "" {
					continue
				}
				if strings.Contains(hn, lb) {
					return r.IfIndex, true
				}
			}
		}
	}
	return 0, false
}

func parseStatsIfNameByInstance(vars []probing.SNMPVar) map[int]string {
	out := map[int]string{}
	for _, v := range vars {
		inst, ok := parseInterfaceStatsNameOID(v.OID)
		if !ok {
			continue
		}
		nm := trimSNMPDisplayString(v.Value)
		if nm != "" {
			out[inst] = nm
		}
	}
	return out
}

func resolveMtxrToIfIndex(mtxrIdx int, mod *mtxrModule, statsName map[int]string, rows []snmpifparse.IfRow) (int, bool) {
	if mod.objectIndexHint > 0 {
		for _, r := range rows {
			if r.IfIndex == mod.objectIndexHint {
				return mod.objectIndexHint, true
			}
		}
	}
	if full, ok := statsName[mtxrIdx]; ok && full != "" {
		if i, ok2 := matchExactIfName(full, rows); ok2 {
			return i, true
		}
	}
	if i, ok := matchMtxrLabelToIfIndex(mod.name, rows); ok {
		return i, true
	}
	if mod.name == "" {
		for _, r := range rows {
			if r.IfIndex == mtxrIdx {
				return mtxrIdx, true
			}
		}
	}
	return 0, false
}

// OpticalPowerByIfIndex agrega mtxrOptical e cruza com IF-MIB (e opcionalmente mtxrInterfaceStatsName).
func OpticalPowerByIfIndex(rows []snmpifparse.IfRow, vars []probing.SNMPVar) map[int]OpticalPower {
	statsNames := parseStatsIfNameByInstance(vars)
	byMtxr := map[int]*mtxrModule{}
	for _, v := range vars {
		col, idx, ok := parseMtxrCell(v.OID)
		if !ok {
			continue
		}
		m := byMtxr[idx]
		if m == nil {
			m = &mtxrModule{}
			byMtxr[idx] = m
		}
		switch col {
		case colOpticalIndex:
			if vi, err := strconv.Atoi(trimSNMPDisplayString(v.Value)); err == nil && vi > 0 {
				m.objectIndexHint = vi
			}
		case colName:
			m.name = trimSNMPDisplayString(v.Value)
		case colTx:
			if f, ok := parseMikrotikMilliDbm(v.Value); ok {
				m.tx = &f
			}
		case colRx:
			if f, ok := parseMikrotikMilliDbm(v.Value); ok {
				m.rx = &f
			}
		case colTemperature:
			if f, ok := parseMikrotikGauge(v.Value, 1); ok {
				m.temperatureC = &f
			}
		case colSupplyV:
			if f, ok := parseMikrotikGauge(v.Value, 1000); ok {
				m.supplyVoltageV = &f
			}
		case colTxBias:
			if f, ok := parseMikrotikGauge(v.Value, 1); ok {
				m.biasCurrentMA = &f
			}
		}
	}
	out := map[int]OpticalPower{}
	for mtxrIdx, mod := range byMtxr {
		if mod.tx == nil && mod.rx == nil && mod.temperatureC == nil && mod.supplyVoltageV == nil && mod.biasCurrentMA == nil {
			continue
		}
		ifIdx, ok := resolveMtxrToIfIndex(mtxrIdx, mod, statsNames, rows)
		if !ok {
			continue
		}
		row := out[ifIdx]
		if mod.tx != nil {
			row.TxDBm = mod.tx
		}
		if mod.rx != nil {
			row.RxDBm = mod.rx
		}
		if mod.temperatureC != nil {
			row.TemperatureC = mod.temperatureC
		}
		if mod.supplyVoltageV != nil {
			row.SupplyVoltageV = mod.supplyVoltageV
		}
		if mod.biasCurrentMA != nil {
			row.BiasCurrentMA = mod.biasCurrentMA
		}
		out[ifIdx] = row
	}
	for ix, row := range out {
		if row.TxDBm == nil && row.RxDBm == nil && row.TemperatureC == nil && row.SupplyVoltageV == nil && row.BiasCurrentMA == nil {
			delete(out, ix)
		}
	}
	return out
}

// parseMikrotikMilliDbm — IDiv1000 do MIB (milésimos de dBm).
func parseMikrotikMilliDbm(s string) (float64, bool) {
	return parseMikrotikGauge(s, 1000)
}

func parseMikrotikGauge(s string, div int) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	switch n {
	case 0, 2147483647, -2147483648, 65535, 4294967295:
		return 0, false
	}
	if div <= 0 {
		div = 1
	}
	return float64(n) / float64(div), true
}

// LooksLikeSfpInterface heurística quando a tabela óptica ainda não tem linha.
func LooksLikeSfpInterface(displayName, descr string) bool {
	hay := strings.ToLower(displayName + " " + descr)
	if strings.Contains(hay, "sfp") || strings.Contains(hay, "sfp+") {
		return true
	}
	if strings.Contains(hay, "xfp") || strings.Contains(hay, "qsfp") {
		return true
	}
	return false
}

// IsSfpPort true se houver leitura óptica útil ou nome típico de porta de módulo.
func IsSfpPort(displayName, descr string, p OpticalPower) bool {
	if p.TxDBm != nil || p.RxDBm != nil {
		return true
	}
	return LooksLikeSfpInterface(displayName, descr)
}
