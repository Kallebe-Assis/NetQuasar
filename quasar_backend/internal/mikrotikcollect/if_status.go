package mikrotikcollect

import (
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const (
	IFColOperStatus  = 8
	IFColAdminStatus = 7
	IFColDescr       = 2
	IFOperStatusOID  = "1.3.6.1.2.1.2.2.1.8"
	IFAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
)

// InterfaceStatusRow status IF-MIB por interface.
type InterfaceStatusRow struct {
	IfIndex          int    `json:"if_index"`
	OperStatus       int    `json:"oper_status,omitempty"`
	OperStatusLabel  string `json:"oper_status_label,omitempty"`
	AdminStatus      int    `json:"admin_status,omitempty"`
	AdminStatusLabel string `json:"admin_status_label,omitempty"`
	Name             string `json:"name,omitempty"`
}

func ifOperStatusLabel(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	case 4:
		return "unknown"
	case 5:
		return "dormant"
	case 6:
		return "notPresent"
	case 7:
		return "lowerLayerDown"
	default:
		return "other"
	}
}

func ifAdminStatusLabel(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	default:
		return "other"
	}
}

func parseIfMibCell(oid string, expectCol int) (col, ifIndex int, ok bool) {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	const prefix = "1.3.6.1.2.1.2.2.1."
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
	if err1 != nil || err2 != nil || idx < 1 {
		return 0, 0, false
	}
	if expectCol > 0 && col != expectCol {
		return 0, 0, false
	}
	return col, idx, true
}

// ParseInterfaceOperStatus walk ifOperStatus (col 8).
func ParseInterfaceOperStatus(vars []probing.SNMPVar) []InterfaceStatusRow {
	byIdx := map[int]*InterfaceStatusRow{}
	for _, v := range vars {
		col, idx, ok := parseIfMibCell(v.OID, IFColOperStatus)
		if !ok || col != IFColOperStatus {
			col, idx, ok = parseIfMibCell(v.OID, 0)
			if !ok || col != IFColOperStatus {
				continue
			}
		}
		n, err := strconv.Atoi(trimSNMPStr(v.Value))
		if err != nil {
			continue
		}
		row := byIdx[idx]
		if row == nil {
			row = &InterfaceStatusRow{IfIndex: idx}
			byIdx[idx] = row
		}
		row.OperStatus = n
		row.OperStatusLabel = ifOperStatusLabel(n)
	}
	return interfaceRowsSorted(byIdx)
}

// ParseInterfaceAdminStatus walk ifAdminStatus (col 7).
func ParseInterfaceAdminStatus(vars []probing.SNMPVar) []InterfaceStatusRow {
	byIdx := map[int]*InterfaceStatusRow{}
	for _, v := range vars {
		col, idx, ok := parseIfMibCell(v.OID, IFColAdminStatus)
		if !ok {
			col, idx, ok = parseIfMibCell(v.OID, 0)
			if !ok || col != IFColAdminStatus {
				continue
			}
		}
		n, err := strconv.Atoi(trimSNMPStr(v.Value))
		if err != nil {
			continue
		}
		row := byIdx[idx]
		if row == nil {
			row = &InterfaceStatusRow{IfIndex: idx}
			byIdx[idx] = row
		}
		row.AdminStatus = n
		row.AdminStatusLabel = ifAdminStatusLabel(n)
	}
	return interfaceRowsSorted(byIdx)
}

func interfaceRowsSorted(byIdx map[int]*InterfaceStatusRow) []InterfaceStatusRow {
	out := make([]InterfaceStatusRow, 0, len(byIdx))
	for i := 1; i <= len(byIdx)+500; i++ {
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
