package bgpcollect

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// report.go — pivota o JSONB bruto de uma amostra telemetry_samples.metrics (gravada por
// CollectAndStoreForDevice) em estruturas prontas para a tela BGP (peers/interfaces/saúde).
// Cada métrica WALK guarda []probing.SNMPVar com a OID completa (base do catálogo + índice da
// tabela) — o índice é o IP do peer (bgpPeerTable, indexado por bgpPeerRemoteAddr) ou o ifIndex
// (IF-MIB/HUAWEI-IF-EXT-MIB). Cruzamos os arrays paralelos por esse índice comum.

// PeerReport uma linha de bgpPeerTable já cruzada entre as métricas activas do perfil.
type PeerReport struct {
	PeerIP             string `json:"peer_ip"`
	RemoteAS           string `json:"remote_as,omitempty"`
	State              string `json:"state,omitempty"`       // valor cru bgpPeerState (1-6)
	StateLabel         string `json:"state_label,omitempty"` // idle/connect/active/opensent/openconfirm/established
	EstablishedSeconds int64  `json:"established_seconds,omitempty"`
	InUpdates          string `json:"in_updates,omitempty"`
	OutUpdates         string `json:"out_updates,omitempty"`
	// Prefixos — HUAWEI-BGP-VPN-MIB hwBgpPeerRouteTable, contagem pré-agregada pelo próprio
	// equipamento (não precisa de varrer a tabela de rotas inteira).
	PrefixesReceived   *int64 `json:"prefixes_received,omitempty"`
	PrefixesActive     *int64 `json:"prefixes_active,omitempty"`
	PrefixesAdvertised *int64 `json:"prefixes_advertised,omitempty"`
}

// InterfaceReport uma linha de IF-MIB/HUAWEI-IF-EXT-MIB já cruzada por ifIndex.
type InterfaceReport struct {
	IfIndex     string `json:"if_index"`
	Descr       string `json:"descr,omitempty"`
	Alias       string `json:"alias,omitempty"`
	OperStatus  string `json:"oper_status,omitempty"` // valor cru ifOperStatus (1=up, 2=down, ...)
	HCInOctets  string `json:"hc_in_octets,omitempty"`
	HCOutOctets string `json:"hc_out_octets,omitempty"`
	InBitRate   string `json:"in_bit_rate,omitempty"`
	OutBitRate  string `json:"out_bit_rate,omitempty"`
}

// Report resultado pivotado pronto para a tela BGP.
type Report struct {
	CollectedAt string            `json:"collected_at,omitempty"`
	Peers       []PeerReport      `json:"peers"`
	Interfaces  []InterfaceReport `json:"interfaces"`
	Health      map[string]string `json:"health,omitempty"`

	// Novos domínios (ver report_hardware.go/report_ext.go/report_radius.go/report_qos.go/
	// report_lldp.go) — todos opcionais, só aparecem quando o perfil SNMP tem as métricas
	// correspondentes activas em Configurações → BGP.
	Optics        []OpticsReport       `json:"optics,omitempty"`
	CpuCores      []CpuCoreReport      `json:"cpu_cores,omitempty"`
	Fans          []FanReport          `json:"fans,omitempty"`
	PowerSupplies []PowerSupplyReport  `json:"power_supplies,omitempty"`
	Temperatures  []TemperatureReport  `json:"temperatures,omitempty"`
	Voltages      []VoltageReport      `json:"voltages,omitempty"`
	BoardAlarms   []BoardAlarmReport   `json:"board_alarms,omitempty"`
	VSList        []VSInfo             `json:"vs_list,omitempty"`
	VSResources   []VSResourceReport   `json:"vs_resources,omitempty"`
	BFDSessions   []BFDSessionReport   `json:"bfd_sessions,omitempty"`
	ETrunks       []ETrunkReport       `json:"etrunks,omitempty"`
	ETrunkMembers []ETrunkMemberReport `json:"etrunk_members,omitempty"`
	QosQueues     []QosQueueReport     `json:"qos_queues,omitempty"`
	CarStats      []CarStatReport      `json:"car_stats,omitempty"`
	RadiusServers []RadiusServerReport `json:"radius_servers,omitempty"`
	LLDPNeighbors []LLDPNeighborReport `json:"lldp_neighbors,omitempty"`
}

var bgpPeerStateLabels = map[string]string{
	"1": "idle",
	"2": "connect",
	"3": "active",
	"4": "opensent",
	"5": "openconfirm",
	"6": "established",
}

type storedField struct {
	Key         string          `json:"key"`
	Section     string          `json:"section"`
	OK          bool            `json:"ok"`
	CollectMode string          `json:"collect_mode"`
	OID         string          `json:"oid"`
	Value       json.RawMessage `json:"value,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type storedCollection struct {
	BgpCollection struct {
		Fields map[string]storedField `json:"fields"`
	} `json:"bgp_collection"`
}

// indexSuffix extrai o índice de tabela (parte da OID depois do prefixo base do catálogo).
func indexSuffix(oid, base string) string {
	base = strings.TrimSuffix(strings.TrimSpace(base), ".")
	oid = strings.TrimSpace(oid)
	if base == "" || !strings.HasPrefix(oid, base+".") {
		return ""
	}
	return strings.TrimPrefix(oid, base+".")
}

func walkVars(f storedField) []probing.SNMPVar {
	if f.CollectMode != ModeSNMPWalk || len(f.Value) == 0 {
		return nil
	}
	var vars []probing.SNMPVar
	if err := json.Unmarshal(f.Value, &vars); err != nil {
		return nil
	}
	return vars
}

// lastIPFromSuffix extrai um IPv4 (últimos 4 tokens) de um sufixo de índice composto — usado
// por hwBgpPeerRouteTable, cujo índice real é
// hwBgpPeerInstanceId.Afi.Safi.PeerType.<4 octetos do IP>. Funciona correctamente para o caso
// sem VRF/só IPv4 unicast (o único cenário testado); com VPNv4/múltiplas AFIs a mesma heurística
// pode juntar prefixos ao peer errado — aceitável nesta entrega, documentado no plano.
func lastIPFromSuffix(suffix string) string {
	parts := strings.Split(suffix, ".")
	if len(parts) < 4 {
		return ""
	}
	return strings.Join(parts[len(parts)-4:], ".")
}

// setPeerPrefixCount grava o contador de prefixos (recebidos=1/activos=2/anunciados=3) num
// PeerReport, ignorando valores vazios/não numéricos.
func setPeerPrefixCount(p *PeerReport, raw string, which int) {
	if p == nil || strings.TrimSpace(raw) == "" {
		return
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return
	}
	switch which {
	case 1:
		p.PrefixesReceived = &n
	case 2:
		p.PrefixesActive = &n
	case 3:
		p.PrefixesAdvertised = &n
	}
}

func scalarValue(f storedField) string {
	if f.CollectMode == ModeSNMPWalk || len(f.Value) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(f.Value, &s); err != nil {
		return ""
	}
	return s
}

// BuildReportFromStoredMetrics pivota o JSONB gravado por CollectAndStoreForDevice
// (metrics::text de uma linha de telemetry_samples) em Report.
func BuildReportFromStoredMetrics(raw []byte) Report {
	out := Report{Health: map[string]string{}}
	var stored storedCollection
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out
	}

	peers := map[string]*PeerReport{}
	ifaces := map[string]*InterfaceReport{}

	peer := func(idx string) *PeerReport {
		if p, ok := peers[idx]; ok {
			return p
		}
		p := &PeerReport{PeerIP: idx}
		peers[idx] = p
		return p
	}
	iface := func(idx string) *InterfaceReport {
		if i, ok := ifaces[idx]; ok {
			return i
		}
		i := &InterfaceReport{IfIndex: idx}
		ifaces[idx] = i
		return i
	}

	// pendingPrefixes — os 3 campos de prefixos por peer (hwBgpPeerRouteTable) são processados
	// numa segunda passagem, depois de "peers" já estar totalmente populado pelo bgpPeerTable
	// (bgp_peer_state/remote_as/etc). Isto evita criar linhas de peer "fantasma": o índice real
	// de hwBgpPeerRouteTable também inclui entradas IPv6/VPNv4 (endereço de 16 octetos), onde a
	// heurística "últimos 4 tokens = IP" (lastIPFromSuffix, pensada para IPv4 unicast sem VRF)
	// produz um IP inválido que nunca corresponde a um peer real — nesse caso o contador é
	// simplesmente descartado em vez de virar uma linha de peer nova e incorrecta na tela.
	type pendingPrefix struct {
		ip    string
		value string
		which int
	}
	var pendingPrefixes []pendingPrefix

	for key, f := range stored.BgpCollection.Fields {
		if !f.OK {
			continue
		}
		if f.CollectMode != ModeSNMPWalk {
			// Escalares (secção "saude": cpu_usage/memory_usage/sys_uptime).
			if v := scalarValue(f); v != "" {
				out.Health[key] = v
			}
			continue
		}
		for _, v := range walkVars(f) {
			idx := indexSuffix(v.OID, f.OID)
			if idx == "" {
				continue
			}
			switch key {
			case "bgp_peer_state":
				p := peer(idx)
				p.State = v.Value
				p.StateLabel = bgpPeerStateLabels[v.Value]
			case "bgp_peer_remote_as":
				peer(idx).RemoteAS = v.Value
			case "bgp_peer_established_time":
				p := peer(idx)
				p.EstablishedSeconds, _ = strconv.ParseInt(v.Value, 10, 64)
			case "bgp_peer_in_updates":
				peer(idx).InUpdates = v.Value
			case "bgp_peer_out_updates":
				peer(idx).OutUpdates = v.Value
			case "bgp_peer_prefix_received":
				if ip := lastIPFromSuffix(idx); ip != "" {
					pendingPrefixes = append(pendingPrefixes, pendingPrefix{ip, v.Value, 1})
				}
			case "bgp_peer_prefix_active":
				if ip := lastIPFromSuffix(idx); ip != "" {
					pendingPrefixes = append(pendingPrefixes, pendingPrefix{ip, v.Value, 2})
				}
			case "bgp_peer_prefix_advertised":
				if ip := lastIPFromSuffix(idx); ip != "" {
					pendingPrefixes = append(pendingPrefixes, pendingPrefix{ip, v.Value, 3})
				}
			case "if_descr":
				iface(idx).Descr = v.Value
			case "if_name":
				if iface(idx).Descr == "" {
					iface(idx).Descr = v.Value
				}
			case "if_alias":
				iface(idx).Alias = v.Value
			case "if_oper_status":
				iface(idx).OperStatus = v.Value
			case "if_hc_in_octets":
				iface(idx).HCInOctets = v.Value
			case "if_hc_out_octets":
				iface(idx).HCOutOctets = v.Value
			case "hw_if_in_bit_rate":
				iface(idx).InBitRate = v.Value
			case "hw_if_out_bit_rate":
				iface(idx).OutBitRate = v.Value
			}
		}
	}

	for _, pp := range pendingPrefixes {
		if p, ok := peers[pp.ip]; ok {
			setPeerPrefixCount(p, pp.value, pp.which)
		}
	}

	for _, p := range peers {
		out.Peers = append(out.Peers, *p)
	}
	sort.Slice(out.Peers, func(i, j int) bool { return out.Peers[i].PeerIP < out.Peers[j].PeerIP })

	for _, i := range ifaces {
		out.Interfaces = append(out.Interfaces, *i)
	}
	sort.Slice(out.Interfaces, func(i, j int) bool {
		ni, _ := strconv.Atoi(out.Interfaces[i].IfIndex)
		nj, _ := strconv.Atoi(out.Interfaces[j].IfIndex)
		return ni < nj
	})

	fields := stored.BgpCollection.Fields
	entNames := pivotEntityNames(fields)
	out.Optics = pivotOptics(fields, entNames)
	out.CpuCores = pivotCpuCores(fields)
	out.Fans = pivotFans(fields)
	out.PowerSupplies = pivotPowerSupplies(fields)
	out.Temperatures = pivotTemperatures(fields)
	out.Voltages = pivotVoltages(fields)
	out.BoardAlarms = pivotBoardAlarms(fields, entNames)
	out.VSList = pivotVSList(fields)
	out.VSResources = pivotVSResources(fields)
	out.BFDSessions = pivotBFDSessions(fields)
	out.ETrunks = pivotETrunks(fields)
	out.ETrunkMembers = pivotETrunkMembers(fields)
	ifaceNames := interfaceNameByIndex(out.Interfaces)
	out.QosQueues = pivotQosQueues(fields, ifaceNames)
	out.CarStats = pivotCarStats(fields, ifaceNames)
	out.RadiusServers = pivotRadiusServers(fields)
	out.LLDPNeighbors = pivotLLDPNeighbors(fields, ifaceNames)

	return out
}

// interfaceNameByIndex mapa ifIndex → nome de exibição (descr/alias), a partir das interfaces
// já pivotadas acima — reaproveitado por QoS/LLDP para rotular pelo nome em vez de só o índice.
func interfaceNameByIndex(ifaces []InterfaceReport) map[string]string {
	out := make(map[string]string, len(ifaces))
	for _, i := range ifaces {
		name := i.Descr
		if name == "" {
			name = i.Alias
		}
		if name != "" {
			out[i.IfIndex] = name
		}
	}
	return out
}
