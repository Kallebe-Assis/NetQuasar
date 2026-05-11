package vsolparse

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OIDGOnuAuthList raiz MIB ONU (enterprise VSOL 37950) — gOnuAuthList e tabelas associadas.
const OIDGOnuAuthList = "1.3.6.1.4.1.37950.1.1.6.1.1"

// OIDLegacyGOnuOptical árvore alternativa comentada no MIB (1.1.5.12.2.1.8) — alguns firmwares só respondem aqui para óptica.
const OIDLegacyGOnuOptical = "1.3.6.1.4.1.37950.1.1.5.12.2.1.8"

const maxOnuRowsOut = 2500

func phaseLabel(v int) string {
	switch v {
	case 0:
		return "logging"
	case 1:
		return "los"
	case 2:
		return "syncMib"
	case 3:
		return "working"
	case 4:
		return "dyingGasp"
	case 5:
		return "authFail"
	case 6:
		return "offLine"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func authModeLabel(v int) string {
	switch v {
	case 1:
		return "sn"
	case 2:
		return "pw"
	case 3:
		return "hpw"
	case 4:
		return "snPpw"
	case 5:
		return "snPhpw"
	case 6:
		return "loid"
	case 7:
		return "loidPpw"
	case 10:
		return "loidPhpw"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func intFromVal(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// onuAcc acumula colunas por (pon, onu).
type onuAcc struct {
	pon, onu                    int
	adminSta, omccSta, phaseSta int
	profileName, authInfo       string
	authMode                    int
	temp, volt, bias, tx, rx   string
	vendor, version, sn, model  string
	detailAdmin, detailOp       int
}

func parseSuffix(oid string) (tbl, col, pon, onu int, ok bool) {
	base := OIDGOnuAuthList + "."
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	if !strings.HasPrefix(oid, base) {
		return 0, 0, 0, 0, false
	}
	rest := strings.TrimPrefix(oid, base)
	parts := strings.Split(rest, ".")
	// Forma típica: T.1.col.pon.onu (MIB MG-SOFT). Alguns agentes omitem o «1» da entrada: T.col.pon.onu
	if len(parts) >= 5 {
		t := intFromVal(parts[0])
		sub := intFromVal(parts[1])
		c := intFromVal(parts[2])
		ponIx := intFromVal(parts[3])
		onuIx := intFromVal(parts[4])
		if t >= 1 && t <= 4 && (sub == 1 || sub == 0) && c >= 1 && ponIx >= 1 && onuIx >= 1 {
			return t, c, ponIx, onuIx, true
		}
	}
	if len(parts) == 4 {
		t := intFromVal(parts[0])
		c := intFromVal(parts[1])
		ponIx := intFromVal(parts[2])
		onuIx := intFromVal(parts[3])
		if t >= 1 && t <= 4 && c >= 1 && ponIx >= 1 && onuIx >= 1 {
			return t, c, ponIx, onuIx, true
		}
	}
	return 0, 0, 0, 0, false
}

// normalizeVSOLString limpa OCTET STRING (zeros à direita, espaços).
func normalizeVSOLString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "\x00")
	return strings.TrimSpace(s)
}

// parseOpticalFloat tenta interpretar potências / temperaturas em texto MIB VSOL.
func parseOpticalFloat(s string) (float64, bool) {
	s = normalizeVSOLString(s)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", ".")
	if i := strings.IndexFunc(s, func(r rune) bool {
		return !(unicode.IsDigit(r) || r == '.' || r == '-' || r == '+')
	}); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	f, err := strconv.ParseFloat(s, 64)
	if err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f, true
	}
	return 0, false
}

// parseLegacyOpticalSuffix interpreta OIDs sob OIDLegacyGOnuOptical (entrada 1, coluna, PON, ONU).
func parseLegacyOpticalSuffix(oid string) (col, pon, onu int, ok bool) {
	base := OIDLegacyGOnuOptical + "."
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	if !strings.HasPrefix(oid, base) {
		return 0, 0, 0, false
	}
	rest := strings.TrimPrefix(oid, base)
	parts := strings.Split(rest, ".")
	if len(parts) != 4 {
		return 0, 0, 0, false
	}
	if intFromVal(parts[0]) != 1 {
		return 0, 0, 0, false
	}
	col = intFromVal(parts[1])
	pon = intFromVal(parts[2])
	onu = intFromVal(parts[3])
	if col < 3 || col > 7 || pon < 1 || onu < 1 {
		return 0, 0, 0, false
	}
	return col, pon, onu, true
}

// VsolOnuRowsFromSummaryBlob extrai vsol_onu_rows do summary JSON (incl. double-encoding em string).
func VsolOnuRowsFromSummaryBlob(sum []byte) []any {
	var sumObj map[string]any
	if len(sum) == 0 || json.Unmarshal(sum, &sumObj) != nil {
		return nil
	}
	return unwrapVsolOnuRowsValue(sumObj["vsol_onu_rows"])
}

func unwrapVsolOnuRowsValue(v any) []any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []any:
		return x
	case string:
		var arr []any
		if json.Unmarshal([]byte(x), &arr) == nil {
			return arr
		}
	case []byte:
		var arr []any
		if json.Unmarshal(x, &arr) == nil {
			return arr
		}
	case json.RawMessage:
		var arr []any
		if json.Unmarshal(x, &arr) == nil {
			return arr
		}
	}
	return nil
}

// FromSNMPWalk agrega um walk sob OIDGOnuAuthList (tabelas gOnuSta/Auth/Optical/Detail).
func FromSNMPWalk(vars []probing.SNMPVar) (summary map[string]any, pons []map[string]any, onuRows []map[string]any) {
	acc := make(map[string]*onuAcc)
	get := func(pon, onu int) *onuAcc {
		k := fmt.Sprintf("%d.%d", pon, onu)
		if acc[k] == nil {
			acc[k] = &onuAcc{pon: pon, onu: onu}
		}
		return acc[k]
	}

	for _, v := range vars {
		t, col, pon, onu, ok := parseSuffix(v.OID)
		if !ok {
			if lc, lp, lu, lok := parseLegacyOpticalSuffix(v.OID); lok {
				t, col, pon, onu = 3, lc, lp, lu
				ok = true
			}
		}
		if !ok {
			continue
		}
		row := get(pon, onu)
		val := normalizeVSOLString(v.Value)
		switch t {
		case 1: // gOnuStaInfo
			switch col {
			case 3:
				row.adminSta = intFromVal(val)
			case 4:
				row.omccSta = intFromVal(val)
			case 5:
				row.phaseSta = intFromVal(val)
			}
		case 2: // gOnuAuthInfo
			switch col {
			case 3:
				row.profileName = val
			case 4:
				row.authMode = intFromVal(val)
			case 5:
				row.authInfo = val
			}
		case 3: // gOnuOpticalInfo
			switch col {
			case 3:
				row.temp = val
			case 4:
				row.volt = val
			case 5:
				row.bias = val
			case 6:
				row.tx = val
			case 7:
				row.rx = val
			}
		case 4: // gOnuDetailInfo
			switch col {
			case 3:
				row.vendor = val
			case 4:
				row.version = val
			case 5:
				row.sn = val
			case 6:
				row.detailAdmin = intFromVal(val)
			case 13:
				row.detailOp = intFromVal(val)
			case 17:
				row.model = val
			}
		}
	}

	ponSet := make(map[int]struct{})
	for _, o := range acc {
		ponSet[o.pon] = struct{}{}
	}
	ponOnline := make(map[int]int)
	ponTotal := make(map[int]int)
	for _, o := range acc {
		ponTotal[o.pon]++
		if o.phaseSta == 3 {
			ponOnline[o.pon]++
		}
	}

	var ponIDs []int
	for p := range ponSet {
		ponIDs = append(ponIDs, p)
	}
	for i := 0; i < len(ponIDs); i++ {
		for j := i + 1; j < len(ponIDs); j++ {
			if ponIDs[j] < ponIDs[i] {
				ponIDs[i], ponIDs[j] = ponIDs[j], ponIDs[i]
			}
		}
	}

	txSum := map[int]float64{}
	txN := map[int]int{}
	for _, o := range acc {
		if f, ok := parseOpticalFloat(o.tx); ok {
			txSum[o.pon] += f
			txN[o.pon]++
		}
	}

	pons = make([]map[string]any, 0, len(ponIDs))
	for _, pid := range ponIDs {
		tot := ponTotal[pid]
		on := ponOnline[pid]
		off := tot - on
		if off < 0 {
			off = 0
		}
		compactID := oltifderive.VsolMibPonCompactID(pid)
		row := map[string]any{
			"id":           compactID,
			"name":         fmt.Sprintf("GPON0/%d", pid),
			"onu_total":    tot,
			"onu_online":   on,
			"onu_offline":  off,
			"status":       "vsol_snmp",
			"vendor_model": "VSOL enterprise MIB (gOnuAuthList)",
		}
		if n := txN[pid]; n > 0 {
			row["tx_dbm"] = txSum[pid] / float64(n)
		}
		pons = append(pons, row)
	}

	for _, o := range acc {
		m := map[string]any{
			"pon":          o.pon,
			"onu":          o.onu,
			"profile_name": o.profileName,
			"auth_mode":    authModeLabel(o.authMode),
			"admin_sta":    o.adminSta,
			"omcc_sta":     o.omccSta,
			"phase_sta":    phaseLabel(o.phaseSta),
			"temp":         o.temp,
			"voltage":      o.volt,
			"bias":         o.bias,
			"tx_pwr":       o.tx,
			"rx_pwr":       o.rx,
			"vendor":       o.vendor,
			"version":      o.version,
			"serial":       o.sn,
			"model":        o.model,
			"detail_admin": o.detailAdmin,
			"detail_op":    o.detailOp,
		}
		onuRows = append(onuRows, m)
	}
	sort.Slice(onuRows, func(i, j int) bool {
		pi, _ := onuRows[i]["pon"].(int)
		pj, _ := onuRows[j]["pon"].(int)
		if pi != pj {
			return pi < pj
		}
		oi, _ := onuRows[i]["onu"].(int)
		oj, _ := onuRows[j]["onu"].(int)
		return oi < oj
	})

	truncated := false
	if len(onuRows) > maxOnuRowsOut {
		onuRows = onuRows[:maxOnuRowsOut]
		truncated = true
	}

	totOnu := len(acc)
	onlineAll := 0
	for _, o := range acc {
		if o.phaseSta == 3 {
			onlineAll++
		}
	}
	raw, _ := json.Marshal(onuRows)
	var arr []any
	_ = json.Unmarshal(raw, &arr)
	summary = map[string]any{
		"source":              "vsol_olt_mib_snmp",
		"vsol_mib":            "OLT gOnuAuthList (VSOL enterprise)",
		"vsol_onu_count":      totOnu,
		"vsol_onu_online":     onlineAll,
		"vsol_onu_offline":    totOnu - onlineAll,
		"vsol_pon_count":      len(ponIDs),
		"vsol_rows_truncated": truncated,
		"vsol_onu_rows":       arr,
	}

	return summary, pons, onuRows
}
