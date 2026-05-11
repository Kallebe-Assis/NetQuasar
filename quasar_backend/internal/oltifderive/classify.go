package oltifderive

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const ifOctets32Max = 2147483647

// Kind classifica interfaces típicas de OLT (VSOL / IF-MIB).
type Kind string

const (
	KindManagement Kind = "ge_vlan" // GE uplinks e interfaces de gestão
	KindVLAN       Kind = "vlan"
	KindPON        Kind = "pon"
	KindONU        Kind = "onu"
	KindOther      Kind = "other"
)

var (
	rePonPhy      = regexp.MustCompile(`(?i)^GPON(\d+)/(\d+)`)
	reOnuIface    = regexp.MustCompile(`(?i)^GPON(\d+)ONU(\d+)`)
	rePonPhyZTE   = regexp.MustCompile(`(?i)^PON-(\d+)/(\d+)/(\d+)`)
	rePonPhyZTEIf = regexp.MustCompile(`(?i)^GPON_OLT-(\d+)/(\d+)/(\d+)`)
	reOnuIfaceZTE = regexp.MustCompile(`(?i)^(?:GPON-ONU_|EPON-ONU_|ONU-)(\d+)/(\d+)/(\d+):(\d+)`)
	reVlan        = regexp.MustCompile(`(?i)^VLAN\d+`)
	reGE          = regexp.MustCompile(`(?i)^GE\d+/\d+`)
	reVsolPonName = regexp.MustCompile(`(?i)^PON\s+(\d+)\s*$`)
)

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Fields(s)[0]
}

// ClassifyKind por ifName/ifDescr (ex.: GE0/1, VLAN500, GPON0/1, GPON01ONU2).
func ClassifyKind(displayName, descr string) Kind {
	tok := firstToken(displayName)
	if tok == "" {
		tok = firstToken(descr)
	}
	if tok == "" {
		return KindOther
	}
	up := strings.ToUpper(tok)
	if reVlan.MatchString(up) {
		return KindVLAN
	}
	if reGE.MatchString(up) {
		return KindManagement
	}
	if reOnuIface.MatchString(tok) {
		return KindONU
	}
	if reOnuIfaceZTE.MatchString(tok) {
		return KindONU
	}
	if rePonPhy.MatchString(tok) {
		return KindPON
	}
	if rePonPhyZTE.MatchString(tok) {
		return KindPON
	}
	if rePonPhyZTEIf.MatchString(tok) {
		return KindPON
	}
	// GE sem barra ou outras VLAN por nome
	if strings.HasPrefix(up, "GE") {
		return KindManagement
	}
	if strings.HasPrefix(up, "VLAN") {
		return KindVLAN
	}
	return KindOther
}

// CanonicalPonRowKey alinha linhas VSOL (ex.: name "PON 1", id "1") com IF-MIB ("GPON0/1", id "01").
func CanonicalPonRowKey(m map[string]any) string {
	name := strings.TrimSpace(fmt.Sprint(m["name"]))
	idStr := strings.TrimSpace(fmt.Sprint(m["id"]))
	if c := PonCompactFromPhy(name, name); c != "" {
		return c
	}
	if c := PonCompactFromPhy(idStr, idStr); c != "" {
		return c
	}
	if sm := reVsolPonName.FindStringSubmatch(name); len(sm) == 2 {
		if n, err := strconv.Atoi(sm[1]); err == nil && n > 0 {
			return "0" + strconv.Itoa(n)
		}
	}
	if strings.TrimSpace(fmt.Sprint(m["status"])) == "vsol_snmp" {
		if n, err := strconv.Atoi(idStr); err == nil && n > 0 {
			return "0" + strconv.Itoa(n)
		}
	}
	return idStr
}

// VsolMibPonCompactID converte o índice de PON no MIB VSOL (1-based) na mesma chave compacta que GPON0/N e IF-MIB.
func VsolMibPonCompactID(ponIndex int) string {
	if ponIndex < 1 {
		return strconv.Itoa(ponIndex)
	}
	return "0" + strconv.Itoa(ponIndex)
}

// PonCompactFromPhy devolve chave estável "01" a partir de "GPON0/1" (slot/porta).
func PonCompactFromPhy(displayName, descr string) string {
	tok := firstToken(displayName)
	if tok == "" {
		tok = firstToken(descr)
	}
	m := rePonPhy.FindStringSubmatch(tok)
	if m != nil {
		return m[1] + m[2]
	}
	m = rePonPhyZTE.FindStringSubmatch(tok)
	if m != nil {
		return m[1] + "/" + m[2] + "/" + m[3]
	}
	m = rePonPhyZTEIf.FindStringSubmatch(tok)
	if m != nil {
		return m[1] + "/" + m[2] + "/" + m[3]
	}
	return ""
}

// PonCompactFromOnuIface ex.: "GPON01ONU2 ..." → "01", onu=2.
func PonCompactFromOnuIface(displayName, descr string) (ponCompact string, onu int, ok bool) {
	s := firstToken(displayName)
	if s == "" {
		s = firstToken(descr)
	}
	m := reOnuIface.FindStringSubmatch(s)
	if m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return m[1], 0, true
		}
		return m[1], n, true
	}
	m = reOnuIfaceZTE.FindStringSubmatch(s)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[4])
	if err != nil {
		return m[1] + "/" + m[2] + "/" + m[3], 0, true
	}
	return m[1] + "/" + m[2] + "/" + m[3], n, true
}

// Saturated32Counters true se contadores parecem 32-bit max (inválidos para tráfego HC).
func Saturated32Counters(in, out int64) bool {
	if in == ifOctets32Max || out == ifOctets32Max {
		return true
	}
	return false
}
