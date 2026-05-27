package vsolparse

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OIDGOnuAuthList raiz MIB ONU (enterprise VSOL 37950) — gOnuAuthList e tabelas associadas.
const OIDGOnuAuthList = "1.3.6.1.4.1.37950.1.1.6.1.1"

// OIDLegacyGOnuOptical árvore alternativa comentada no MIB (1.1.5.12.2.1.8).
const OIDLegacyGOnuOptical = "1.3.6.1.4.1.37950.1.1.5.12.2.1.8"

const maxOnuRowsOut = 2500

const fieldUnset = -1

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
		if v < 0 {
			return "—"
		}
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
		return fieldUnset
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fieldUnset
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
	onuOnlineSta               int // tabela 4 col 8: 4.1.8.pon.onu — 1 online, 0 offline
}

func newOnuAcc(pon, onu int) *onuAcc {
	return &onuAcc{
		pon: pon, onu: onu,
		adminSta: fieldUnset, omccSta: fieldUnset, phaseSta: fieldUnset,
		authMode: fieldUnset, detailAdmin: fieldUnset, detailOp: fieldUnset,
		onuOnlineSta: fieldUnset,
	}
}

// onuIsOnline só com OID 4.1.8: 1=online; 0, 2 ou outro=offline. Sem leitura = não online.
func onuIsOnline(o *onuAcc) bool {
	if o == nil {
		return false
	}
	return OnuOnlineFromSta(o.onuOnlineSta)
}

func onuPhaseLabel(o *onuAcc) string {
	if o.phaseSta == fieldUnset {
		if o.onuOnlineSta == 1 {
			return "working"
		}
		return "—"
	}
	return phaseLabel(o.phaseSta)
}

func parseSuffix(oid string) (tbl, col, pon, onu int, ok bool) {
	base := OIDGOnuAuthList + "."
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	if !strings.HasPrefix(oid, base) {
		return 0, 0, 0, 0, false
	}
	rest := strings.TrimPrefix(oid, base)
	parts := strings.Split(rest, ".")
	// T.1.1.col.pon.onu (firmware com índice de entrada explícito)
	if len(parts) >= 6 {
		t := intFromVal(parts[0])
		sub := intFromVal(parts[1])
		entry := intFromVal(parts[2])
		c := intFromVal(parts[3])
		ponIx := intFromVal(parts[4])
		onuIx := intFromVal(parts[5])
		if t >= 1 && t <= 4 && (sub == 1 || sub == 0) && entry == 1 && c >= 1 && ponIx >= 1 && onuIx >= 1 {
			return t, c, ponIx, onuIx, true
		}
	}
	// T.1.col.pon.onu (MIB MG-SOFT)
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

func looksLikeSerial(s string) bool {
	s = normalizeVSOLString(s)
	if len(s) < 4 {
		return false
	}
	up := strings.ToUpper(s)
	return strings.HasPrefix(up, "MONU") || strings.HasPrefix(up, "HWTC") ||
		strings.HasPrefix(up, "VSOL") || strings.HasPrefix(up, "GPON")
}

func normalizeVSOLString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "\x00")
	return strings.TrimSpace(s)
}

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

// FromSNMPWalk agrega walk SNMP VSOL em contagens por PON e lista de ONUs.
func FromSNMPWalk(vars []probing.SNMPVar, walkTruncated bool) (summary map[string]any, pons []map[string]any, onuRows []map[string]any) {
	acc := make(map[string]*onuAcc)
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
		case 1:
			switch col {
			case 3:
				row.adminSta = intFromVal(val)
			case 4:
				row.omccSta = intFromVal(val)
			case 5:
				row.phaseSta = intFromVal(val)
			}
		case 2:
			// Firmware V1600: 2.1.F.pon.onu — F=1 phase, 3 profile, 4 authMode, 5 serial, 6 model.
			switch col {
			case 1:
				row.phaseSta = intFromVal(val)
			case 3:
				row.profileName = val
			case 4:
				row.authMode = intFromVal(val)
			case 5:
				if strings.TrimSpace(val) != "" {
					row.sn = val
				}
			case 6:
				row.model = val
			}
		case 3:
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
		case 4:
			switch col {
			case 8:
				row.onuOnlineSta = intFromVal(val)
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

	onByPon, offByPon := OnlineOfflineByPon(vars)
	pons = nil

	for _, o := range acc {
		hasOnline := o.onuOnlineSta == 0 || o.onuOnlineSta == 1
		if !hasOnline && strings.TrimSpace(o.sn) == "" && o.phaseSta == fieldUnset &&
			strings.TrimSpace(o.model) == "" && strings.TrimSpace(o.rx) == "" &&
			strings.TrimSpace(o.volt) == "" {
			continue
		}
		onuRows = append(onuRows, map[string]any{
			"pon":          o.pon,
			"onu":          o.onu,
			"profile_name": o.profileName,
			"auth_mode":    authModeLabel(o.authMode),
			"admin_sta":    o.adminSta,
			"omcc_sta":     o.omccSta,
			"phase_sta":    onuPhaseLabel(o),
			"online":       onuIsOnline(o),
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
			"onu_online_sta": o.onuOnlineSta,
		})
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

	rowTruncated := false
	if len(onuRows) > maxOnuRowsOut {
		onuRows = onuRows[:maxOnuRowsOut]
		rowTruncated = true
	}

	onlineAll, offlineAll := 0, 0
	for _, n := range onByPon {
		onlineAll += n
	}
	for _, n := range offByPon {
		offlineAll += n
	}

	raw, _ := json.Marshal(onuRows)
	var arr []any
	_ = json.Unmarshal(raw, &arr)

	summary = map[string]any{
		"source":              "vsol_olt_mib_snmp",
		"vsol_mib":            "OLT gOnuAuthList (VSOL enterprise)",
		"vsol_onu_count":       len(onuRows),
		"vsol_onu_online":      onlineAll,
		"vsol_onu_offline":     offlineAll,
		"vsol_onu_status_rows": onlineAll + offlineAll,
		"vsol_rows_truncated": rowTruncated || walkTruncated,
		"vsol_onu_rows":       arr,
	}
	return summary, pons, onuRows
}
