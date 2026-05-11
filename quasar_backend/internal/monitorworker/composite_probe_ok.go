package monitorworker

import "database/sql"

func compositeProbeOK(mode string, reachOK bool, snmp sql.NullBool) bool {
	if mode == ModeSimplePing || mode == ModeOff || mode == "" {
		return reachOK
	}
	snmpPart := true
	if snmp.Valid && !snmp.Bool {
		snmpPart = false
	}
	return reachOK && snmpPart
}
