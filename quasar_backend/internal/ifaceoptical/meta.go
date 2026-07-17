package ifaceoptical

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

// MetaOID registo meta em interface_snapshots com potências ópticas (SNMP e/ou Telnet).
const MetaOID = "__netquasar.optical_ports"

// Port potência óptica associada a uma interface (por ifIndex e/ou nome).
type Port struct {
	IfIndex        int      `json:"if_index,omitempty"`
	Name           string   `json:"name,omitempty"`
	TxDBm          *float64 `json:"tx_dbm,omitempty"`
	RxDBm          *float64 `json:"rx_dbm,omitempty"`
	TemperatureC   *float64 `json:"temperature_c,omitempty"`
	SupplyVoltageV *float64 `json:"supply_voltage_v,omitempty"`
	BiasCurrentMA  *float64 `json:"bias_current_ma,omitempty"`
}

// AppendMeta acrescenta/substitui o bloco óptico no array do snapshot.
func AppendMeta(arr []map[string]any, ports []Port) []map[string]any {
	if len(ports) == 0 {
		return arr
	}
	b, err := json.Marshal(ports)
	if err != nil {
		return arr
	}
	out := make([]map[string]any, 0, len(arr)+1)
	for _, row := range arr {
		if strings.TrimSpace(str(row["oid"])) == MetaOID {
			continue
		}
		out = append(out, row)
	}
	out = append(out, map[string]any{
		"oid":   MetaOID,
		"value": string(b),
		"type":  "meta",
	})
	return out
}

// ParseMetaFromWalkJSON extrai portas ópticas do JSON de interface_snapshots.
func ParseMetaFromWalkJSON(raw []byte) []Port {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil {
		return nil
	}
	for _, row := range arr {
		if strings.TrimSpace(str(row["oid"])) != MetaOID {
			continue
		}
		return decodePortsValue(row["value"])
	}
	return nil
}

func decodePortsValue(v any) []Port {
	switch x := v.(type) {
	case string:
		var ports []Port
		if json.Unmarshal([]byte(x), &ports) != nil {
			return nil
		}
		return ports
	case []byte:
		var ports []Port
		if json.Unmarshal(x, &ports) != nil {
			return nil
		}
		return ports
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var ports []Port
		if json.Unmarshal(b, &ports) != nil {
			return nil
		}
		return ports
	}
}

// PortsFromTelnet extrai RX/TX (e extras) dos campos telnet_sfp_* do perfil.
func PortsFromTelnet(out mikrotikcollect.TelnetCollectOutput) []Port {
	byName := map[string]*Port{}
	ensure := func(name string) *Port {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		key := strings.ToLower(mikrotikcollect.NormalizeNxosIfName(name))
		if key == "" {
			key = strings.ToLower(name)
		}
		if p, ok := byName[key]; ok {
			return p
		}
		p := &Port{Name: mikrotikcollect.NormalizeNxosIfName(name)}
		if p.Name == "" {
			p.Name = name
		}
		byName[key] = p
		return p
	}
	merge := func(metricKey, field string, set func(*Port, float64)) {
		fr, ok := out.Fields[metricKey]
		if !ok || !fr.OK || fr.Value == nil {
			return
		}
		items, ok := fr.Value.([]map[string]any)
		if !ok {
			// JSON round-trip pode devolver []any
			if arr, ok2 := fr.Value.([]any); ok2 {
				for _, it := range arr {
					m, ok3 := it.(map[string]any)
					if !ok3 {
						continue
					}
					iface := str(m["interface"])
					if iface == "" {
						iface = str(m["name"])
					}
					p := ensure(iface)
					if p == nil {
						continue
					}
					if f, ok4 := parseFloatAny(m[field]); ok4 {
						set(p, f)
					}
				}
			}
			return
		}
		for _, m := range items {
			iface := str(m["interface"])
			if iface == "" {
				iface = str(m["name"])
			}
			p := ensure(iface)
			if p == nil {
				continue
			}
			if f, ok := parseFloatAny(m[field]); ok {
				set(p, f)
			}
			// vendor bundle may carry extra fields on same row
			if field == "sfp-vendor-name" {
				continue
			}
		}
	}
	merge("telnet_sfp_rx_power", "sfp-rx-power", func(p *Port, f float64) { p.RxDBm = &f })
	merge("telnet_sfp_tx_power", "sfp-tx-power", func(p *Port, f float64) { p.TxDBm = &f })
	merge("telnet_sfp_temperature", "sfp-temperature", func(p *Port, f float64) { p.TemperatureC = &f })
	merge("telnet_sfp_voltage", "sfp-supply-voltage", func(p *Port, f float64) { p.SupplyVoltageV = &f })
	merge("telnet_sfp_bias_current", "sfp-tx-bias-current", func(p *Port, f float64) { p.BiasCurrentMA = &f })

	outPorts := make([]Port, 0, len(byName))
	for _, p := range byName {
		if p.TxDBm == nil && p.RxDBm == nil && p.TemperatureC == nil {
			continue
		}
		outPorts = append(outPorts, *p)
	}
	return outPorts
}

// ResolveIfIndexes preenche IfIndex cruzando nomes com IF-MIB.
func ResolveIfIndexes(ports []Port, rows []snmpifparse.IfRow) []Port {
	if len(ports) == 0 || len(rows) == 0 {
		return ports
	}
	out := make([]Port, len(ports))
	copy(out, ports)
	for i := range out {
		if out[i].IfIndex > 0 {
			continue
		}
		if idx, ok := matchNameToIfIndex(out[i].Name, rows); ok {
			out[i].IfIndex = idx
		}
	}
	return out
}

func matchNameToIfIndex(name string, rows []snmpifparse.IfRow) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	cands := mikrotikcollect.NxosIfNameAliases(name)
	cands = append(cands, name)
	seen := map[string]struct{}{}
	for _, c := range cands {
		k := strings.ToLower(strings.TrimSpace(c))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		for _, r := range rows {
			for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
				if strings.EqualFold(strings.TrimSpace(h), c) {
					return r.IfIndex, true
				}
			}
		}
	}
	if idx, ok := matchPortKeyToIfIndex(name, rows); ok {
		return idx, true
	}
	// fallback: contains
	low := strings.ToLower(mikrotikcollect.NormalizeNxosIfName(name))
	if low == "" {
		low = strings.ToLower(name)
	}
	for _, r := range rows {
		for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
			hn := strings.ToLower(strings.TrimSpace(h))
			if hn == "" {
				continue
			}
			if strings.Contains(hn, low) || strings.Contains(low, hn) {
				return r.IfIndex, true
			}
		}
	}
	return 0, false
}

// MergeIntoOpticalMap junta meta Telnet ao mapa SNMP (meta preenche lacunas; não apaga SNMP).
func MergeIntoOpticalMap(opt map[int]snmpmikrotik.OpticalPower, ports []Port) map[int]snmpmikrotik.OpticalPower {
	if opt == nil {
		opt = map[int]snmpmikrotik.OpticalPower{}
	}
	for _, p := range ports {
		if p.IfIndex <= 0 {
			continue
		}
		cur := opt[p.IfIndex]
		if cur.TxDBm == nil && p.TxDBm != nil {
			cur.TxDBm = p.TxDBm
		}
		if cur.RxDBm == nil && p.RxDBm != nil {
			cur.RxDBm = p.RxDBm
		}
		if cur.TemperatureC == nil && p.TemperatureC != nil {
			cur.TemperatureC = p.TemperatureC
		}
		if cur.SupplyVoltageV == nil && p.SupplyVoltageV != nil {
			cur.SupplyVoltageV = p.SupplyVoltageV
		}
		if cur.BiasCurrentMA == nil && p.BiasCurrentMA != nil {
			cur.BiasCurrentMA = p.BiasCurrentMA
		}
		opt[p.IfIndex] = cur
	}
	return opt
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(strings.Trim(strings.TrimSpace(stringify(v)), `"`))
	}
}

func stringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseFloatAny(v any) (float64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(x, ",", "."))
		s = strings.TrimSuffix(strings.ToLower(s), "dbm")
		s = strings.TrimSuffix(s, "c")
		s = strings.TrimSuffix(s, "v")
		s = strings.TrimSuffix(s, "ma")
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	default:
		return parseFloatAny(str(v))
	}
}
