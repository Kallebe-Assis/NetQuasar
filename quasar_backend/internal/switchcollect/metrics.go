package switchcollect

import (
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

// SwitchSectionLabels secções do catálogo Switch (SNMP padrão / Cisco).
var SwitchSectionLabels = map[string]string{
	"system":     "Sistema",
	"health":     "Saúde",
	"interfaces": "Interfaces",
	"inventory":  "Inventário (ENTITY-MIB)",
	"optical":    "Óptica / transceiver",
}

// SwitchMetricCatalog métricas SNMP para switches (IF-MIB + Cisco ENTITY/PROCESS).
// OIDs alinhados à saída snmpwalk do Nexus 5000 (switch_cisco.txt).
var SwitchMetricCatalog = []mikrotikcollect.CatalogEntry{
	// Sistema — SNMPv2-MIB (confirmado no walk)
	{Key: "sys_descr", Section: "system", Label: "Descrição (sysDescr)", Description: "MIB-2 — modelo, software e build (ex.: Cisco NX-OS n5000).", Placeholder: "1.3.6.1.2.1.1.1.0", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry},
	{Key: "sys_uptime", Section: "system", Label: "Uptime", Description: "sysUpTimeInstance / sysUpTime.0 — timeticks desde o boot.", Placeholder: "1.3.6.1.2.1.1.3.0", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "ticks"},
	{Key: "sys_name", Section: "system", Label: "Nome (sysName)", Description: "Hostname configurado no equipamento.", Placeholder: "1.3.6.1.2.1.1.5.0", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry},

	// Inventário — ENTITY-MIB (serial, modelo, software)
	{Key: "serial_number", Section: "inventory", Label: "Número de série", Description: "entPhysicalSerialNum — primeiro módulo/chassis.", Placeholder: "1.3.6.1.2.1.47.1.1.1.1.11.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry},
	{Key: "firmware_version", Section: "inventory", Label: "Versão software", Description: "entPhysicalSoftwareRev ou versão no sysDescr.", Placeholder: "1.3.6.1.2.1.47.1.1.1.1.10.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry},
	{Key: "board_name", Section: "inventory", Label: "Modelo / board", Description: "entPhysicalModelName — modelo do chassis.", Placeholder: "1.3.6.1.2.1.47.1.1.1.1.13.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry},

	// Saúde — CISCO-PROCESS-MIB (confirmado em walk 10.100.120.2)
	{Key: "cpu_load", Section: "health", Label: "CPU (%)", Description: "cpmCPUTotal5min (.8) — média 5 min. Alternativas: .6 (5 s), .7 (1 min). Valor já em %.", Placeholder: "1.3.6.1.4.1.9.9.109.1.1.1.1.8.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "%"},
	{Key: "cpu_hr", Section: "health", Label: "CPU HOST-RESOURCES", Description: "hrProcessorLoad — alternativa quando CISCO-PROCESS-MIB não estiver disponível.", Placeholder: "1.3.6.1.2.1.25.3.3.1.2.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "%"},
	{Key: "memory_used", Section: "health", Label: "Memória usada", Description: "cpmCPUMemoryUsed (.12) — KB de RAM em uso (índice 1).", Placeholder: "1.3.6.1.4.1.9.9.109.1.1.1.1.12.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "KB"},
	{Key: "memory_free", Section: "health", Label: "Memória livre", Description: "cpmCPUMemoryFree (.13) — KB livres. Total = usada + livre.", Placeholder: "1.3.6.1.4.1.9.9.109.1.1.1.1.13.1", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "KB"},
	{Key: "memory_total", Section: "health", Label: "Memória total", Description: "Opcional — hrMemorySize ou outro OID escalar. Se vazio, usa usada+livre.", Placeholder: "1.3.6.1.2.1.25.2.2.0", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "KB"},
	{Key: "temperature", Section: "health", Label: "Temperatura", Description: "entSensorValue (.4) — índice do sensor varia por modelo (ex.: 21590 → 33 °C).", Placeholder: "1.3.6.1.4.1.9.9.91.1.1.1.1.4.21590", CollectModes: []string{mikrotikcollect.ModeSNMPGet}, DefaultMode: mikrotikcollect.ModeSNMPGet, WalkTarget: mikrotikcollect.TargetTelemetry, Unit: "°C"},

	// Interfaces — IF-MIB (confirmado: ifDescr, ifName, ifOperStatus, ifHCInOctets)
	{Key: "if_mib_table", Section: "interfaces", Label: "IF-MIB (ifTable)", Description: "Walk ifTable — descrição, status e contadores por interface.", Placeholder: "1.3.6.1.2.1.2.2.1", CollectModes: []string{mikrotikcollect.ModeSNMPWalk, mikrotikcollect.ModeIFMibTable}, DefaultMode: mikrotikcollect.ModeIFMibTable, WalkTarget: mikrotikcollect.TargetInterfaces},
	{Key: "if_x_table", Section: "interfaces", Label: "IF-MIB estendido (ifXTable)", Description: "Walk ifXTable — ifName, ifAlias e contadores 64-bit (ifHCInOctets / ifHCOutOctets).", Placeholder: "1.3.6.1.2.1.31.1.1.1", CollectModes: []string{mikrotikcollect.ModeSNMPWalk, mikrotikcollect.ModeIFMibTable}, DefaultMode: mikrotikcollect.ModeIFMibTable, WalkTarget: mikrotikcollect.TargetInterfaces},
	{Key: "if_oper_status", Section: "interfaces", Label: "Status operacional (ifOperStatus)", Description: "Estado da interface: up(1), down(2), etc.", Placeholder: mikrotikcollect.IFOperStatusOID, CollectModes: []string{mikrotikcollect.ModeIFMibStatus, mikrotikcollect.ModeSNMPWalk, mikrotikcollect.ModeIFMibTable}, DefaultMode: mikrotikcollect.ModeIFMibStatus, WalkTarget: mikrotikcollect.TargetInterfaces, IFMibColumn: mikrotikcollect.IFColOperStatus},
	{Key: "if_admin_status", Section: "interfaces", Label: "Status administrativo (ifAdminStatus)", Description: "Estado admin da interface. 1=up, 2=down, 3=testing.", Placeholder: mikrotikcollect.IFAdminStatusOID, CollectModes: []string{mikrotikcollect.ModeIFMibStatus, mikrotikcollect.ModeSNMPWalk}, DefaultMode: mikrotikcollect.ModeIFMibStatus, WalkTarget: mikrotikcollect.TargetInterfaces, IFMibColumn: mikrotikcollect.IFColAdminStatus},
	{Key: "vlan_membership_table", Section: "interfaces", Label: "VLAN por porta (vmVlan)", Description: "CISCO-VLAN-MEMBERSHIP-MIB vlanMembershipPortTable — tipo (access/trunk), VLAN nativa e lista em trunk.", Placeholder: "1.3.6.1.4.1.9.9.68.1.2.2.1", CollectModes: []string{mikrotikcollect.ModeSNMPWalk}, DefaultMode: mikrotikcollect.ModeSNMPWalk, WalkTarget: mikrotikcollect.TargetInterfaces},
	{Key: "vlan_name_table", Section: "interfaces", Label: "Nomes VLAN (VTP)", Description: "CISCO-VTP-MIB vtpVlanName — nomes das VLANs para exibição.", Placeholder: "1.3.6.1.4.1.9.9.46.1.3.1.1.4", CollectModes: []string{mikrotikcollect.ModeSNMPWalk}, DefaultMode: mikrotikcollect.ModeSNMPWalk, WalkTarget: mikrotikcollect.TargetInterfaces},

	// Óptica — opcional (walk ENTITY-SENSOR; desactivado por defeito)
	{Key: "optical_table", Section: "optical", Label: "Sensores ENTITY (walk)", Description: "Walk entSensorTable — temperatura/voltagem de módulos (quando suportado).", Placeholder: "1.3.6.1.4.1.9.9.91.1.1.1.1", CollectModes: []string{mikrotikcollect.ModeSNMPWalk}, DefaultMode: mikrotikcollect.ModeSNMPWalk, WalkTarget: mikrotikcollect.TargetInterfaces},
}

func defaultSwitchEnabled(key string) bool {
	switch key {
	case "sys_descr", "sys_uptime", "sys_name",
		"cpu_load", "memory_used", "memory_free", "temperature",
		"if_mib_table", "if_x_table", "if_oper_status",
		"vlan_membership_table", "vlan_name_table":
		return true
	default:
		return false
	}
}

// DefaultSwitchMetrics perfil inicial para switches Cisco / IF-MIB padrão.
func DefaultSwitchMetrics() mikrotikcollect.MetricsConfig {
	return mikrotikcollect.DefaultMetricsForCatalog(SwitchMetricCatalog, defaultSwitchEnabled)
}
