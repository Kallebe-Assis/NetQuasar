// Package bgpcollect é o análogo BGP de internal/bngcollect: perfil de métricas SNMP
// configurável (Configurações → BGP), com catálogo pré-populado e coleta periódica opcional
// via pipeline de monitorização. Ao contrário do BNG (um único perfil global, id=1 em
// settings_bng_collection), BGP suporta MÚLTIPLOS perfis nomeados (bgp_snmp_profiles, mesmo
// padrão de mikrotik_telnet_profiles/switch_telnet_profiles) — só o perfil is_default=true é
// usado pela coleta periódica nesta entrega; os demais ficam disponíveis para referência/edição
// manual e uma futura selecção por equipamento.
package bgpcollect

import (
	"encoding/json"
	"strings"
)

const (
	ModeSNMPGet  = "snmp_get"
	ModeSNMPWalk = "snmp_walk"
)

// MetricDef configuração de uma métrica BGP.
type MetricDef struct {
	Enabled     bool   `json:"enabled"`
	OID         string `json:"oid"`
	CollectMode string `json:"collect_mode"`
}

// MetricsConfig mapa chave → definição.
type MetricsConfig map[string]MetricDef

// CatalogEntry metadados para UI e coleta.
type CatalogEntry struct {
	Key          string   `json:"key"`
	Section      string   `json:"section"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Placeholder  string   `json:"placeholder"`
	CollectModes []string `json:"collect_modes"`
	DefaultMode  string   `json:"default_mode"`
	Unit         string   `json:"unit,omitempty"`
	Recommended  bool     `json:"recommended,omitempty"`
}

var SectionLabels = map[string]string{
	"saude":       "Saúde do equipamento",
	"interfaces":  "Interfaces (IF-MIB)",
	"trafego":     "Tráfego (contadores + taxa instantânea)",
	"peers":       "Peers BGP (BGP4-MIB)",
	"optica":      "Diagnóstico óptico por porta",
	"chassi":      "Saúde do chassi (ventoinhas/fontes/temperatura/tensão)",
	"vs":          "Virtual System (VS)",
	"bfd":         "Sessões BFD",
	"etrunk":      "E-Trunk (LAG entre equipamentos)",
	"qos":         "QoS — descarte por fila/classe",
	"cpu_nucleos": "CPU por núcleo",
	"radius":      "Saúde do RADIUS",
	"lldp":        "Vizinhos LLDP",
}

// MetricCatalog catálogo de métricas BGP. As entradas de "interfaces"/"trafego" foram
// confirmadas ao vivo (snmpwalk) contra um Huawei NE8000 M8 real nesta sessão — ver
// data/mibs/HUAWEI/...MIB Reference.csv para a documentação oficial dos OIDs IF-MIB/
// HUAWEI-IF-EXT-MIB. As de "peers" são o BGP4-MIB padrão (RFC 1657/4273) — genérico, não
// específico de fabricante, mas não testado ao vivo; "saude" reaproveita como ponto de partida
// os mesmos placeholders de entidade física já usados no perfil BNG (ajustar o índice da placa
// por equipamento). Todo o catálogo é editável na UI — isto é só o perfil "Padrão" inicial.
var MetricCatalog = []CatalogEntry{
	// Saúde — placeholders de entidade física Huawei (mesmo padrão do catálogo BNG); ajustar
	// o índice final (".17367041" no exemplo) para a placa/VS correta em cada equipamento.
	{Key: "cpu_usage", Section: "saude", Label: "CPU (%)", Description: "hwEntityCpuUsage — substitua o índice da placa/VS.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5.17367041", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "%", Recommended: true},
	{Key: "memory_usage", Section: "saude", Label: "Memória (%)", Description: "hwEntityMemUsage — índice da placa/VS.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7.17367041", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "%"},
	{Key: "sys_uptime", Section: "saude", Label: "Uptime", Description: "MIB-2 sysUpTime.", Placeholder: "1.3.6.1.2.1.1.3.0", CollectModes: []string{ModeSNMPGet}, DefaultMode: ModeSNMPGet, Unit: "ticks", Recommended: true},

	// Interfaces — IF-MIB padrão, confirmado nesta sessão contra o NE8000 M8 real do utilizador.
	{Key: "if_descr", Section: "interfaces", Label: "Nome (ifDescr)", Description: "IF-MIB ifDescr — ex.: GigabitEthernet0/1/6, Eth-Trunk10.", Placeholder: "1.3.6.1.2.1.2.2.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "if_name", Section: "interfaces", Label: "Nome curto (ifName)", Description: "IF-MIB ifXTable ifName.", Placeholder: "1.3.6.1.2.1.31.1.1.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "if_alias", Section: "interfaces", Label: "Descrição configurada (ifAlias)", Description: "IF-MIB ifAlias — ex.: OPERADORA-K2-01.", Placeholder: "1.3.6.1.2.1.31.1.1.1.18", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "if_oper_status", Section: "interfaces", Label: "Estado do link (ifOperStatus)", Description: "up(1) / down(2) — testado ao vivo.", Placeholder: "1.3.6.1.2.1.2.2.1.8", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// Tráfego — contadores acumulados (IF-MIB) + taxa instantânea (HUAWEI-IF-EXT-MIB),
	// ambos confirmados ao vivo (snmpwalk) contra o NE8000 M8 do utilizador nesta sessão.
	{Key: "if_hc_in_octets", Section: "trafego", Label: "Octetos Rx acumulados (ifHCInOctets)", Description: "Contador 64-bit — total recebido desde o boot.", Placeholder: "1.3.6.1.2.1.31.1.1.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "if_hc_out_octets", Section: "trafego", Label: "Octetos Tx acumulados (ifHCOutOctets)", Description: "Contador 64-bit — total transmitido desde o boot.", Placeholder: "1.3.6.1.2.1.31.1.1.1.10", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "hw_if_in_bit_rate", Section: "trafego", Label: "Taxa Rx instantânea (hwIFExtInputBitRate)", Description: "Bits/s já calculados no equipamento (Huawei) — mesmo valor do \"Last 300 seconds input rate\" do display interface.", Placeholder: "1.3.6.1.4.1.2011.5.25.41.1.1.1.1.39", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "bps", Recommended: true},
	{Key: "hw_if_out_bit_rate", Section: "trafego", Label: "Taxa Tx instantânea (hwIFExtOutputBitRate)", Description: "Bits/s já calculados no equipamento (Huawei).", Placeholder: "1.3.6.1.4.1.2011.5.25.41.1.1.1.1.40", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "bps", Recommended: true},

	// Peers — BGP4-MIB padrão (RFC 1657/4273), indexado por bgpPeerRemoteAddr.
	{Key: "bgp_peer_state", Section: "peers", Label: "Estado da sessão (bgpPeerState)", Description: "idle(1)…established(6).", Placeholder: "1.3.6.1.2.1.15.3.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bgp_peer_remote_as", Section: "peers", Label: "AS remoto (bgpPeerRemoteAs)", Description: "Sistema autónomo do peer.", Placeholder: "1.3.6.1.2.1.15.3.1.9", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	// established_time/in_updates/out_updates confirmados AO VIVO (POST /devices/{id}/collect)
	// contra o Huawei NE8000 M8 real do utilizador nesta sessão — valores reais retornados
	// (ex.: 2671963s ≈ 30.9 dias, batendo com "0736h28m" do `display bgp peer`). Marcados
	// Recommended para entrarem activos por omissão (antes ficavam no catálogo mas nunca
	// eram colectados por nenhum perfil "Padrão" novo).
	{Key: "bgp_peer_established_time", Section: "peers", Label: "Tempo estabelecido (bgpPeerFsmEstablishedTime)", Description: "Segundos desde a última vez que entrou em Established.", Placeholder: "1.3.6.1.2.1.15.3.1.16", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "s", Recommended: true},
	{Key: "bgp_peer_in_updates", Section: "peers", Label: "Updates recebidos (bgpPeerInUpdates)", Description: "Contador de mensagens UPDATE recebidas.", Placeholder: "1.3.6.1.2.1.15.3.1.10", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bgp_peer_out_updates", Section: "peers", Label: "Updates enviados (bgpPeerOutUpdates)", Description: "Contador de mensagens UPDATE enviadas.", Placeholder: "1.3.6.1.2.1.15.3.1.11", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// Prefixos por peer — HUAWEI-BGP-VPN-MIB hwBgpPeerRouteTable. Índice real da tabela é
	// composto (hwBgpPeerInstanceId.Afi.Safi.PeerType.IP) — para o caso do utilizador (sem VRF,
	// só IPv4 unicast) extraímos os últimos 4 octetos do sufixo como IP do peer, mesma
	// heurística das outras tabelas de peers. Leve (1 linha por peer, não por prefixo) —
	// Recommended.
	{Key: "bgp_peer_prefix_received", Section: "peers", Label: "Prefixos recebidos (hwBgpPeerPrefixRcvCounter)", Description: "Contagem pré-agregada pelo próprio equipamento — não é preciso varrer a tabela de rotas inteira.", Placeholder: "1.3.6.1.4.1.2011.5.25.177.1.1.3.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bgp_peer_prefix_active", Section: "peers", Label: "Prefixos activos (hwBgpPeerPrefixActiveCounter)", Description: "Prefixos recebidos e activos (melhor rota) deste peer.", Placeholder: "1.3.6.1.4.1.2011.5.25.177.1.1.3.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bgp_peer_prefix_advertised", Section: "peers", Label: "Prefixos anunciados (hwBgpPeerPrefixAdvCounter)", Description: "Prefixos enviados a este peer.", Placeholder: "1.3.6.1.4.1.2011.5.25.177.1.1.3.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// Óptica por porta — HUAWEI-ENTITY-EXTENT-MIB hwOpticalModuleInfoTable, índice
	// entPhysicalIndex (confirmado no MIB Reference — "The index of the table is
	// entPhysicalIndex"). entPhysicalName (ENTITY-MIB padrão) é colectado à parte para rotular
	// cada leitura com o nome da porta/placa. Desligado por omissão (pode ter dezenas de
	// portas) — activar em Configurações → BGP.
	{Key: "ent_physical_name", Section: "chassi", Label: "Nome da entidade física (entPhysicalName)", Description: "ENTITY-MIB padrão — usado para rotular óptica/ventoinhas/fontes/sensores pelo nome da porta/placa.", Placeholder: "1.3.6.1.2.1.47.1.1.1.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "optical_rx_power", Section: "optica", Label: "Potência Rx (hwEntityOpticalRxPower)", Description: "Potência óptica recebida por porta.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.3.1.8", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "optical_tx_power", Section: "optica", Label: "Potência Tx (hwEntityOpticalTxPower)", Description: "Potência óptica transmitida por porta.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.3.1.9", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "optical_temperature", Section: "optica", Label: "Temperatura do módulo (hwEntityOpticalTemperature)", Description: "Temperatura do transceiver óptico.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.3.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "optical_voltage", Section: "optica", Label: "Tensão do módulo (hwEntityOpticalVoltage)", Description: "Tensão de alimentação do transceiver.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.3.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "optical_bias_current", Section: "optica", Label: "Corrente do laser (hwEntityOpticalBiasCurrent)", Description: "Corrente de polarização do laser — subir ao longo do tempo é sinal precoce de envelhecimento.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.3.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},

	// Saúde do chassi — HUAWEI-ENTITY-EXTENT-MIB. Ventoinhas/fontes indexadas por
	// slot+número-de-série (legível directamente); temperatura/tensão indexadas por
	// slot-codificado+I2C (descodificado no pivot conforme algoritmo documentado no MIB:
	// 1º dígito hex = chassi, 2 seguintes = slot). "entity_alarm_light" (BITS
	// crítico/major/minor/warning) dá o semáforo por placa — leve, Recommended.
	{Key: "entity_alarm_light", Section: "chassi", Label: "Luz de alarme da placa (hwEntityAlarmLight)", Description: "BITS: crítico/major/minor/aviso — semáforo de hardware por placa.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.1.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "fan_speed", Section: "chassi", Label: "Velocidade da ventoinha (hwEntityFanSpeed)", Description: "RPM.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.10.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "fan_present", Section: "chassi", Label: "Ventoinha presente (hwEntityFanPresent)", Description: "present(1)/absent(2).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.10.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "fan_state", Section: "chassi", Label: "Estado da ventoinha (hwEntityFanState)", Description: "normal(1)/abnormal(2).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.10.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "power_present", Section: "chassi", Label: "Fonte presente (hwEntityPwrPresent)", Description: "present(1)/absent(2).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.18.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "power_state", Section: "chassi", Label: "Estado da fonte (hwEntityPwrState)", Description: "supply(1)/notSupply(2)/sleep(3)/unknown(4).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.18.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "power_current", Section: "chassi", Label: "Corrente da fonte (hwEntityPwrCurrent)", Description: "mA.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.18.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "power_voltage", Section: "chassi", Label: "Tensão da fonte (hwEntityPwrVoltage)", Description: "mV.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.18.1.8", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "temp_status", Section: "chassi", Label: "Estado do sensor de temperatura (hwEntityTempStatus)", Description: "normal(1)/minor(2)/major(3)/fatal(4).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.8.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "temp_value", Section: "chassi", Label: "Temperatura (hwEntityTempValue)", Description: "°C.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.8.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "volt_status", Section: "chassi", Label: "Estado do sensor de tensão (hwEntityVolStatus)", Description: "abnormal(0)/normal(1)/major(2)/fatal(3).", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.9.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "volt_value", Section: "chassi", Label: "Tensão (hwEntityVolCurValue)", Description: "mV.", Placeholder: "1.3.6.1.4.1.2011.5.25.31.1.1.9.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},

	// Virtual System — HUAWEI-VS-MIB. hwVSTable (Id/Nome/Estado por VS, índice hwVSVsId) +
	// hwVSPhysicalResTable (CPU/memória por slot, índice hwVSSlot — não é o VS-ID, por isso
	// aparecem como duas listas separadas no relatório). Leve — Recommended.
	{Key: "vs_id", Section: "vs", Label: "ID da VS (hwVSVsId)", Description: "Índice da virtual-system.", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.1.1.1", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "vs_name", Section: "vs", Label: "Nome da VS (hwVSVsName)", Description: "Nome configurado da virtual-system.", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.1.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "vs_status", Section: "vs", Label: "Estado da VS (hwVSStatus)", Description: "running(1)/stop(2)/restoring(3)/shutdowning(4).", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.1.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "vs_res_cpu", Section: "vs", Label: "CPU por slot (hwVSCPUUsage)", Description: "Uso de CPU da VS nesse slot.", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.2.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "%", Recommended: true},
	{Key: "vs_res_mem_used", Section: "vs", Label: "Memória usada por slot (hwVSMemoryUsedSize)", Description: "Memória usada pela VS nesse slot.", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.2.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "vs_res_mem_total", Section: "vs", Label: "Memória total por slot (hwVSMemoryTotalSize)", Description: "Memória total disponível à VS nesse slot.", Placeholder: "1.3.6.1.4.1.2011.5.25.255.1.2.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// BFD — HUAWEI-BFD-MIB hwBfdSessionTable, índice simples hwBfdSessIndex. Leve — Recommended.
	{Key: "bfd_peer_addr", Section: "bfd", Label: "IP do peer (hwBfdSessPeerAddr)", Description: "Destino da sessão BFD.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bfd_bind_if_name", Section: "bfd", Label: "Interface associada (hwBfdSessBindIfName)", Description: "Interface a que a sessão BFD está associada.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bfd_state", Section: "bfd", Label: "Estado (hwBfdSessState)", Description: "0-admin down / 1-down / 2-init / 3-up.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.17", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bfd_diag", Section: "bfd", Label: "Diagnóstico (hwBfdSessDiag)", Description: "Código do motivo da última mudança de estado.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.18", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bfd_vpn_name", Section: "bfd", Label: "VPN/VRF (hwBfdSessVPNName)", Description: "Instância VPN da sessão, quando aplicável.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.21", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "bfd_down_reason", Section: "bfd", Label: "Motivo da queda (hwBfdSessDownReason)", Description: "Texto legível do motivo — o melhor campo de diagnóstico desta tabela.", Placeholder: "1.3.6.1.4.1.2011.5.25.38.2.3.1.66", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// E-Trunk — HUAWEI-E-TRUNK-MIB. hwETrunkTable (índice hwETrunkId) e hwETrunkMemberTable
	// (índice composto ParentId.Type.Id). Estado master/backup + motivo real (BFD caiu, peer
	// caiu, todos os membros caíram) — coisa que o IF-MIB (só up/down) não dá. Recommended.
	{Key: "etrunk_status", Section: "etrunk", Label: "Estado do E-Trunk (hwETrunkStatus)", Description: "initialize(1)/backup(2)/master(3).", Placeholder: "1.3.6.1.4.1.2011.5.25.178.1.1.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "etrunk_status_reason", Section: "etrunk", Label: "Motivo do estado (hwETrunkStatusReason)", Description: "pri/timeout/bfdDown/peerTimeout/peerBfdDown/allMemberDown/init/peerNodeDown.", Placeholder: "1.3.6.1.4.1.2011.5.25.178.1.1.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "etrunk_member_status", Section: "etrunk", Label: "Estado do membro (hwETrunkMemberStatus)", Description: "backup(1)/master(2).", Placeholder: "1.3.6.1.4.1.2011.5.25.178.1.2.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},
	{Key: "etrunk_member_status_reason", Section: "etrunk", Label: "Motivo do estado do membro (hwETrunkMemberStatusReason)", Description: "peerMemberDown/peerLinkDown/linkDown/etc.", Placeholder: "1.3.6.1.4.1.2011.5.25.178.1.2.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Recommended: true},

	// QoS — HUAWEI-HQOS-MIB (por fila, índice ifIndex.direction.layer1.layer2.queue) +
	// HUAWEI-CBQOS-MIB (por classe/CAR, índice ifIndex.direction.vlan1.vlan2.classe). Índices
	// compostos pesados — só extraímos o ifIndex (1º token) para juntar com o nome da
	// interface; desligado por omissão (pode ter muitas filas/classes).
	{Key: "hqos_queue_forward_bytes", Section: "qos", Label: "Bytes encaminhados na fila (hwhqosQueueForwardBytes)", Description: "Por interface/fila.", Placeholder: "1.3.6.1.4.1.2011.5.25.132.1.1.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "hqos_queue_forward_packets", Section: "qos", Label: "Pacotes encaminhados na fila (hwhqosQueueForwardPackets)", Description: "Por interface/fila.", Placeholder: "1.3.6.1.4.1.2011.5.25.132.1.1.1.6", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "hqos_queue_drop_bytes", Section: "qos", Label: "Bytes descartados na fila (hwhqosQueueDropBytes)", Description: "O indicador de congestionamento por fila.", Placeholder: "1.3.6.1.4.1.2011.5.25.132.1.1.1.9", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "hqos_queue_drop_packets", Section: "qos", Label: "Pacotes descartados na fila (hwhqosQueueDropPackets)", Description: "Por interface/fila.", Placeholder: "1.3.6.1.4.1.2011.5.25.132.1.1.1.8", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "cbqos_car_conformed_bytes", Section: "qos", Label: "Bytes conformados (hwCBQoSCarConformedBytes)", Description: "Tráfego dentro do CIR, por classe.", Placeholder: "1.3.6.1.4.1.2011.5.25.32.1.1.5.6.1.1.12", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "cbqos_car_exceeded_bytes", Section: "qos", Label: "Bytes excedentes (hwCBQoSCarExceededBytes)", Description: "Tráfego entre CIR e PIR, por classe.", Placeholder: "1.3.6.1.4.1.2011.5.25.32.1.1.5.6.1.1.16", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "cbqos_car_dropped_bytes", Section: "qos", Label: "Bytes descartados por CAR (hwCBQoSCarDroppedBytes)", Description: "Descartado pelo policiamento de tráfego, por classe.", Placeholder: "1.3.6.1.4.1.2011.5.25.32.1.1.5.6.1.1.26", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},

	// CPU por núcleo — HUAWEI-CPU-MIB hwMultiCpuDevTable, índice simples hwMultiCpuDevIndex
	// (núcleo). Diferente do placeholder de entidade em "saude" (que é só um valor agregado
	// por placa/VS) — aqui é mesmo por núcleo, com médias 1min/5min. Leve — Recommended.
	{Key: "cpu_core_duty", Section: "cpu_nucleos", Label: "Uso actual do núcleo (hwMultiCpuDuty)", Description: "Amostra dos últimos 5s.", Placeholder: "1.3.6.1.4.1.2011.6.3.33.1.2", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "%", Recommended: true},
	{Key: "cpu_core_avg_1min", Section: "cpu_nucleos", Label: "Média 1 min do núcleo (hwMultiCpuAvgDuty1min)", Description: "", Placeholder: "1.3.6.1.4.1.2011.6.3.33.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "%", Recommended: true},
	{Key: "cpu_core_avg_5min", Section: "cpu_nucleos", Label: "Média 5 min do núcleo (hwMultiCpuAvgDuty5min)", Description: "", Placeholder: "1.3.6.1.4.1.2011.6.3.33.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk, Unit: "%", Recommended: true},

	// RADIUS — HUAWEI-BRAS-RADIUS-MIB. Cada tabela já traz o IP do servidor como parte do
	// próprio índice (hwRadiusStatAuthenIpv4ServerIP/hwRadiusStatAcctIpv4ServerIP), extraído
	// dos 4 primeiros octetos do sufixo — não precisa de juntar com hwRadiusServerTable à
	// parte. Aviso: esta é tipicamente uma função do lado BNG do chassi — pode não responder
	// numa virtual-system dedicada só a BGP. Desligado por omissão.
	{Key: "radius_authen_requests", Section: "radius", Label: "Pedidos de autenticação (hwRadiusStatAuthenIpv4Requests)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.6.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_authen_accepts", Section: "radius", Label: "Aceites (hwRadiusStatAuthenIpv4Accepts)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.6.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_authen_rejects", Section: "radius", Label: "Rejeitados (hwRadiusStatAuthenIpv4Rejects)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.6.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_authen_timeouts", Section: "radius", Label: "Timeouts (hwRadiusStatAuthenIpv4Timeouts)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.6.1.11", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_authen_server_not_reply", Section: "radius", Label: "Sem resposta (hwRadiusStatAuthenIpv4ServerNotReply)", Description: "O indicador mais directo de um servidor RADIUS degradado.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.6.1.18", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_acct_requests", Section: "radius", Label: "Pedidos de accounting (hwRadiusStatAcctIpv4Requests)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.7.1.3", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_acct_responses", Section: "radius", Label: "Respostas de accounting (hwRadiusStatAcctIpv4Responses)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.7.1.4", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_acct_timeouts", Section: "radius", Label: "Timeouts de accounting (hwRadiusStatAcctIpv4Timeouts)", Description: "Por servidor.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.7.1.9", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "radius_acct_server_not_reply", Section: "radius", Label: "Accounting sem resposta (hwRadiusStatAcctIpv4ServerNotReply)", Description: "Falha de accounting = perda de dados de uso/billing mesmo com auth OK.", Placeholder: "1.3.6.1.4.1.2011.5.25.40.15.1.7.1.16", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},

	// LLDP — LLDP-MIB padrão (IEEE 802.1AB) lldpRemTable, índice
	// TimeMark.LocalPortNum.RemIndex — usamos o LocalPortNum (2º token do sufixo) para juntar
	// com o nome da interface local já colectado. Só informativo nesta tela — NÃO alimenta a
	// tela Topologia (o utilizador desenha essa à mão). Desligado por omissão.
	{Key: "lldp_rem_chassis_id", Section: "lldp", Label: "Chassi remoto (lldpRemChassisId)", Description: "Identificador do equipamento do outro lado do link (normalmente MAC).", Placeholder: "1.0.8802.1.1.2.1.4.1.1.5", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "lldp_rem_port_id", Section: "lldp", Label: "Porta remota (lldpRemPortId)", Description: "Identificador da porta do outro lado.", Placeholder: "1.0.8802.1.1.2.1.4.1.1.7", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "lldp_rem_port_desc", Section: "lldp", Label: "Descrição da porta remota (lldpRemPortDesc)", Description: "", Placeholder: "1.0.8802.1.1.2.1.4.1.1.8", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "lldp_rem_sys_name", Section: "lldp", Label: "Nome do sistema remoto (lldpRemSysName)", Description: "Hostname do vizinho.", Placeholder: "1.0.8802.1.1.2.1.4.1.1.9", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
	{Key: "lldp_rem_sys_desc", Section: "lldp", Label: "Descrição do sistema remoto (lldpRemSysDesc)", Description: "Modelo/versão do vizinho.", Placeholder: "1.0.8802.1.1.2.1.4.1.1.10", CollectModes: []string{ModeSNMPWalk}, DefaultMode: ModeSNMPWalk},
}

func DefaultMetrics() MetricsConfig {
	out := make(MetricsConfig, len(MetricCatalog))
	for _, e := range MetricCatalog {
		mode := e.DefaultMode
		if mode == "" {
			mode = ModeSNMPGet
		}
		out[e.Key] = MetricDef{
			Enabled:     e.Recommended,
			OID:         e.Placeholder,
			CollectMode: mode,
		}
	}
	return out
}

func ParseMetrics(raw []byte) MetricsConfig {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var m MetricsConfig
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// Normalize filtra chaves desconhecidas e preenche collect_mode em falta a partir do
// catálogo — mesmo padrão de bngcollect.MetricsConfig.Normalize, chamado antes de gravar um
// perfil vindo da UI.
func (m MetricsConfig) Normalize() MetricsConfig {
	if m == nil {
		return MetricsConfig{}
	}
	out := make(MetricsConfig)
	for _, e := range MetricCatalog {
		if def, ok := m[e.Key]; ok {
			if def.CollectMode == "" {
				def.CollectMode = e.DefaultMode
			}
			if def.CollectMode == "" {
				def.CollectMode = ModeSNMPGet
			}
			out[e.Key] = def
		}
	}
	return out
}

func (m MetricsConfig) MergeWithDefaults() MetricsConfig {
	base := DefaultMetrics()
	for k, v := range m {
		if def, ok := base[k]; ok {
			if v.OID == "" {
				v.OID = def.OID
			}
			if v.CollectMode == "" {
				v.CollectMode = def.CollectMode
			}
			base[k] = v
		}
	}
	return base
}
