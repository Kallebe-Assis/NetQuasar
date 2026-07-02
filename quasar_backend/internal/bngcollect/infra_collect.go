package bngcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// IPPoolSnapshot utilização de pool IPv4.
type IPPoolSnapshot struct {
	Index       string `json:"index"`
	Name        string `json:"name"`
	TotalIPs    int    `json:"total_ips"`
	UsedIPs     int    `json:"used_ips"`
	IdleIPs     int    `json:"idle_ips"`
	UsedPercent int    `json:"used_percent"`
	VRF         string `json:"vrf,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
}

// IPv6PoolSnapshot utilização de pool IPv6.
type IPv6PoolSnapshot struct {
	Index             string `json:"index"`
	Name              string `json:"name"`
	AddressTotal      int    `json:"address_total"`
	AddressUsed       int    `json:"address_used"`
	AddressFree       int    `json:"address_free"`
	AddressUsedPct    int    `json:"address_used_percent"`
	PDPrefixTotal     int    `json:"pd_prefix_total"`
	PDPrefixUsed      int    `json:"pd_prefix_used"`
	PDPrefixFree      int    `json:"pd_prefix_free"`
	PDPrefixUsedPct   int    `json:"pd_prefix_used_percent"`
}

// RadiusServerSnapshot estatísticas por servidor RADIUS.
type RadiusServerSnapshot struct {
	Key       string `json:"key"`
	Group     string `json:"group,omitempty"`
	Type      string `json:"type,omitempty"`
	IP        string `json:"ip,omitempty"`
	Port      string `json:"port,omitempty"`
	Responses string `json:"responses,omitempty"`
	VRF       string `json:"vrf,omitempty"`
}

// CGNSnapshot totais CGN/NAT.
type CGNSnapshot struct {
	CurrentSessions   string `json:"current_sessions,omitempty"`
	LicenseTotalM     string `json:"license_total_m,omitempty"`
	LicenseUsedM      string `json:"license_used_m,omitempty"`
	LicenseFreeM      string `json:"license_free_m,omitempty"`
	BitThroughputUp   string `json:"bit_throughput_up,omitempty"`
	BitThroughputDown string `json:"bit_throughput_down,omitempty"`
	DsliteTunnels     string `json:"dslite_tunnels,omitempty"`
}

// AAAScalars contadores escalares AAA.
type AAAScalars struct {
	HistoricMaxOnline string `json:"historic_max_online,omitempty"`
	MaxPPPoEOnline    string `json:"max_pppoe_online,omitempty"`
	TotalConnect      string `json:"total_connect,omitempty"`
	TotalSuccess      string `json:"total_success,omitempty"`
	TotalAuthenFail   string `json:"total_authen_fail,omitempty"`
	TotalPPPFail      string `json:"total_ppp_fail,omitempty"`
	TotalLCPFail      string `json:"total_lcp_fail,omitempty"`
	TotalIPAllocFail  string `json:"total_ip_alloc_fail,omitempty"`
	IPv4FlowUpBytes   string `json:"ipv4_flow_up_bytes,omitempty"`
	IPv4FlowDnBytes   string `json:"ipv4_flow_dn_bytes,omitempty"`
	IPv6FlowUpBytes   string `json:"ipv6_flow_up_bytes,omitempty"`
	IPv6FlowDnBytes   string `json:"ipv6_flow_dn_bytes,omitempty"`
	WiredOnline       string `json:"wired_online,omitempty"`
	VLANOnline        string `json:"vlan_online,omitempty"`
}

// InfrastructureSnapshot pools, RADIUS, CGN, energia e escalares.
type InfrastructureSnapshot struct {
	CollectedAt          time.Time                `json:"collected_at"`
	AAA                  AAAScalars               `json:"aaa_scalars"`
	PowerConsumption     string                   `json:"power_consumption,omitempty"`
	IPv4Pools            []IPPoolSnapshot         `json:"ipv4_pools"`
	IPv6Pools            []IPv6PoolSnapshot       `json:"ipv6_pools"`
	RadiusServers        []RadiusServerSnapshot   `json:"radius_servers"`
	CGN                  CGNSnapshot              `json:"cgn"`
	BGP                  BGPSnapshot              `json:"bgp"`
	PowerSupplies        []PowerSupplySnapshot    `json:"power_supplies"`
	PhysicalInterfaces   PhysicalInterfaceSummary `json:"physical_interfaces"`
	LinkTraffic          []LinkTrafficSnapshot    `json:"link_traffic"`
	CGNPublicPools       []CGNPublicPoolSnapshot  `json:"cgn_public_pools"`
	Errors               []string                 `json:"errors,omitempty"`
}

var infraScalarOIDs = map[string]string{
	"historic_max_online": "1.3.6.1.4.1.2011.5.2.1.14.1.8.0",
	"max_pppoe_online":    "1.3.6.1.4.1.2011.5.2.1.14.1.12.0",
	"wired_online":        "1.3.6.1.4.1.2011.5.2.1.14.1.33.0",
	"vlan_online":         "1.3.6.1.4.1.2011.5.2.1.14.1.7.0",
	"total_connect":       "1.3.6.1.4.1.2011.5.2.1.29.1.1.0",
	"total_success":       "1.3.6.1.4.1.2011.5.2.1.29.1.2.0",
	"total_lcp_fail":      "1.3.6.1.4.1.2011.5.2.1.29.1.3.0",
	"total_authen_fail":   "1.3.6.1.4.1.2011.5.2.1.29.1.4.0",
	"total_ip_alloc_fail": "1.3.6.1.4.1.2011.5.2.1.29.1.6.0",
	"total_ppp_fail":      "1.3.6.1.4.1.2011.5.2.1.29.1.8.0",
	"ipv4_flow_up_bytes":  "1.3.6.1.4.1.2011.5.2.1.14.1.20.0",
	"ipv4_flow_dn_bytes":  "1.3.6.1.4.1.2011.5.2.1.14.1.18.0",
	"ipv6_flow_up_bytes":  "1.3.6.1.4.1.2011.5.2.1.14.1.24.0",
	"ipv6_flow_dn_bytes":  "1.3.6.1.4.1.2011.5.2.1.14.1.22.0",
	"power_consumption":   "1.3.6.1.4.1.2011.6.157.1.1.0",
	"cgn_current_sessions": "1.3.6.1.4.1.2011.5.25.306.1.13.2.0",
	"cgn_license_total":   "1.3.6.1.4.1.2011.5.25.306.1.13.11.0",
	"cgn_license_used":    "1.3.6.1.4.1.2011.5.25.306.1.13.12.0",
	"cgn_license_free":    "1.3.6.1.4.1.2011.5.25.306.1.13.13.0",
	"cgn_bit_up":          "1.3.6.1.4.1.2011.5.25.306.1.13.5.0",
	"cgn_bit_down":        "1.3.6.1.4.1.2011.5.25.306.1.13.6.0",
	"cgn_dslite_tunnels":  "1.3.6.1.4.1.2011.5.25.306.1.13.10.0",
}

// CollectInfrastructure coleta escalares, pools IPv4/v6, RADIUS, CGN e operações de rede.
func CollectInfrastructure(ctx context.Context, host, community string, timeout time.Duration, opts CollectionOptions) InfrastructureSnapshot {
	snap := InfrastructureSnapshot{
		CollectedAt:   time.Now().UTC(),
		IPv4Pools:     []IPPoolSnapshot{},
		IPv6Pools:     []IPv6PoolSnapshot{},
		RadiusServers: []RadiusServerSnapshot{},
		PowerSupplies: []PowerSupplySnapshot{},
		LinkTraffic:   []LinkTrafficSnapshot{},
		CGNPublicPools: []CGNPublicPoolSnapshot{},
		BGP: BGPSnapshot{Peers: []BGPPeerSnapshot{}},
		PhysicalInterfaces: PhysicalInterfaceSummary{Interfaces: []PhysicalInterfaceSnapshot{}},
	}
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" {
		snap.Errors = append(snap.Errors, "host ou community em falta")
		return snap
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	scalarVals := snmpGetScalars(ctx, host, community, timeout, infraScalarOIDs)
	snap.AAA = AAAScalars{
		HistoricMaxOnline: scalarVals["historic_max_online"],
		MaxPPPoEOnline:    scalarVals["max_pppoe_online"],
		TotalConnect:      scalarVals["total_connect"],
		TotalSuccess:      scalarVals["total_success"],
		TotalAuthenFail:   scalarVals["total_authen_fail"],
		TotalPPPFail:      scalarVals["total_ppp_fail"],
		TotalLCPFail:      scalarVals["total_lcp_fail"],
		TotalIPAllocFail:  scalarVals["total_ip_alloc_fail"],
		IPv4FlowUpBytes:   scalarVals["ipv4_flow_up_bytes"],
		IPv4FlowDnBytes:   scalarVals["ipv4_flow_dn_bytes"],
		IPv6FlowUpBytes:   scalarVals["ipv6_flow_up_bytes"],
		IPv6FlowDnBytes:   scalarVals["ipv6_flow_dn_bytes"],
		WiredOnline:       scalarVals["wired_online"],
		VLANOnline:        scalarVals["vlan_online"],
	}
	snap.PowerConsumption = scalarVals["power_consumption"]
	snap.CGN = CGNSnapshot{
		CurrentSessions:   scalarVals["cgn_current_sessions"],
		LicenseTotalM:     scalarVals["cgn_license_total"],
		LicenseUsedM:      scalarVals["cgn_license_used"],
		LicenseFreeM:      scalarVals["cgn_license_free"],
		BitThroughputUp:   scalarVals["cgn_bit_up"],
		BitThroughputDown: scalarVals["cgn_bit_down"],
		DsliteTunnels:     scalarVals["cgn_dslite_tunnels"],
	}

	snap.IPv4Pools = collectIPv4Pools(ctx, host, community, timeout)
	snap.IPv6Pools = collectIPv6Pools(ctx, host, community, timeout)
	snap.RadiusServers = collectRadiusServers(ctx, host, community, timeout)
	snap.BGP = collectBGPSnapshot(ctx, host, community, timeout)
	snap.PowerSupplies = collectPowerSupplies(ctx, host, community, timeout)
	snap.PhysicalInterfaces = collectPhysicalInterfaces(ctx, host, community, timeout)
	snap.LinkTraffic = collectLinkTraffic(ctx, host, community, timeout, opts.UplinkInterfaces)
	snap.CGNPublicPools = collectCGNPublicPools(ctx, host, community, timeout)
	return snap
}

// CollectAndStoreInfrastructure coleta infra, calcula taxas de uplink e persiste.
func CollectAndStoreInfrastructure(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string, timeout time.Duration, opts CollectionOptions) error {
	prev, prevAt, hasPrev := LoadLatestInfrastructureSnapshot(ctx, pool, deviceID)
	snap := CollectInfrastructure(ctx, host, community, timeout, opts)
	if hasPrev && prevAt != nil {
		enrichLinkTrafficRatesFromPrevious(&snap, prev, *prevAt)
	}
	if linkTrafficNeedsInstantRates(snap.LinkTraffic) {
		snap.LinkTraffic = measureInstantLinkRates(ctx, host, community, timeout, snap.LinkTraffic)
	}
	return StoreInfrastructureSnapshot(ctx, pool, deviceID, snap)
}

func snmpGetScalars(ctx context.Context, host, community string, timeout time.Duration, oids map[string]string) map[string]string {
	out := make(map[string]string, len(oids))
	list := make([]string, 0, len(oids))
	keyByOID := make(map[string]string, len(oids))
	for k, oid := range oids {
		list = append(list, oid)
		keyByOID[oid] = k
		keyByOID[probing.NormalizeSNMPOID(oid)] = k
	}
	vars, _ := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, list, 40)
	for _, v := range vars {
		key := keyByOID[v.OID]
		if key == "" {
			key = keyByOID[probing.NormalizeSNMPOID(v.OID)]
		}
		if key != "" {
			out[key] = strings.TrimSpace(v.Value)
		}
	}
	return out
}

func collectIPv4Pools(ctx context.Context, host, community string, timeout time.Duration) []IPPoolSnapshot {
	const base = "1.3.6.1.4.1.2011.6.8.1.1.1"
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"name":    base + ".2",
		"gateway": base + ".3",
		"total":   base + ".15",
		"used":    base + ".16",
		"idle":    base + ".19",
		"pct":     base + ".20",
		"vrf":     base + ".11",
	})
	indices := mergeIndices(cols)
	out := make([]IPPoolSnapshot, 0, len(indices))
	for _, idx := range indices {
		name := strings.TrimSpace(cols["name"][idx])
		if name == "" {
			continue
		}
		out = append(out, IPPoolSnapshot{
			Index:       idx,
			Name:        name,
			TotalIPs:    parseIntDefault(cols["total"][idx]),
			UsedIPs:     parseIntDefault(cols["used"][idx]),
			IdleIPs:     parseIntDefault(cols["idle"][idx]),
			UsedPercent: parseIntDefault(cols["pct"][idx]),
			VRF:         strings.TrimSpace(cols["vrf"][idx]),
			Gateway:     strings.TrimSpace(cols["gateway"][idx]),
		})
	}
	return out
}

func collectIPv6Pools(ctx context.Context, host, community string, timeout time.Duration) []IPv6PoolSnapshot {
	const base = "1.3.6.1.4.1.2011.6.8.1.18.1"
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"name":       base + ".23",
		"addr_total": base + ".5",
		"addr_used":  base + ".6",
		"addr_free":  base + ".7",
		"addr_pct":   base + ".10",
		"pd_total":   base + ".17",
		"pd_used":    base + ".18",
		"pd_free":    base + ".19",
		"pd_pct":     base + ".22",
	})
	indices := mergeIndices(cols)
	out := make([]IPv6PoolSnapshot, 0, len(indices))
	seen := map[string]struct{}{}
	for _, idx := range indices {
		name := strings.TrimSpace(cols["name"][idx])
		if name == "" {
			name = idx
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, IPv6PoolSnapshot{
			Index:           idx,
			Name:            name,
			AddressTotal:    parseIntDefault(cols["addr_total"][idx]),
			AddressUsed:     parseIntDefault(cols["addr_used"][idx]),
			AddressFree:     parseIntDefault(cols["addr_free"][idx]),
			AddressUsedPct:  parseIntDefault(cols["addr_pct"][idx]),
			PDPrefixTotal:   parseIntDefault(cols["pd_total"][idx]),
			PDPrefixUsed:    parseIntDefault(cols["pd_used"][idx]),
			PDPrefixFree:    parseIntDefault(cols["pd_free"][idx]),
			PDPrefixUsedPct: parseIntDefault(cols["pd_pct"][idx]),
		})
	}
	return out
}

func collectRadiusServers(ctx context.Context, host, community string, timeout time.Duration) []RadiusServerSnapshot {
	const tableBase = "1.3.6.1.4.1.2011.5.25.40.15.1.2.1"
	cols := fetchWalkColumns(ctx, host, community, timeout, map[string]string{
		"type":      tableBase + ".2",
		"ip":        tableBase + ".4",
		"port":      tableBase + ".5",
		"responses": tableBase + ".12",
		"vrf":       tableBase + ".3",
	})
	indices := mergeIndices(cols)
	out := make([]RadiusServerSnapshot, 0, len(indices))
	for _, idx := range indices {
		ip := strings.TrimSpace(cols["ip"][idx])
		if ip == "" || ip == "0.0.0.0" {
			continue
		}
		typeLabel := mapRadiusServerType(cols["type"][idx])
		out = append(out, RadiusServerSnapshot{
			Key:       idx,
			Type:      typeLabel,
			IP:        ip,
			Port:      strings.TrimSpace(cols["port"][idx]),
			Responses: strings.TrimSpace(cols["responses"][idx]),
			VRF:       strings.TrimSpace(cols["vrf"][idx]),
		})
	}
	return out
}

func mapRadiusServerType(v string) string {
	switch strings.TrimSpace(v) {
	case "1", "auth":
		return "Autenticação"
	case "2", "acct":
		return "Accounting"
	default:
		if v == "" {
			return "—"
		}
		return "Tipo " + v
	}
}

func mergeIndices(cols map[string]map[string]string) []string {
	set := map[string]struct{}{}
	for _, m := range cols {
		for idx := range m {
			set[idx] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for idx := range set {
		out = append(out, idx)
	}
	return out
}

func parseIntDefault(v string) int {
	n, ok := parseIntMetric(strings.TrimSpace(v))
	if !ok {
		return 0
	}
	return n
}

// StoreInfrastructureSnapshot persiste snapshot de infra na telemetria.
func StoreInfrastructureSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, snap InfrastructureSnapshot) error {
	if pool == nil {
		return fmt.Errorf("pool indisponível")
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO telemetry_samples (device_id, collected_at, metrics)
		VALUES ($1, now(), jsonb_build_object('bng_infrastructure', $2::jsonb))
	`, deviceID, b)
	return err
}

// LoadLatestInfrastructureSnapshot lê o último snapshot de infra.
func LoadLatestInfrastructureSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (InfrastructureSnapshot, *time.Time, bool) {
	var raw []byte
	var at time.Time
	err := pool.QueryRow(ctx, `
		SELECT collected_at, metrics->'bng_infrastructure'
		FROM telemetry_samples
		WHERE device_id=$1 AND metrics ? 'bng_infrastructure'
		ORDER BY collected_at DESC LIMIT 1
	`, deviceID).Scan(&at, &raw)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return InfrastructureSnapshot{}, nil, false
	}
	var snap InfrastructureSnapshot
	if json.Unmarshal(raw, &snap) != nil {
		return InfrastructureSnapshot{}, nil, false
	}
	return snap, &at, true
}

func fetchWalkColumns(ctx context.Context, host, community string, timeout time.Duration, columns map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(columns))
	for key, root := range columns {
		vars, _, _ := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
			Host: host, Community: community, RootOID: root, Version: "2c",
			Timeout: timeout, MaxRows: 5000,
		})
		m := make(map[string]string)
		for _, v := range vars {
			idx := extractIndexFromOID(v.OID, root)
			if idx == "" {
				continue
			}
			m[idx] = strings.TrimSpace(v.Value)
		}
		out[key] = m
	}
	return out
}
