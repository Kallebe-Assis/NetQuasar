package mikrotikcollect

import (
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const OpticalTableBaseOID = "1.3.6.1.4.1.14988.1.1.19.1.1"

// Colunas mtxrOpticalTable (…19.1.1.{col}.{idx}) — confirmado em walk RouterOS.
const (
	OptColName          = 2
	OptColRxLoss        = 3
	OptColTxFault       = 4
	OptColWavelength    = 5
	OptColTemperature   = 6
	OptColSupplyVoltage = 7
	OptColTxBias        = 8
	OptColTxPower       = 9
	OptColRxPower       = 10
)

// OpticalPortRow porta SFP parseada com divisores aplicados.
type OpticalPortRow struct {
	Index        int      `json:"index"`
	Name         string   `json:"name,omitempty"`
	RxLoss       *bool    `json:"rx_loss,omitempty"`
	TxFault      *bool    `json:"tx_fault,omitempty"`
	WavelengthNm *float64 `json:"wavelength_nm,omitempty"`
	TemperatureC *float64 `json:"temperature_c,omitempty"`
	SupplyVoltageV *float64 `json:"supply_voltage_v,omitempty"`
	BiasCurrentMA  *float64 `json:"bias_current_ma,omitempty"`
	TxDBm        *float64 `json:"tx_dbm,omitempty"`
	RxDBm        *float64 `json:"rx_dbm,omitempty"`
}

func isOpticalSectionKey(key string) bool {
	return strings.HasPrefix(key, "optical_")
}

func anyOpticalEnabledForCatalog(c MetricsConfig, catalog []CatalogEntry) bool {
	for _, e := range catalog {
		if e.Section != "optical" {
			continue
		}
		def, ok := c[e.Key]
		if ok && def.Enabled && strings.TrimSpace(def.OID) != "" {
			return true
		}
	}
	return false
}

// OpticalWalkRoot devolve um único walk para toda a tabela óptica quando qualquer métrica óptica está activa.
func OpticalWalkRoot(c MetricsConfig) string {
	return OpticalWalkRootForCatalog(c, MetricCatalog)
}

func OpticalWalkRootForCatalog(c MetricsConfig, catalog []CatalogEntry) string {
	if !anyOpticalEnabledForCatalog(c, catalog) {
		return ""
	}
	if def, ok := c["optical_table"]; ok && def.Enabled && strings.TrimSpace(def.OID) != "" {
		return strings.TrimSpace(def.OID)
	}
	return OpticalTableBaseOID
}

func parseOpticalCell(oid string) (col, idx int, ok bool) {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	const prefix = OpticalTableBaseOID + "."
	if !strings.HasPrefix(oid, prefix) {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(oid, prefix)
	parts := strings.Split(rest, ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	col, err1 := strconv.Atoi(parts[0])
	idx, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || col < 1 || idx < 1 {
		return 0, 0, false
	}
	return col, idx, true
}

func trimSNMPStr(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func parseNumericRaw(s string) (float64, bool) {
	s = trimSNMPStr(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func applyDivisorFloat(raw float64, div int) float64 {
	if div <= 1 {
		return raw
	}
	return raw / float64(div)
}

func parseBoolInt(s string) *bool {
	n, ok := parseNumericRaw(s)
	if !ok {
		return nil
	}
	v := n != 0
	return &v
}

// ParseOpticalPorts agrega walk mtxrOptical com divisores do perfil.
func ParseOpticalPorts(vars []probing.SNMPVar, profile MetricsConfig) []OpticalPortRow {
	div := func(key string, fallback int) int {
		def, ok := profile[key]
		if ok && def.ValueDivisor > 0 {
			return def.ValueDivisor
		}
		if e, ok := CatalogEntryFor(key); ok && e.DefaultDivisor > 0 {
			return e.DefaultDivisor
		}
		return fallback
	}
	divRx := div("optical_rx_power", 1000)
	divTx := div("optical_tx_power", 1000)
	divVolt := div("optical_supply_voltage", 1000)
	divWave := div("optical_wavelength", 100)
	divTemp := div("optical_temperature", 1)
	divBias := div("optical_bias_current", 1)

	byIdx := map[int]*OpticalPortRow{}
	for _, v := range vars {
		col, idx, ok := parseOpticalCell(v.OID)
		if !ok {
			continue
		}
		row := byIdx[idx]
		if row == nil {
			row = &OpticalPortRow{Index: idx}
			byIdx[idx] = row
		}
		switch col {
		case OptColName:
			row.Name = trimSNMPStr(v.Value)
		case OptColRxLoss:
			row.RxLoss = parseBoolInt(v.Value)
		case OptColTxFault:
			row.TxFault = parseBoolInt(v.Value)
		case OptColWavelength:
			if n, ok := parseNumericRaw(v.Value); ok && n != 0 {
				f := applyDivisorFloat(n, divWave)
				row.WavelengthNm = &f
			}
		case OptColTemperature:
			if n, ok := parseNumericRaw(v.Value); ok && n != 0 {
				f := applyDivisorFloat(n, divTemp)
				row.TemperatureC = &f
			}
		case OptColSupplyVoltage:
			if n, ok := parseNumericRaw(v.Value); ok && n != 0 {
				f := applyDivisorFloat(n, divVolt)
				row.SupplyVoltageV = &f
			}
		case OptColTxBias:
			if n, ok := parseNumericRaw(v.Value); ok && n != 0 {
				f := applyDivisorFloat(n, divBias)
				row.BiasCurrentMA = &f
			}
		case OptColTxPower:
			if f, ok := parseOpticalDbm(v.Value, divTx); ok {
				row.TxDBm = &f
			}
		case OptColRxPower:
			if f, ok := parseOpticalDbm(v.Value, divRx); ok {
				row.RxDBm = &f
			}
		}
	}
	out := make([]OpticalPortRow, 0, len(byIdx))
	for i := 1; i <= len(byIdx)+20; i++ {
		if row, ok := byIdx[i]; ok {
			out = append(out, *row)
		}
	}
	if len(out) == 0 {
		for _, row := range byIdx {
			out = append(out, *row)
		}
	}
	return out
}

func parseOpticalDbm(s string, div int) (float64, bool) {
	n, ok := parseNumericRaw(s)
	if !ok {
		return 0, false
	}
	switch int64(n) {
	case 0, 2147483647, -2147483648, 65535, 4294967295:
		return 0, false
	}
	if div <= 0 {
		div = 1000
	}
	return n / float64(div), true
}

func transformWalkVars(vars []probing.SNMPVar, div int) []probing.SNMPVar {
	if div <= 1 {
		return vars
	}
	out := make([]probing.SNMPVar, len(vars))
	for i, v := range vars {
		out[i] = v
		if n, ok := parseNumericRaw(v.Value); ok {
			out[i].Value = formatScaled(n, div)
		}
	}
	return out
}

func formatScaled(n float64, div int) string {
	f := applyDivisorFloat(n, div)
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

func EffectiveDivisor(def MetricDef, entry CatalogEntry) int {
	if def.ValueDivisor > 0 {
		return def.ValueDivisor
	}
	if entry.DefaultDivisor > 0 {
		return entry.DefaultDivisor
	}
	return 1
}

func parseOpticalColumnOnly(vars []probing.SNMPVar, col, div int) []OpticalPortRow {
	var out []OpticalPortRow
	for _, v := range vars {
		c, idx, ok := parseOpticalCell(v.OID)
		if !ok || c != col {
			continue
		}
		row := OpticalPortRow{Index: idx}
		switch col {
		case OptColName:
			row.Name = trimSNMPStr(v.Value)
		case OptColRxLoss:
			row.RxLoss = parseBoolInt(v.Value)
		case OptColTxFault:
			row.TxFault = parseBoolInt(v.Value)
		case OptColWavelength:
			if n, ok := parseNumericRaw(v.Value); ok {
				f := applyDivisorFloat(n, div)
				row.WavelengthNm = &f
			}
		case OptColTemperature:
			if n, ok := parseNumericRaw(v.Value); ok {
				f := applyDivisorFloat(n, div)
				row.TemperatureC = &f
			}
		case OptColSupplyVoltage:
			if n, ok := parseNumericRaw(v.Value); ok {
				f := applyDivisorFloat(n, div)
				row.SupplyVoltageV = &f
			}
		case OptColTxBias:
			if n, ok := parseNumericRaw(v.Value); ok {
				f := applyDivisorFloat(n, div)
				row.BiasCurrentMA = &f
			}
		case OptColTxPower:
			if f, ok := parseOpticalDbm(v.Value, div); ok {
				row.TxDBm = &f
			}
		case OptColRxPower:
			if f, ok := parseOpticalDbm(v.Value, div); ok {
				row.RxDBm = &f
			}
		}
		out = append(out, row)
	}
	return out
}

func filterOpticalPortsByColumn(ports []OpticalPortRow, col int) []OpticalPortRow {
	if col <= 0 {
		return ports
	}
	out := make([]OpticalPortRow, 0, len(ports))
	for _, p := range ports {
		r := OpticalPortRow{Index: p.Index, Name: p.Name}
		switch col {
		case OptColName:
			r.Name = p.Name
		case OptColRxLoss:
			r.RxLoss = p.RxLoss
		case OptColTxFault:
			r.TxFault = p.TxFault
		case OptColWavelength:
			r.WavelengthNm = p.WavelengthNm
		case OptColTemperature:
			r.TemperatureC = p.TemperatureC
		case OptColSupplyVoltage:
			r.SupplyVoltageV = p.SupplyVoltageV
		case OptColTxBias:
			r.BiasCurrentMA = p.BiasCurrentMA
		case OptColTxPower:
			r.TxDBm = p.TxDBm
		case OptColRxPower:
			r.RxDBm = p.RxDBm
		}
		out = append(out, r)
	}
	return out
}
