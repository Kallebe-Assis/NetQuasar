package bngcollect

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// BGPPeerSnapshot peer BGP individual.
type BGPPeerSnapshot struct {
	RemoteAddr string `json:"remote_addr"`
	State      string `json:"state"`
	StateCode  string `json:"state_code,omitempty"`
	LocalIface string `json:"local_iface,omitempty"`
}

// BGPSnapshot totais e lista de peers BGP.
type BGPSnapshot struct {
	TotalPeers  int               `json:"total_peers"`
	Established int               `json:"established"`
	Peers       []BGPPeerSnapshot `json:"peers,omitempty"`
}

// PowerSupplySnapshot fonte de alimentação.
type PowerSupplySnapshot struct {
	Index       string `json:"index"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	StatusCode  string `json:"status_code,omitempty"`
}

// PhysicalInterfaceSnapshot interface física.
type PhysicalInterfaceSnapshot struct {
	IfIndex    int    `json:"if_index"`
	Name       string `json:"name"`
	OperStatus string `json:"oper_status"`
	AdminStatus string `json:"admin_status,omitempty"`
}

// PhysicalInterfaceSummary contagem UP/DOWN.
type PhysicalInterfaceSummary struct {
	UpCount    int                         `json:"up_count"`
	DownCount  int                         `json:"down_count"`
	Total      int                         `json:"total"`
	Interfaces []PhysicalInterfaceSnapshot `json:"interfaces,omitempty"`
}

// LinkTrafficSnapshot tráfego por interface de uplink.
type LinkTrafficSnapshot struct {
	IfIndex      int    `json:"if_index"`
	Name         string `json:"name"`
	OperStatus   string `json:"oper_status,omitempty"`
	InOctets     string `json:"in_octets,omitempty"`
	OutOctets    string `json:"out_octets,omitempty"`
	InBps        int64  `json:"in_bps,omitempty"`
	OutBps       int64  `json:"out_bps,omitempty"`
	InDisplay    string `json:"in_display,omitempty"`
	OutDisplay   string `json:"out_display,omitempty"`
	RateInterval string `json:"rate_interval_ms,omitempty"`
}

// CGNPublicPoolSnapshot pool público CGNAT.
type CGNPublicPoolSnapshot struct {
	Index       string `json:"index"`
	Instance    string `json:"instance,omitempty"`
	PoolName    string `json:"pool_name,omitempty"`
	StartAddr   string `json:"start_addr,omitempty"`
	EndAddr     string `json:"end_addr,omitempty"`
	MaskAddr    string `json:"mask_addr,omitempty"`
	UsagePct    int    `json:"usage_percent,omitempty"`
}

// CGNATMappingRow mapeamento aproximado privado → pool público.
type CGNATMappingRow struct {
	PrivateIP    string `json:"private_ip"`
	PublicHint   string `json:"public_hint"`
	PoolName     string `json:"pool_name,omitempty"`
	CGNAT        bool   `json:"cgnat"`
	SessionCount int    `json:"session_count"`
}

var physicalIfTypes = map[string]bool{
	"6": true, "62": true, "117": true, "161": true,
}

var virtualIfacePatterns = []string{
	"loopback", "null", "vlanif", "virtual", "tunnel", "nve", "meth", "ip-trunk",
}

func collectBGPSnapshot(ctx context.Context, host, community string, timeout time.Duration) BGPSnapshot {
	out := BGPSnapshot{Peers: []BGPPeerSnapshot{}}
	const peerBase = "1.3.6.1.2.1.15.3.1"
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"state":  peerBase + ".2",
		"remote": peerBase + ".7",
	})
	indices := mergeIndices(cols)
	for _, idx := range indices {
		remote := decodeSNMPIPValue(cols["remote"][idx])
		if remote == "" {
			remote = idx
		}
		stateCode := strings.TrimSpace(cols["state"][idx])
		state := mapBGPState(stateCode)
		out.Peers = append(out.Peers, BGPPeerSnapshot{
			RemoteAddr: remote,
			State:      state,
			StateCode:  stateCode,
		})
		if state == "Established" {
			out.Established++
		}
	}
	out.TotalPeers = len(out.Peers)
	if out.TotalPeers == 0 {
		scalar := snmpGetScalars(ctx, host, community, timeout, map[string]string{
			"total": "1.3.6.1.4.1.2011.5.25.177.1.4.1",
		})
		if n := parseIntDefault(scalar["total"]); n > 0 {
			out.TotalPeers = n
		}
	}
	return out
}

func collectPowerSupplies(ctx context.Context, host, community string, timeout time.Duration) []PowerSupplySnapshot {
	const entBase = "1.3.6.1.2.1.47.1.1.1.1"
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"class": entBase + ".5",
		"descr": entBase + ".2",
		"name":  entBase + ".7",
	})
	statusCols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"status": "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.2",
	})
	out := []PowerSupplySnapshot{}
	for idx, class := range cols["class"] {
		if strings.TrimSpace(class) != "6" {
			continue
		}
		name := strings.TrimSpace(cols["name"][idx])
		if name == "" {
			name = strings.TrimSpace(cols["descr"][idx])
		}
		statusCode := strings.TrimSpace(statusCols["status"][idx])
		out = append(out, PowerSupplySnapshot{
			Index:       idx,
			Name:        name,
			Description: strings.TrimSpace(cols["descr"][idx]),
			Status:      mapEntityOperStatus(statusCode),
			StatusCode:  statusCode,
		})
	}
	return out
}

func collectPhysicalInterfaces(ctx context.Context, host, community string, timeout time.Duration) PhysicalInterfaceSummary {
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"name":  "1.3.6.1.2.1.31.1.1.1.1",
		"type":  "1.3.6.1.2.1.2.2.1.3",
		"oper":  "1.3.6.1.2.1.2.2.1.8",
		"admin": "1.3.6.1.2.1.2.2.1.7",
	})
	summary := PhysicalInterfaceSummary{Interfaces: []PhysicalInterfaceSnapshot{}}
	indices := mergeIndices(cols)
	for _, idx := range indices {
		ifType := strings.TrimSpace(cols["type"][idx])
		if !physicalIfTypes[ifType] {
			continue
		}
		name := strings.TrimSpace(cols["name"][idx])
		if name == "" || isVirtualIfaceName(name) {
			continue
		}
		ifIndex, _ := strconv.Atoi(idx)
		oper := mapIfOperStatus(cols["oper"][idx])
		row := PhysicalInterfaceSnapshot{
			IfIndex:     ifIndex,
			Name:        name,
			OperStatus:  oper,
			AdminStatus: mapIfAdminStatus(cols["admin"][idx]),
		}
		summary.Interfaces = append(summary.Interfaces, row)
		summary.Total++
		if oper == "UP" {
			summary.UpCount++
		} else {
			summary.DownCount++
		}
	}
	return summary
}

func collectLinkTraffic(ctx context.Context, host, community string, timeout time.Duration, uplinkNames []string) []LinkTrafficSnapshot {
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"name": "1.3.6.1.2.1.31.1.1.1.1",
		"in":   "1.3.6.1.2.1.31.1.1.1.6",
		"out":  "1.3.6.1.2.1.31.1.1.1.10",
		"oper": "1.3.6.1.2.1.2.2.1.8",
	})
	targets := normalizeUplinkNames(uplinkNames)
	out := []LinkTrafficSnapshot{}
	for idx, name := range cols["name"] {
		name = strings.TrimSpace(name)
		if name == "" || !matchesUplink(name, targets) {
			continue
		}
		ifIndex, _ := strconv.Atoi(idx)
		out = append(out, LinkTrafficSnapshot{
			IfIndex:    ifIndex,
			Name:       name,
			OperStatus: mapIfOperStatus(cols["oper"][idx]),
			InOctets:   strings.TrimSpace(cols["in"][idx]),
			OutOctets:  strings.TrimSpace(cols["out"][idx]),
		})
	}
	return out
}

func collectCGNPublicPools(ctx context.Context, host, community string, timeout time.Duration) []CGNPublicPoolSnapshot {
	const infoRoot = "1.3.6.1.4.1.2011.5.25.306.1.9.1"
	infoCols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"start": infoRoot + ".5",
		"end":   infoRoot + ".6",
		"mask":  infoRoot + ".7",
		"vpn":   infoRoot + ".9",
	})
	const groupRoot = "1.3.6.1.4.1.2011.5.25.306.1.3.1"
	groupCols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"inst_name":  groupRoot + ".4",
		"group_name": groupRoot + ".5",
		"usage":      groupRoot + ".6",
	})
	const poolGroupRoot = "1.3.6.1.4.1.2011.5.25.306.1.20.1"
	poolGroupCols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"name":  poolGroupRoot + ".1",
		"usage": poolGroupRoot + ".2",
	})

	groupNameByPrefix := map[string]string{}
	for idx, name := range groupCols["group_name"] {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		prefix := cgnIndexPrefix(idx, 3)
		if prefix == "" {
			prefix = idx
		}
		if _, ok := groupNameByPrefix[prefix]; !ok {
			groupNameByPrefix[prefix] = name
		}
	}

	out := []CGNPublicPoolSnapshot{}
	seen := map[string]struct{}{}
	for idx := range infoCols["start"] {
		start := decodeSNMPIPValue(infoCols["start"][idx])
		end := decodeSNMPIPValue(infoCols["end"][idx])
		if start == "" && end == "" {
			continue
		}
		key := start + "|" + end
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		poolName := strings.TrimSpace(infoCols["vpn"][idx])
		if poolName == "" {
			poolName = groupNameByPrefix[cgnIndexPrefix(idx, 3)]
		}
		if poolName == "" {
			poolName = groupNameByPrefix[cgnIndexPrefix(idx, 2)]
		}
		out = append(out, CGNPublicPoolSnapshot{
			Index:     idx,
			PoolName:  poolName,
			StartAddr: start,
			EndAddr:   end,
			MaskAddr:  decodeSNMPIPValue(infoCols["mask"][idx]),
		})
	}

	for idx, name := range poolGroupCols["name"] {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := "pool:" + strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CGNPublicPoolSnapshot{
			Index:      idx,
			PoolName:   name,
			UsagePct:   parseIntDefault(poolGroupCols["usage"][idx]),
			StartAddr:  "",
			EndAddr:    "",
		})
	}

	for idx := range groupCols["group_name"] {
		name := strings.TrimSpace(groupCols["group_name"][idx])
		if name == "" {
			continue
		}
		key := "group:" + strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CGNPublicPoolSnapshot{
			Index:     idx,
			PoolName:  name,
			Instance:  strings.TrimSpace(groupCols["inst_name"][idx]),
			UsagePct:  parseIntDefault(groupCols["usage"][idx]),
		})
	}
	return out
}

func cgnIndexPrefix(idx string, parts int) string {
	p := strings.Split(strings.TrimSpace(idx), ".")
	if len(p) <= parts {
		return strings.TrimSpace(idx)
	}
	return strings.Join(p[:parts], ".")
}

func enrichLinkTrafficRatesFromPrevious(cur *InfrastructureSnapshot, prev InfrastructureSnapshot, prevAt time.Time) {
	if cur == nil || len(cur.LinkTraffic) == 0 {
		return
	}
	dt := cur.CollectedAt.Sub(prevAt).Seconds()
	if dt <= 0 {
		return
	}
	prevByName := map[string]LinkTrafficSnapshot{}
	for _, l := range prev.LinkTraffic {
		prevByName[strings.ToLower(l.Name)] = l
	}
	for i, link := range cur.LinkTraffic {
		p, ok := prevByName[strings.ToLower(link.Name)]
		if !ok {
			continue
		}
		in0, okIn0 := parseUint64Metric(p.InOctets)
		out0, okOut0 := parseUint64Metric(p.OutOctets)
		in1, okIn1 := parseUint64Metric(link.InOctets)
		out1, okOut1 := parseUint64Metric(link.OutOctets)
		if !okIn0 || !okIn1 || in1 < in0 {
			continue
		}
		if !okOut0 || !okOut1 || out1 < out0 {
			continue
		}
		inBps := int64((float64(in1-in0) * 8.0) / dt)
		outBps := int64((float64(out1-out0) * 8.0) / dt)
		cur.LinkTraffic[i].InBps = inBps
		cur.LinkTraffic[i].OutBps = outBps
		cur.LinkTraffic[i].InDisplay = FormatBitrateBps(inBps)
		cur.LinkTraffic[i].OutDisplay = FormatBitrateBps(outBps)
		cur.LinkTraffic[i].RateInterval = strconv.FormatInt(int64(dt*1000), 10) + "ms"
	}
}

func measureInstantLinkRates(ctx context.Context, host, community string, timeout time.Duration, links []LinkTrafficSnapshot) []LinkTrafficSnapshot {
	if len(links) == 0 {
		return links
	}
	type sample struct{ in, out uint64 }
	s0 := map[int]sample{}
	for _, l := range links {
		if l.IfIndex <= 0 {
			continue
		}
		suffix := "." + strconv.Itoa(l.IfIndex)
		vars, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, []string{
			"1.3.6.1.2.1.31.1.1.1.6" + suffix,
			"1.3.6.1.2.1.31.1.1.1.10" + suffix,
		}, 2)
		var in, out uint64
		for _, v := range vars {
			n, ok := parseUint64Metric(v.Value)
			if !ok {
				continue
			}
			switch probing.NormalizeSNMPOID(v.OID) {
			case probing.NormalizeSNMPOID("1.3.6.1.2.1.31.1.1.1.6" + suffix):
				in = n
			case probing.NormalizeSNMPOID("1.3.6.1.2.1.31.1.1.1.10" + suffix):
				out = n
			}
		}
		s0[l.IfIndex] = sample{in: in, out: out}
	}
	gap := 2 * time.Second
	select {
	case <-ctx.Done():
		return links
	case <-time.After(gap):
	}
	dt := gap.Seconds()
	for i, l := range links {
		s, ok := s0[l.IfIndex]
		if !ok {
			continue
		}
		vars, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, []string{
			"1.3.6.1.2.1.31.1.1.1.6" + "." + strconv.Itoa(l.IfIndex),
			"1.3.6.1.2.1.31.1.1.1.10" + "." + strconv.Itoa(l.IfIndex),
		}, 2)
		inOID := probing.NormalizeSNMPOID("1.3.6.1.2.1.31.1.1.1.6." + strconv.Itoa(l.IfIndex))
		outOID := probing.NormalizeSNMPOID("1.3.6.1.2.1.31.1.1.1.10." + strconv.Itoa(l.IfIndex))
		var in1, out1 uint64
		for _, v := range vars {
			n, ok := parseUint64Metric(v.Value)
			if !ok {
				continue
			}
			switch probing.NormalizeSNMPOID(v.OID) {
			case inOID:
				in1 = n
			case outOID:
				out1 = n
			}
		}
		if in1 >= s.in {
			inBps := int64((float64(in1-s.in) * 8.0) / dt)
			links[i].InBps = inBps
			links[i].InDisplay = FormatBitrateBps(inBps)
		}
		if out1 >= s.out {
			outBps := int64((float64(out1-s.out) * 8.0) / dt)
			links[i].OutBps = outBps
			links[i].OutDisplay = FormatBitrateBps(outBps)
		}
		if links[i].InBps > 0 || links[i].OutBps > 0 {
			links[i].RateInterval = "2000ms"
		}
	}
	return links
}

func linkTrafficNeedsInstantRates(links []LinkTrafficSnapshot) bool {
	for _, l := range links {
		if l.InBps == 0 && l.OutBps == 0 && (l.InOctets != "" || l.OutOctets != "") {
			return true
		}
	}
	return false
}

// BuildCGNATSummary agrega IPs privados das sessões com pool público provável.
func BuildCGNATSummary(sessions []map[string]any, pools []CGNPublicPoolSnapshot) []CGNATMappingRow {
	type agg struct {
		count int
	}
	byIP := map[string]*agg{}
	for _, s := range sessions {
		ip := strings.TrimSpace(fmtSprint(s["ipv4"]))
		if ip == "" {
			continue
		}
		if byIP[ip] == nil {
			byIP[ip] = &agg{}
		}
		byIP[ip].count++
	}
	out := make([]CGNATMappingRow, 0, len(byIP))
	for ip, a := range byIP {
		cgnat := isPrivateIPv4(ip)
		row := CGNATMappingRow{
			PrivateIP:    ip,
			CGNAT:        cgnat,
			SessionCount: a.count,
		}
		if cgnat {
			row.PublicHint = formatCGNATPublicHint(pools)
			if len(pools) == 1 {
				row.PoolName = pools[0].PoolName
			} else if len(pools) > 1 {
				row.PoolName = fmt.Sprintf("%d pools", len(pools))
			}
		} else {
			row.PublicHint = ip
		}
		out = append(out, row)
	}
	sortCGNATSummary(out)
	return out
}

func fmtSprint(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(fmt.Sprintf("%v", v), " "))
}

func sortCGNATSummary(rows []CGNATMappingRow) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].PrivateIP < rows[i].PrivateIP {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func formatCGNATPublicHint(pools []CGNPublicPoolSnapshot) string {
	if len(pools) == 0 {
		return "Pools CGNAT não detectados (execute coleta SNMP completa no BNG)"
	}
	if len(pools) == 1 {
		r := formatPoolRange(&pools[0])
		if r != "" && r != "—" {
			return "Pool: " + r
		}
	}
	parts := make([]string, 0, len(pools))
	for i := range pools {
		if r := formatPoolRange(&pools[i]); r != "" && r != "—" {
			parts = append(parts, r)
		}
	}
	if len(parts) == 0 {
		return "CGNAT — pools públicos sem range SNMP"
	}
	if len(parts) <= 2 {
		return strings.Join(parts, "; ")
	}
	return fmt.Sprintf("%d pools públicos (%s…)", len(parts), parts[0])
}

func formatPoolRange(p *CGNPublicPoolSnapshot) string {
	if p == nil {
		return ""
	}
	if p.StartAddr != "" && p.EndAddr != "" && p.StartAddr != p.EndAddr {
		return p.StartAddr + " – " + p.EndAddr
	}
	if p.StartAddr != "" {
		return p.StartAddr
	}
	return "—"
}

func isPrivateIPv4(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	parsed = parsed.To4()
	if parsed == nil {
		return false
	}
	return parsed[0] == 10 ||
		(parsed[0] == 172 && parsed[1] >= 16 && parsed[1] <= 31) ||
		(parsed[0] == 192 && parsed[1] == 168) ||
		(parsed[0] == 100 && parsed[1] >= 64 && parsed[1] <= 127) // RFC6598 CGNAT
}

func normalizeUplinkNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			out = append(out, strings.ToLower(n))
		}
	}
	return out
}

func matchesUplink(name string, targets []string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if len(targets) == 0 {
		return strings.Contains(lower, "bgp") || strings.Contains(lower, "wan-bgp")
	}
	for _, t := range targets {
		if lower == t || strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

func isVirtualIfaceName(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range virtualIfacePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func decodeSNMPIPValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if ip := net.ParseIP(v); ip != nil {
		return ip.String()
	}
	if decoded := decodeColonHexASCII(v); decoded != "" {
		if ip := net.ParseIP(decoded); ip != nil {
			return ip.String()
		}
		return decoded
	}
	parts := strings.Split(v, ".")
	if len(parts) == 4 {
		ok := true
		for _, p := range parts {
			if _, err := strconv.Atoi(strings.TrimSpace(p)); err != nil {
				ok = false
				break
			}
		}
		if ok {
			return v
		}
	}
	return v
}

func mapBGPState(v string) string {
	switch strings.TrimSpace(v) {
	case "1", "idle":
		return "Idle"
	case "2", "connect":
		return "Connect"
	case "3", "active":
		return "Active"
	case "4", "opensent":
		return "OpenSent"
	case "5", "openconfirm":
		return "OpenConfirm"
	case "6", "established":
		return "Established"
	default:
		if v == "" {
			return "—"
		}
		return "Estado " + v
	}
}

func mapEntityOperStatus(v string) string {
	switch strings.TrimSpace(v) {
	case "11", "up":
		return "UP"
	case "12", "down":
		return "DOWN"
	case "3", "enabled":
		return "Enabled"
	case "2", "disabled":
		return "Disabled"
	case "4", "offline":
		return "Offline"
	case "18", "present":
		return "Present"
	case "19", "absent":
		return "Absent"
	default:
		if v == "" {
			return "—"
		}
		return "Estado " + v
	}
}

func mapIfOperStatus(v string) string {
	switch strings.TrimSpace(v) {
	case "1", "up":
		return "UP"
	case "2", "down":
		return "DOWN"
	case "3", "testing":
		return "Testing"
	default:
		if v == "" {
			return "—"
		}
		return "Estado " + v
	}
}

func mapIfAdminStatus(v string) string {
	switch strings.TrimSpace(v) {
	case "1", "up":
		return "UP"
	case "2", "down":
		return "DOWN"
	default:
		if v == "" {
			return ""
		}
		return "Estado " + v
	}
}
