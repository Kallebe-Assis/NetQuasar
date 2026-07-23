package snmpifparse

import (
	"strconv"
	"strings"
)

// FormatAlertIfaceLabel monta o rótulo de interface para alertas/Telegram.
// Prioridade da descrição: custom (utilizador) > ifAlias > vazio.
// Ex.: "ether1 (Uplink-POP1)" ou "sfp-sfpplus1".
func FormatAlertIfaceLabel(ifName, ifAlias, customDescription string) string {
	name := strings.TrimSpace(ifName)
	if name == "" {
		name = strings.TrimSpace(ifAlias)
	}
	desc := strings.TrimSpace(customDescription)
	if desc == "" {
		desc = strings.TrimSpace(ifAlias)
	}
	// Evitar duplicar se o alias for igual ao nome.
	if desc != "" && strings.EqualFold(desc, name) {
		desc = ""
	}
	if name == "" {
		return desc
	}
	if desc == "" {
		return name
	}
	return name + " (" + desc + ")"
}

// PreferIfaceName escolhe o melhor nome SNMP (ifName > DisplayName > descr > ifN).
func PreferIfaceName(ifName, displayName, descr string, ifIndex int) string {
	if s := strings.TrimSpace(ifName); s != "" {
		return s
	}
	if s := strings.TrimSpace(displayName); s != "" {
		return s
	}
	if s := strings.TrimSpace(descr); s != "" {
		return s
	}
	if ifIndex > 0 {
		return "if" + strconv.Itoa(ifIndex)
	}
	return ""
}
