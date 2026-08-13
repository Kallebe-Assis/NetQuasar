package networkevents

// Category agrupamento de tipos de evento da rede.
type Category struct {
	Code      string   `json:"code"`
	Label     string   `json:"label"`
	Subgroups []string `json:"subgroups,omitempty"`
	Fields    []string `json:"fields"`
}

// Type tipo concreto de evento (código estável para relatórios).
type Type struct {
	Code         string `json:"code"`
	CategoryCode string `json:"category_code"`
	Subgroup     string `json:"subgroup,omitempty"`
	Label        string `json:"label"`
}

const (
	FieldPop       = "pop"
	FieldDevice    = "device"
	FieldProject   = "project"
	FieldCTO       = "cto"
	FieldCable     = "cable"
	FieldSplice    = "splice"
	FieldPole      = "pole"
	FieldInterface = "interface"
	FieldVLAN      = "vlan"
)

var (
	fieldsPOP      = []string{FieldPop, FieldDevice}
	fieldsOptical  = []string{FieldPop, FieldDevice, FieldInterface}
	fieldsGBIC     = []string{FieldPop, FieldDevice, FieldInterface}
	fieldsCTO      = []string{FieldPop, FieldProject, FieldCTO}
	fieldsCable    = []string{FieldPop, FieldProject, FieldCable}
	fieldsSplice   = []string{FieldPop, FieldProject, FieldSplice}
	fieldsIncident = []string{FieldPop, FieldProject, FieldCable, FieldCTO, FieldSplice}
	fieldsPole     = []string{FieldPop, FieldProject, FieldPole}
	fieldsNet      = []string{FieldPop, FieldDevice, FieldInterface, FieldVLAN}
	fieldsOLT      = []string{FieldPop, FieldDevice, FieldInterface}
	fieldsSec      = []string{FieldDevice}
	fieldsMon      = []string{FieldDevice}
)

// Categories catálogo de categorias (ordem de apresentação).
var Categories = []Category{
	{Code: "pop_maintenance", Label: "Alterações e manutenções no POP", Subgroups: []string{"Equipamentos", "Fontes e energia", "Rack e infraestrutura"}, Fields: fieldsPOP},
	{Code: "pop_optical", Label: "Alterações ópticas no POP", Fields: fieldsOptical},
	{Code: "gbic", Label: "GBIC / SFP / PON / Interfaces", Fields: fieldsGBIC},
	{Code: "ftth_cto", Label: "Rede FTTH — CTOs", Fields: fieldsCTO},
	{Code: "ftth_cable", Label: "Rede FTTH — cabos e fibras", Fields: fieldsCable},
	{Code: "ftth_splice", Label: "Rede FTTH — caixas de emenda e fusões", Fields: fieldsSplice},
	{Code: "incident", Label: "Rompimentos e incidentes físicos", Fields: fieldsIncident},
	{Code: "pole", Label: "Postes e infraestrutura externa", Fields: fieldsPole},
	{Code: "net_config", Label: "Configurações de rede", Fields: fieldsNet},
	{Code: "olt_onu", Label: "Configurações de OLT e ONU", Fields: fieldsOLT},
	{Code: "security", Label: "Usuários, segurança e administração", Fields: fieldsSec},
	{Code: "monitoring", Label: "Monitoramento, automações e backup", Fields: fieldsMon},
}

// Types catálogo de tipos (código estável; não usar o rótulo como chave).
var Types = []Type{
	// POP — equipamentos
	t("pop_maintenance", "Equipamentos", "pop.equip.added", "Adicionado equipamento ao POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.removed", "Removido equipamento do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.replaced", "Substituído equipamento do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.rack_position", "Alterada a posição física do equipamento no rack"),
	t("pop_maintenance", "Equipamentos", "pop.equip.rack_changed", "Alterado o rack onde o equipamento está instalado"),
	t("pop_maintenance", "Equipamentos", "pop.equip.switch_installed", "Instalado novo switch no POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.switch_removed", "Removido switch do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.olt_installed", "Instalada nova OLT no POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.olt_removed", "Removida OLT do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.olt_replaced", "Substituída OLT do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.router_installed", "Instalado novo roteador no POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.router_removed", "Removido roteador do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.router_replaced", "Substituído roteador do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.mikrotik_installed", "Instalada nova MikroTik no POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.mikrotik_removed", "Removida MikroTik do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.odf_installed", "Instalado novo equipamento de distribuição óptica"),
	t("pop_maintenance", "Equipamentos", "pop.equip.odf_removed", "Removido equipamento de distribuição óptica"),
	t("pop_maintenance", "Equipamentos", "pop.equip.contingency_added", "Adicionado equipamento de contingência ao POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.contingency_removed", "Retirado equipamento de contingência do POP"),
	t("pop_maintenance", "Equipamentos", "pop.equip.role_changed", "Alterado equipamento de função dentro do POP"),
	// POP — fontes
	t("pop_maintenance", "Fontes e energia", "pop.power.psu_installed", "Instalada nova fonte de alimentação"),
	t("pop_maintenance", "Fontes e energia", "pop.power.psu_removed", "Removida fonte de alimentação"),
	t("pop_maintenance", "Fontes e energia", "pop.power.psu_replaced", "Substituída fonte de alimentação"),
	t("pop_maintenance", "Fontes e energia", "pop.power.primary_changed", "Alterada a fonte principal do equipamento"),
	t("pop_maintenance", "Fontes e energia", "pop.power.secondary_changed", "Alterada a fonte secundária do equipamento"),
	t("pop_maintenance", "Fontes e energia", "pop.power.mains_connected", "Ligada fonte do equipamento à rede elétrica"),
	t("pop_maintenance", "Fontes e energia", "pop.power.ups_connected", "Ligada fonte do equipamento ao nobreak"),
	t("pop_maintenance", "Fontes e energia", "pop.power.battery_connected", "Ligada fonte do equipamento ao banco de baterias"),
	t("pop_maintenance", "Fontes e energia", "pop.power.mains_to_ups", "Alterada alimentação de rede elétrica para nobreak"),
	t("pop_maintenance", "Fontes e energia", "pop.power.ups_to_battery", "Alterada alimentação de nobreak para banco de baterias"),
	t("pop_maintenance", "Fontes e energia", "pop.power.battery_to_mains", "Alterada alimentação do banco de baterias para rede elétrica"),
	t("pop_maintenance", "Fontes e energia", "pop.power.redundancy_added", "Adicionada redundância de alimentação ao equipamento"),
	t("pop_maintenance", "Fontes e energia", "pop.power.redundancy_removed", "Removida redundância de alimentação do equipamento"),
	t("pop_maintenance", "Fontes e energia", "pop.power.outlet_changed", "Trocada tomada elétrica do equipamento"),
	t("pop_maintenance", "Fontes e energia", "pop.power.circuit_installed", "Instalado novo circuito elétrico no POP"),
	t("pop_maintenance", "Fontes e energia", "pop.power.ups_installed", "Instalado novo nobreak no POP"),
	t("pop_maintenance", "Fontes e energia", "pop.power.ups_removed", "Removido nobreak do POP"),
	t("pop_maintenance", "Fontes e energia", "pop.power.ups_replaced", "Substituído nobreak do POP"),
	t("pop_maintenance", "Fontes e energia", "pop.power.battery_bank_added", "Adicionado banco de baterias ao POP"),
	t("pop_maintenance", "Fontes e energia", "pop.power.battery_bank_removed", "Removido banco de baterias do POP"),
	// POP — rack
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.installed", "Instalado novo rack no POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.removed", "Removido rack do POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.pdu_added", "Adicionada régua de tomadas"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.pdu_removed", "Removida régua de tomadas"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.pdu_replaced", "Substituída régua de tomadas"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.patch_installed", "Instalado novo patch panel"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.patch_removed", "Removido patch panel"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.patch_replaced", "Substituído patch panel"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.dio_installed", "Instalado DIO no POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.dio_removed", "Removido DIO do POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.dio_replaced", "Substituído DIO do POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.organizer_added", "Adicionado organizador de cabos"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.organizer_removed", "Removido organizador de cabos"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.cabling_reorg", "Realizada reorganização do cabeamento do rack"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.cables_labeled", "Realizada identificação física dos cabos do rack"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.equipment_labeled", "Realizada identificação dos equipamentos do rack"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.equip_moved", "Alterada posição de equipamento dentro do rack"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.env_monitor_installed", "Instalado sistema de monitoramento ambiental no POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.access_control_installed", "Instalado sistema de controle de acesso no POP"),
	t("pop_maintenance", "Rack e infraestrutura", "pop.rack.general_maintenance", "Realizada manutenção geral da infraestrutura do POP"),
	// Óptica POP
	t("pop_optical", "", "pop.opt.patchcord_installed", "Instalado novo cordão óptico"),
	t("pop_optical", "", "pop.opt.patchcord_removed", "Removido cordão óptico"),
	t("pop_optical", "", "pop.opt.patchcord_replaced", "Substituído cordão óptico"),
	t("pop_optical", "", "pop.opt.patchcord_iface_changed", "Alterado cordão óptico da interface do equipamento"),
	t("pop_optical", "", "pop.opt.dio_fiber_added", "Adicionada fibra ao DIO"),
	t("pop_optical", "", "pop.opt.dio_fiber_removed", "Removida fibra do DIO"),
	t("pop_optical", "", "pop.opt.dio_fusion", "Realizada fusão de fibra no DIO"),
	t("pop_optical", "", "pop.opt.dio_refusion", "Refusionada fibra existente no DIO"),
	t("pop_optical", "", "pop.opt.dio_fiber_moved", "Alterada posição da fibra no DIO"),
	t("pop_optical", "", "pop.opt.dio_fiber_relabeled", "Alterada identificação da fibra no DIO"),
	t("pop_optical", "", "pop.opt.tray_installed", "Instalada nova bandeja de fusão"),
	t("pop_optical", "", "pop.opt.tray_removed", "Removida bandeja de fusão"),
	t("pop_optical", "", "pop.opt.tray_reorg", "Reorganizada bandeja de fusão"),
	t("pop_optical", "", "pop.opt.equip_route_changed", "Alterada rota óptica de equipamento"),
	t("pop_optical", "", "pop.opt.backbone_fiber_maint", "Realizada manutenção em fibra de backbone"),
	t("pop_optical", "", "pop.opt.uplink_fiber_maint", "Realizada manutenção em fibra de uplink"),
	t("pop_optical", "", "pop.opt.dist_fiber_maint", "Realizada manutenção em fibra de distribuição"),
	t("pop_optical", "", "pop.opt.backbone_fiber_replaced", "Substituída fibra óptica de backbone"),
	t("pop_optical", "", "pop.opt.uplink_fiber_replaced", "Substituída fibra óptica de uplink"),
	t("pop_optical", "", "pop.opt.backbone_unattenuated", "Desatenuada fibra óptica do backbone"),
	t("pop_optical", "", "pop.opt.attenuation_adjusted", "Readequada atenuação da fibra óptica"),
	t("pop_optical", "", "pop.opt.port_changed", "Alterada porta óptica utilizada pelo equipamento"),
	t("pop_optical", "", "pop.opt.xcvr_changed", "Alterado transceptor óptico da interface"),
	t("pop_optical", "", "pop.opt.xcvr_added", "Adicionado transceptor óptico"),
	t("pop_optical", "", "pop.opt.xcvr_removed", "Removido transceptor óptico"),
	// GBIC / SFP
	t("gbic", "", "gbic.installed", "Instalado novo GBIC/SFP"),
	t("gbic", "", "gbic.removed", "Removido GBIC/SFP"),
	t("gbic", "", "gbic.replaced", "Substituído GBIC/SFP"),
	t("gbic", "", "gbic.pon_replaced", "Trocado GBIC da PON"),
	t("gbic", "", "gbic.uplink_replaced", "Trocado GBIC da interface de uplink"),
	t("gbic", "", "gbic.model_changed", "Alterado modelo de transceptor óptico"),
	t("gbic", "", "gbic.wavelength_changed", "Alterado comprimento de onda do transceptor"),
	t("gbic", "", "gbic.uplink_iface_changed", "Alterada interface utilizada para uplink"),
	t("gbic", "", "gbic.downlink_iface_changed", "Alterada interface utilizada para downlink"),
	t("gbic", "", "gbic.uplink_iface_added", "Adicionada nova interface de uplink"),
	t("gbic", "", "gbic.uplink_iface_removed", "Removida interface de uplink"),
	t("gbic", "", "gbic.speed_changed", "Alterada velocidade da interface"),
	t("gbic", "", "gbic.mode_changed", "Alterado modo de operação da interface"),
	t("gbic", "", "gbic.iface_enabled", "Ativada interface física"),
	t("gbic", "", "gbic.iface_disabled", "Desativada interface física"),
	// CTO
	t("ftth_cto", "", "ftth.cto.installed", "Instalada nova CTO"),
	t("ftth_cto", "", "ftth.cto.removed", "Removida CTO"),
	t("ftth_cto", "", "ftth.cto.replaced", "Substituída CTO"),
	t("ftth_cto", "", "ftth.cto.location_changed", "Alterada localização da CTO"),
	t("ftth_cto", "", "ftth.cto.id_changed", "Alterada identificação da CTO"),
	t("ftth_cto", "", "ftth.cto.model_changed", "Alterado modelo da CTO"),
	t("ftth_cto", "", "ftth.cto.capacity_changed", "Alterada capacidade da CTO"),
	t("ftth_cto", "", "ftth.cto.splitter_added", "Adicionado splitter à CTO"),
	t("ftth_cto", "", "ftth.cto.splitter_removed", "Removido splitter da CTO"),
	t("ftth_cto", "", "ftth.cto.splitter_replaced", "Substituído splitter da CTO"),
	t("ftth_cto", "", "ftth.cto.splitter_model_changed", "Alterado modelo do splitter"),
	t("ftth_cto", "", "ftth.cto.in_fiber_changed", "Alterada fibra de entrada da CTO"),
	t("ftth_cto", "", "ftth.cto.splitter_in_port_changed", "Alterada porta de entrada do splitter"),
	t("ftth_cto", "", "ftth.cto.splitter_out_fiber_changed", "Alterada fibra de saída do splitter"),
	t("ftth_cto", "", "ftth.cto.splitter_dest_changed", "Alterado destino de fibra do splitter"),
	t("ftth_cto", "", "ftth.cto.reserve_added", "Adicionada reserva técnica na CTO"),
	t("ftth_cto", "", "ftth.cto.reserve_removed", "Removida reserva técnica da CTO"),
	t("ftth_cto", "", "ftth.cto.client_port_added", "Adicionado cliente à porta do splitter"),
	t("ftth_cto", "", "ftth.cto.client_port_removed", "Removido cliente da porta do splitter"),
	t("ftth_cto", "", "ftth.cto.client_port_changed", "Alterada porta do cliente no splitter"),
	// Cabos
	t("ftth_cable", "", "ftth.cable.installed", "Instalado novo cabo óptico"),
	t("ftth_cable", "", "ftth.cable.removed", "Removido cabo óptico"),
	t("ftth_cable", "", "ftth.cable.span_replaced", "Substituído trecho de cabo óptico"),
	t("ftth_cable", "", "ftth.cable.route_changed", "Alterada rota do cabo óptico"),
	t("ftth_cable", "", "ftth.cable.span_added", "Adicionado novo trecho de cabo"),
	t("ftth_cable", "", "ftth.cable.span_removed", "Removido trecho de cabo"),
	t("ftth_cable", "", "ftth.cable.model_changed", "Alterado modelo do cabo óptico"),
	t("ftth_cable", "", "ftth.cable.fiber_count_changed", "Alterada quantidade de fibras do cabo"),
	t("ftth_cable", "", "ftth.cable.id_changed", "Alterada identificação do cabo"),
	t("ftth_cable", "", "ftth.cable.origin_changed", "Alterada origem do cabo"),
	t("ftth_cable", "", "ftth.cable.dest_changed", "Alterado destino do cabo"),
	t("ftth_cable", "", "ftth.fiber.added", "Adicionada nova fibra ao projeto"),
	t("ftth_cable", "", "ftth.fiber.removed", "Removida fibra do projeto"),
	t("ftth_cable", "", "ftth.fiber.status_changed", "Alterado status da fibra"),
	t("ftth_cable", "", "ftth.fiber.usage_changed", "Alterada utilização da fibra"),
	t("ftth_cable", "", "ftth.fiber.dest_changed", "Alterado destino da fibra"),
	t("ftth_cable", "", "ftth.fiber.reserved", "Reservada fibra óptica"),
	t("ftth_cable", "", "ftth.fiber.released", "Liberada fibra óptica"),
	t("ftth_cable", "", "ftth.fiber.color_changed", "Alterada cor/identificação da fibra"),
	t("ftth_cable", "", "ftth.fiber.number_changed", "Alterada numeração da fibra"),
	// Emendas
	t("ftth_splice", "", "ftth.splice.installed", "Instalada nova caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.removed", "Removida caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.replaced", "Substituída caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.location_changed", "Alterada localização da caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.id_changed", "Alterada identificação da caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.cable_added", "Adicionado cabo à caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.cable_removed", "Removido cabo da caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.fusion_new", "Realizada nova fusão óptica"),
	t("ftth_splice", "", "ftth.splice.fusion_redone", "Refusionada fibra óptica"),
	t("ftth_splice", "", "ftth.splice.fusion_removed", "Removida fusão óptica"),
	t("ftth_splice", "", "ftth.splice.fiber_map_changed", "Alterada correspondência entre fibras"),
	t("ftth_splice", "", "ftth.splice.tray_changed", "Alterada bandeja de fusão"),
	t("ftth_splice", "", "ftth.splice.tray_pos_changed", "Alterada posição da fibra na bandeja"),
	t("ftth_splice", "", "ftth.splice.reserve_added", "Criada reserva técnica na caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.reserve_removed", "Removida reserva técnica da caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.reorg", "Reorganizada caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.preventive", "Realizada manutenção preventiva na caixa de emenda"),
	t("ftth_splice", "", "ftth.splice.corrective", "Realizada manutenção corretiva na caixa de emenda"),
	// Incidentes
	t("incident", "", "incident.cable_cut", "Rompimento de cabo óptico"),
	t("incident", "", "incident.fiber_cut", "Rompimento de fibra óptica"),
	t("incident", "", "incident.backbone_cut", "Rompimento de cabo de backbone"),
	t("incident", "", "incident.distribution_cut", "Rompimento de cabo de distribuição"),
	t("incident", "", "incident.third_party_works", "Rompimento causado por obra de terceiros"),
	t("incident", "", "incident.traffic_accident", "Rompimento causado por acidente de trânsito"),
	t("incident", "", "incident.pole_fall", "Rompimento causado por queda de poste"),
	t("incident", "", "incident.tree", "Rompimento causado por árvore"),
	t("incident", "", "incident.weather", "Rompimento causado por intempérie"),
	t("incident", "", "incident.vandalism", "Rompimento causado por vandalismo"),
	t("incident", "", "incident.third_party_maint", "Rompimento causado por manutenção de terceiros"),
	t("incident", "", "incident.cable_recovered", "Realizada recuperação de cabo rompido"),
	t("incident", "", "incident.span_replaced", "Realizada substituição de trecho após rompimento"),
	t("incident", "", "incident.fusion_after_cut", "Realizada nova fusão após rompimento"),
	t("incident", "", "incident.emergency_reroute", "Realizado desvio emergencial de fibra"),
	t("incident", "", "incident.emergency_client_move", "Realizada transferência emergencial de clientes"),
	t("incident", "", "incident.cto_recovered", "Realizada recuperação de CTO após incidente"),
	t("incident", "", "incident.splice_recovered", "Realizada recuperação de caixa de emenda após incidente"),
	// Postes
	t("pole", "", "pole.registered", "Cadastrado novo poste"),
	t("pole", "", "pole.removed", "Removido poste do projeto"),
	t("pole", "", "pole.location_changed", "Alterada localização do poste"),
	t("pole", "", "pole.type_changed", "Alterado tipo de poste"),
	t("pole", "", "pole.cto_support_installed", "Instalado novo suporte de CTO no poste"),
	t("pole", "", "pole.cto_support_removed", "Removido suporte de CTO"),
	t("pole", "", "pole.cable_support_installed", "Instalado novo suporte de cabo"),
	t("pole", "", "pole.cable_support_removed", "Removido suporte de cabo"),
	t("pole", "", "pole.cable_fix_changed", "Alterada fixação do cabo no poste"),
	t("pole", "", "pole.cables_reorg", "Reorganizados cabos no poste"),
	t("pole", "", "pole.reserve_added", "Adicionada reserva técnica no poste"),
	t("pole", "", "pole.reserve_removed", "Removida reserva técnica do poste"),
	t("pole", "", "pole.external_works", "Alterada infraestrutura devido a obra externa"),
	t("pole", "", "pole.preventive", "Realizada manutenção preventiva na infraestrutura externa"),
	// Config rede
	t("net_config", "", "net.vlan.added", "Adicionada VLAN ao equipamento"),
	t("net_config", "", "net.vlan.removed", "Removida VLAN do equipamento"),
	t("net_config", "", "net.vlan.iface_changed", "Alterada VLAN da interface"),
	t("net_config", "", "net.vlan.mgmt_changed", "Alterada VLAN de gerenciamento"),
	t("net_config", "", "net.vlan.client_changed", "Alterada VLAN de cliente"),
	t("net_config", "", "net.vlan.service_changed", "Alterada VLAN de serviço"),
	t("net_config", "", "net.bridge.created", "Criada nova bridge"),
	t("net_config", "", "net.bridge.removed", "Removida bridge"),
	t("net_config", "", "net.bridge.changed", "Alterada configuração de bridge"),
	t("net_config", "", "net.trunk.changed", "Alterada configuração de trunk"),
	t("net_config", "", "net.access.changed", "Alterada configuração de access"),
	t("net_config", "", "net.bonding.changed", "Alterada configuração de bonding"),
	t("net_config", "", "net.lag.created", "Criado novo LAG"),
	t("net_config", "", "net.lag.removed", "Removido LAG"),
	t("net_config", "", "net.lag.changed", "Alterada configuração de LAG"),
	t("net_config", "", "net.ip.iface_changed", "Alterado endereço IP da interface"),
	t("net_config", "", "net.ip.added", "Adicionado novo endereço IP"),
	t("net_config", "", "net.ip.removed", "Removido endereço IP"),
	t("net_config", "", "net.ip.mask_changed", "Alterada máscara de rede"),
	t("net_config", "", "net.ip.gateway_changed", "Alterado gateway"),
	t("net_config", "", "net.route.changed", "Alterada rota estática"),
	t("net_config", "", "net.route.added", "Adicionada rota estática"),
	t("net_config", "", "net.route.removed", "Removida rota estática"),
	t("net_config", "", "net.routing_proto_changed", "Alterado protocolo de roteamento"),
	t("net_config", "", "net.bgp.changed", "Alterada configuração BGP"),
	t("net_config", "", "net.ospf.changed", "Alterada configuração OSPF"),
	t("net_config", "", "net.mpls.changed", "Alterada configuração MPLS"),
	t("net_config", "", "net.qos.changed", "Alterada configuração de QoS"),
	t("net_config", "", "net.bandwidth_limit_changed", "Alterado limite de banda"),
	t("net_config", "", "net.firewall.changed", "Alterada política de firewall"),
	// OLT / ONU
	t("olt_onu", "", "olt.profile.changed", "Alterado perfil da OLT"),
	t("olt_onu", "", "olt.profile.added", "Adicionado novo perfil de OLT"),
	t("olt_onu", "", "olt.snmp_oid.changed", "Alterado OID SNMP da OLT"),
	t("olt_onu", "", "olt.snmp_oid.added", "Adicionado novo OID SNMP"),
	t("olt_onu", "", "olt.telnet_cmd.changed", "Alterado comando Telnet da OLT"),
	t("olt_onu", "", "olt.onu_prov_cmd.changed", "Alterado comando de provisionamento de ONU"),
	t("olt_onu", "", "olt.onu_deprov_cmd.changed", "Alterado comando de desprovisionamento de ONU"),
	t("olt_onu", "", "olt.onu_prov_method.changed", "Alterado método de provisionamento de ONU"),
	t("olt_onu", "", "olt.onu_prov_manual_to_auto", "Alterado provisionamento manual para automático"),
	t("olt_onu", "", "olt.onu_prov_auto_to_manual", "Alterado provisionamento automático para manual"),
	t("olt_onu", "", "olt.onu_profile.changed", "Alterado perfil de ONU"),
	t("olt_onu", "", "olt.onu_rule.added", "Adicionada nova regra de provisionamento"),
	t("olt_onu", "", "olt.onu_rule.removed", "Removida regra de provisionamento"),
	t("olt_onu", "", "olt.onu_template.changed", "Alterado template de configuração de ONU"),
	t("olt_onu", "", "olt.onu_vlan.changed", "Alterado VLAN de ONU"),
	t("olt_onu", "", "olt.onu_speed.changed", "Alterado velocidade/perfil de banda da ONU"),
	t("olt_onu", "", "olt.onu_service.changed", "Alterado perfil de serviço da ONU"),
	t("olt_onu", "", "olt.onu_pon.changed", "Alterada PON utilizada pela ONU"),
	t("olt_onu", "", "olt.onu_logical_pos.changed", "Alterada posição lógica da ONU"),
	t("olt_onu", "", "olt.onu_id_method.changed", "Alterado método de identificação da ONU"),
	// Segurança
	t("security", "", "sec.device_user.created", "Criado usuário no equipamento"),
	t("security", "", "sec.device_user.removed", "Removido usuário do equipamento"),
	t("security", "", "sec.device_user.password", "Alterada senha de usuário"),
	t("security", "", "sec.device_user.privilege", "Alterado nível de privilégio do usuário"),
	t("security", "", "sec.app_user.created", "Criado usuário no NetQuasar"),
	t("security", "", "sec.app_user.removed", "Removido usuário do NetQuasar"),
	t("security", "", "sec.app_user.profile", "Alterado perfil de acesso do usuário"),
	t("security", "", "sec.app_user.permission", "Alterada permissão de acesso"),
	t("security", "", "sec.auth_policy.changed", "Alterada política de autenticação"),
	t("security", "", "sec.remote_access.changed", "Alterada configuração de acesso remoto"),
	t("security", "", "sec.mgmt_port.changed", "Alterada porta de gerenciamento"),
	t("security", "", "sec.ssh.changed", "Alterada configuração SSH"),
	t("security", "", "sec.telnet.changed", "Alterada configuração Telnet"),
	t("security", "", "sec.snmp.changed", "Alterada configuração SNMP"),
	t("security", "", "sec.snmp_community.changed", "Alterada comunidade SNMP"),
	t("security", "", "sec.device_credential.changed", "Alterada credencial de acesso ao equipamento"),
	t("security", "", "sec.monitor.added", "Adicionado equipamento ao monitoramento"),
	t("security", "", "sec.monitor.removed", "Removido equipamento do monitoramento"),
	t("security", "", "sec.monitor.interval.changed", "Alterado intervalo de monitoramento"),
	// Monitoramento
	t("monitoring", "", "mon.automation.created", "Criada nova automação"),
	t("monitoring", "", "mon.automation.changed", "Alterada automação existente"),
	t("monitoring", "", "mon.automation.removed", "Removida automação"),
	t("monitoring", "", "mon.automation.schedule.changed", "Alterado horário de execução da automação"),
	t("monitoring", "", "mon.automation.recurrence.changed", "Alterada recorrência da automação"),
	t("monitoring", "", "mon.collect.interval.changed", "Alterado intervalo de coleta"),
	t("monitoring", "", "mon.snmp.enabled", "Ativada coleta SNMP"),
	t("monitoring", "", "mon.snmp.disabled", "Desativada coleta SNMP"),
	t("monitoring", "", "mon.ping.enabled", "Ativada coleta de ping"),
	t("monitoring", "", "mon.ping.disabled", "Desativada coleta de ping"),
	t("monitoring", "", "mon.ifaces.enabled", "Ativada coleta de interfaces"),
	t("monitoring", "", "mon.ifaces.disabled", "Desativada coleta de interfaces"),
	t("monitoring", "", "mon.pons.enabled", "Ativada coleta de PONs"),
	t("monitoring", "", "mon.pons.disabled", "Desativada coleta de PONs"),
	t("monitoring", "", "mon.onus.enabled", "Ativada coleta de ONUs"),
	t("monitoring", "", "mon.onus.disabled", "Desativada coleta de ONUs"),
	t("monitoring", "", "mon.backup.auto.created", "Criada rotina de backup automático"),
	t("monitoring", "", "mon.backup.changed", "Alterada rotina de backup"),
	t("monitoring", "", "mon.backup.dest.changed", "Alterado destino do backup"),
	t("monitoring", "", "mon.backup.retention.changed", "Alterada política de retenção de backups"),
	t("monitoring", "", "mon.backup.manual", "Realizado backup manual"),
	t("monitoring", "", "mon.backup.restore", "Realizado restore do banco de dados"),
	t("monitoring", "", "mon.backup.cloud.changed", "Alterada configuração de armazenamento em nuvem"),
}

func t(cat, sub, code, label string) Type {
	return Type{Code: code, CategoryCode: cat, Subgroup: sub, Label: label}
}

// TypeByCode devolve o tipo ou nil.
func TypeByCode(code string) *Type {
	for i := range Types {
		if Types[i].Code == code {
			return &Types[i]
		}
	}
	return nil
}

// CategoryByCode devolve a categoria ou nil.
func CategoryByCode(code string) *Category {
	for i := range Categories {
		if Categories[i].Code == code {
			return &Categories[i]
		}
	}
	return nil
}

// ValidImpact valores aceites para impacto.
func ValidImpact(s string) bool {
	switch s {
	case "none", "low", "medium", "high", "critical":
		return true
	}
	return false
}
