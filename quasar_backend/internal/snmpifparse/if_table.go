package snmpifparse

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

var (
	reIfMib   = regexp.MustCompile(`^\.?1\.3\.6\.1\.2\.1\.2\.2\.1\.(\d+)\.(\d+)$`)
	reIfName  = regexp.MustCompile(`^\.?1\.3\.6\.1\.2\.1\.31\.1\.1\.1\.1\.(\d+)$`)
	reIfHCIn  = regexp.MustCompile(`^\.?1\.3\.6\.1\.2\.1\.31\.1\.1\.1\.6\.(\d+)$`)
	reIfHCOut = regexp.MustCompile(`^\.?1\.3\.6\.1\.2\.1\.31\.1\.1\.1\.10\.(\d+)$`)
)

// IfRow representa uma linha agregada ifTable + ifXTable (nome e contadores 64 bits).
type IfRow struct {
	IfIndex     int    `json:"if_index"`
	Descr       string `json:"descr"`
	IfName      string `json:"if_name"`
	DisplayName string `json:"display_name"`
	Speed       int64  `json:"speed"`
	AdminStatus int    `json:"admin_status"`
	OperStatus  int    `json:"oper_status"`
	InOctets    int64  `json:"in_octets"`
	OutOctets   int64  `json:"out_octets"`
}

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return n
	}
	// Counter64 pode ultrapassar int64; nesse caso, mantém valor máximo em vez de zerar.
	u, errU := strconv.ParseUint(s, 10, 64)
	if errU != nil {
		return 0
	}
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(u)
}

func pickDisplayName(ifName, descr string) string {
	ifName = strings.TrimSpace(ifName)
	if ifName != "" {
		return ifName
	}
	return strings.TrimSpace(descr)
}

// BuildIfTable agrega ifTable (1.3.6.1.2.1.2.2.1), ifName e ifHC* (1.3.6.1.2.1.31.1.1.1.*) por ifIndex.
func BuildIfTable(vars []probing.SNMPVar) []IfRow {
	byIdx := map[int]map[int]string{}
	ifName := map[int]string{}
	hcIn := map[int]int64{}
	hcOut := map[int]int64{}
	hcInSeen := map[int]struct{}{}
	hcOutSeen := map[int]struct{}{}

	for _, v := range vars {
		oid := strings.TrimSpace(v.OID)
		val := strings.TrimSpace(v.Value)

		if m := reIfMib.FindStringSubmatch(oid); m != nil {
			col, _ := strconv.Atoi(m[1])
			idx, _ := strconv.Atoi(m[2])
			if idx <= 0 {
				continue
			}
			if byIdx[idx] == nil {
				byIdx[idx] = map[int]string{}
			}
			byIdx[idx][col] = val
			continue
		}
		if m := reIfName.FindStringSubmatch(oid); m != nil {
			idx, _ := strconv.Atoi(m[1])
			if idx > 0 {
				ifName[idx] = val
			}
			continue
		}
		if m := reIfHCIn.FindStringSubmatch(oid); m != nil {
			idx, _ := strconv.Atoi(m[1])
			if idx > 0 {
				hcIn[idx] = parseInt64(val)
				hcInSeen[idx] = struct{}{}
			}
			continue
		}
		if m := reIfHCOut.FindStringSubmatch(oid); m != nil {
			idx, _ := strconv.Atoi(m[1])
			if idx > 0 {
				hcOut[idx] = parseInt64(val)
				hcOutSeen[idx] = struct{}{}
			}
		}
	}

	all := map[int]struct{}{}
	for ix := range byIdx {
		all[ix] = struct{}{}
	}
	for ix := range ifName {
		all[ix] = struct{}{}
	}
	for ix := range hcIn {
		all[ix] = struct{}{}
	}
	for ix := range hcOut {
		all[ix] = struct{}{}
	}

	var indexes []int
	for ix := range all {
		indexes = append(indexes, ix)
	}
	sort.Ints(indexes)

	rows := make([]IfRow, 0, len(indexes))
	for _, idx := range indexes {
		c := byIdx[idx]
		if c == nil {
			c = map[int]string{}
		}
		in32 := parseInt64(c[10])
		out32 := parseInt64(c[16])
		in := in32
		if _, ok := hcInSeen[idx]; ok {
			in = hcIn[idx]
		}
		outOct := out32
		if _, ok := hcOutSeen[idx]; ok {
			outOct = hcOut[idx]
		}
		nm := ifName[idx]
		ds := c[2]
		row := IfRow{
			IfIndex:     idx,
			Descr:       ds,
			IfName:      nm,
			DisplayName: pickDisplayName(nm, ds),
			Speed:       parseInt64(c[5]),
			AdminStatus: int(parseInt64(c[7])),
			OperStatus:  int(parseInt64(c[8])),
			InOctets:    in,
			OutOctets:   outOct,
		}
		rows = append(rows, row)
	}
	return rows
}

// AdminStatusLabel IF-MIB ifAdminStatus.
func AdminStatusLabel(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	default:
		return strconv.Itoa(v)
	}
}

// OperStatusLabel IF-MIB ifOperStatus.
func OperStatusLabel(v int) string {
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
		return strconv.Itoa(v)
	}
}
