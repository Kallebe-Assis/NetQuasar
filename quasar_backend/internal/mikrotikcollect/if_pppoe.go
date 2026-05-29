package mikrotikcollect

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const (
	IFColInOctets  = 10
	IFColOutOctets = 16
	IFTableBaseOID = "1.3.6.1.2.1.2.2.1"
)

// PPPoESessionRow sessão PPPoE activa (interface dinâmica no IF-MIB).
type PPPoESessionRow struct {
	IfIndex         int    `json:"if_index"`
	Name            string `json:"name"`
	OperStatus      int    `json:"oper_status,omitempty"`
	OperStatusLabel string `json:"oper_status_label,omitempty"`
	InOctets        uint64 `json:"in_octets,omitempty"`
	OutOctets       uint64 `json:"out_octets,omitempty"`
}

func parseSNMPUint64(v string) uint64 {
	n, err := strconv.ParseUint(trimSNMPStr(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func isPPPoEInterfaceName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(n, "pppoe") || strings.HasPrefix(n, "<pppoe")
}

// ParsePPPoESessionsFromIFMib agrega ifTable e filtra interfaces cujo nome contém «pppoe».
func ParsePPPoESessionsFromIFMib(vars []probing.SNMPVar) []PPPoESessionRow {
	byIdx := map[int]*PPPoESessionRow{}
	for _, v := range vars {
		col, idx, ok := parseIfMibCell(v.OID, 0)
		if !ok {
			continue
		}
		row := byIdx[idx]
		if row == nil {
			row = &PPPoESessionRow{IfIndex: idx}
			byIdx[idx] = row
		}
		switch col {
		case IFColDescr:
			row.Name = trimSNMPStr(v.Value)
		case IFColOperStatus:
			if n, err := strconv.Atoi(trimSNMPStr(v.Value)); err == nil {
				row.OperStatus = n
				row.OperStatusLabel = ifOperStatusLabel(n)
			}
		case IFColInOctets:
			row.InOctets = parseSNMPUint64(v.Value)
		case IFColOutOctets:
			row.OutOctets = parseSNMPUint64(v.Value)
		}
	}
	out := make([]PPPoESessionRow, 0)
	for _, row := range byIdx {
		if row.Name == "" || !isPPPoEInterfaceName(row.Name) {
			continue
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].Name < out[j].Name
	})
	return out
}
