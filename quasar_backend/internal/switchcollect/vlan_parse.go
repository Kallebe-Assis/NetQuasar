package switchcollect

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const (
	ciscoVlanMembershipBase = "1.3.6.1.4.1.9.9.68.1.2.2.1"
	ciscoVtpVlanNameBase    = "1.3.6.1.4.1.9.9.46.1.3.1.1.4"

	vmColType  = 1
	vmColVlan  = 2
	vmColVlans = 4
)

// PortVlanInfo VLANs associadas a uma interface (ifIndex).
type PortVlanInfo struct {
	Mode    string `json:"mode"` // access, trunk, dynamic
	VlanIDs []int  `json:"vlan_ids"`
}

type portVlanAcc struct {
	vlanType int
	vmVlan   int
	vmVlans  string
}

// ParsePortVlanMap extrai vmVlanType / vmVlan / vmVlans (CISCO-VLAN-MEMBERSHIP-MIB).
func ParsePortVlanMap(vars []probing.SNMPVar) map[int]PortVlanInfo {
	byIf := map[int]*portVlanAcc{}
	prefix := ciscoVlanMembershipBase + "."
	for _, v := range vars {
		oid := strings.TrimPrefix(strings.TrimSpace(v.OID), ".")
		if !strings.HasPrefix(oid, prefix) {
			continue
		}
		rest := strings.TrimPrefix(oid, prefix)
		parts := strings.Split(rest, ".")
		if len(parts) < 2 {
			continue
		}
		col, err := strconv.Atoi(parts[0])
		if err != nil || col <= 0 {
			continue
		}
		ifIdx, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || ifIdx <= 0 {
			continue
		}
		acc := byIf[ifIdx]
		if acc == nil {
			acc = &portVlanAcc{}
			byIf[ifIdx] = acc
		}
		val := strings.TrimSpace(v.Value)
		switch col {
		case vmColType:
			acc.vlanType = atoiLoose(val)
		case vmColVlan:
			acc.vmVlan = atoiLoose(val)
		case vmColVlans:
			acc.vmVlans = val
		}
	}

	out := make(map[int]PortVlanInfo, len(byIf))
	for ifIdx, acc := range byIf {
		info := PortVlanInfo{}
		switch acc.vlanType {
		case 1:
			info.Mode = "access"
			if acc.vmVlan > 0 {
				info.VlanIDs = []int{acc.vmVlan}
			}
		case 2:
			info.Mode = "dynamic"
			if acc.vmVlan > 0 {
				info.VlanIDs = []int{acc.vmVlan}
			}
		case 3:
			info.Mode = "trunk"
			info.VlanIDs = parseVlanBitmap(acc.vmVlans)
		default:
			if len(acc.vmVlans) > 0 {
				info.Mode = "trunk"
				info.VlanIDs = parseVlanBitmap(acc.vmVlans)
			} else if acc.vmVlan > 0 {
				info.Mode = "access"
				info.VlanIDs = []int{acc.vmVlan}
			}
		}
		if len(info.VlanIDs) > 0 || info.Mode != "" {
			out[ifIdx] = info
		}
	}
	return out
}

// ParseVlanNames mapeia VLAN ID → nome (CISCO-VTP-MIB vtpVlanName).
func ParseVlanNames(vars []probing.SNMPVar) map[int]string {
	out := map[int]string{}
	prefix := ciscoVtpVlanNameBase + "."
	for _, v := range vars {
		oid := strings.TrimPrefix(strings.TrimSpace(v.OID), ".")
		if !strings.HasPrefix(oid, prefix) {
			continue
		}
		rest := strings.TrimPrefix(oid, prefix)
		// índice: managementDomainIndex.vlanIndex (ex.: 1.740)
		parts := strings.Split(rest, ".")
		if len(parts) < 2 {
			continue
		}
		vlanID, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || vlanID <= 0 {
			continue
		}
		name := strings.TrimSpace(v.Value)
		if name != "" {
			out[vlanID] = name
		}
	}
	return out
}

// FormatVlanLabel texto para UI (IDs + nomes quando disponíveis).
func FormatVlanLabel(info PortVlanInfo, names map[int]string) string {
	if len(info.VlanIDs) == 0 {
		if info.Mode == "trunk" {
			return "trunk"
		}
		return ""
	}
	const maxShow = 8
	ids := append([]int(nil), info.VlanIDs...)
	sort.Ints(ids)
	parts := make([]string, 0, len(ids))
	for i, id := range ids {
		if i >= maxShow {
			parts = append(parts, fmt.Sprintf("+%d", len(ids)-maxShow))
			break
		}
		if name := strings.TrimSpace(names[id]); name != "" {
			parts = append(parts, fmt.Sprintf("%d (%s)", id, name))
		} else {
			parts = append(parts, strconv.Itoa(id))
		}
	}
	label := strings.Join(parts, ", ")
	if info.Mode == "trunk" && label != "" {
		return "trunk: " + label
	}
	if info.Mode == "access" && label != "" {
		return label
	}
	return label
}

// EnrichInterfaceVlans adiciona vlan_mode, vlans e vlan_label às linhas da interface_table.
func EnrichInterfaceVlans(tab []map[string]any, vars []probing.SNMPVar) {
	if len(tab) == 0 || len(vars) == 0 {
		return
	}
	byIf := ParsePortVlanMap(vars)
	if len(byIf) == 0 {
		return
	}
	names := ParseVlanNames(vars)
	for _, row := range tab {
		ifIdx := intFromAny(row["if_index"])
		if ifIdx <= 0 {
			continue
		}
		info, ok := byIf[ifIdx]
		if !ok {
			continue
		}
		row["vlan_mode"] = info.Mode
		row["vlans"] = info.VlanIDs
		if lbl := FormatVlanLabel(info, names); lbl != "" {
			row["vlan_label"] = lbl
		}
	}
}

func atoiLoose(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// INTEGER: 3 ou Gauge32: 740
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	n, _ := strconv.Atoi(s)
	return n
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func parseVlanBitmap(raw string) []int {
	b := decodeHexOctets(raw)
	if len(b) == 0 {
		return nil
	}
	var out []int
	for vlanID := 1; vlanID <= 4094; vlanID++ {
		byteIdx := (vlanID - 1) / 8
		if byteIdx >= len(b) {
			break
		}
		bit := 7 - ((vlanID - 1) % 8)
		if b[byteIdx]&(1<<uint(bit)) != 0 {
			out = append(out, vlanID)
		}
	}
	return out
}

func decodeHexOctets(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if i := strings.Index(raw, ":"); i >= 0 && strings.Contains(strings.ToLower(raw[:i]), "string") {
		raw = strings.TrimSpace(raw[i+1:])
	}
	raw = strings.ReplaceAll(raw, " ", "")
	raw = strings.ReplaceAll(raw, ":", "")
	if len(raw)%2 != 0 {
		return nil
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil
	}
	return b
}
