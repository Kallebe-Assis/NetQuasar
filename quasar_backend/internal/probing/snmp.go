package probing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
)

const snmpMACOctets = 6

// SNMPGetParams parâmetros para GET SNMP v2c.
type SNMPGetParams struct {
	Host      string
	Port      uint16
	Community string
	OIDs      []string
	Version   string // "2c" | "1"
	Timeout   time.Duration
	Retries   int
}

// SNMPVar resposta por OID.
type SNMPVar struct {
	OID   string `json:"oid"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// SNMPGetResult resultado de operação Get.
type SNMPGetResult struct {
	OK    bool      `json:"ok"`
	Vars  []SNMPVar `json:"vars,omitempty"`
	Error string    `json:"error,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// SNMPWalkParams parâmetros para Walk SNMP (v1/v2c).
type SNMPWalkParams struct {
	Host      string
	Port      uint16
	Community string
	RootOID   string
	Version   string // "2c" | "1"
	Timeout   time.Duration
	Retries   int
	MaxRows    int
	OnProgress func(rowCount int)
}

// SNMPWalkStopSentinel indica paragem intencional do walk (ex.: login encontrado).
var SNMPWalkStopSentinel = errors.New("snmp walk: stopped")

const snmpBulkMaxRepetitions = 50

func setupGoSNMP(ctx context.Context, p SNMPWalkParams) (*gosnmp.GoSNMP, time.Duration, error) {
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return nil, 0, fmt.Errorf("host vazio")
	}
	reqTimeout := p.Timeout
	if reqTimeout <= 0 {
		reqTimeout = 30 * time.Second
	}
	// Timeout por pedido SNMP (não o tempo total do walk — isso vem do ctx).
	const maxReqTimeout = 45 * time.Second
	if reqTimeout > maxReqTimeout {
		reqTimeout = maxReqTimeout
	}
	if p.Port == 0 {
		p.Port = 161
	}
	if p.Retries < 0 {
		p.Retries = 0
	}
	if p.Retries > 2 {
		p.Retries = 2
	}
	comm := strings.TrimSpace(p.Community)
	if comm == "" {
		comm = "public"
	}
	g := &gosnmp.GoSNMP{
		Target:         host,
		Port:           p.Port,
		Community:      comm,
		Timeout:        reqTimeout,
		Retries:        p.Retries,
		Context:        ctx,
		MaxRepetitions: snmpBulkMaxRepetitions,
	}
	switch strings.ToLower(strings.TrimSpace(p.Version)) {
	case "1", "v1":
		g.Version = gosnmp.Version1
	default:
		g.Version = gosnmp.Version2c
	}
	if err := g.Connect(); err != nil {
		return nil, 0, fmt.Errorf("connect: %w", err)
	}
	return g, reqTimeout, nil
}

// SNMPGet executa SNMP GET (v1 ou v2c).
func SNMPGet(ctx context.Context, p SNMPGetParams) SNMPGetResult {
	if p.Timeout <= 0 {
		p.Timeout = 5 * time.Second
	}
	if p.Timeout > 30*time.Second {
		p.Timeout = 30 * time.Second
	}
	if p.Retries < 0 {
		p.Retries = 0
	}
	if p.Retries > 3 {
		p.Retries = 3
	}
	if p.Port == 0 {
		p.Port = 161
	}
	host := strings.TrimSpace(p.Host)
	if host == "" || len(p.OIDs) == 0 {
		return SNMPGetResult{OK: false, Error: "host e oids obrigatórios"}
	}
	comm := p.Community
	if comm == "" {
		comm = "public"
	}

	g := &gosnmp.GoSNMP{
		Target:    host,
		Port:      p.Port,
		Community: comm,
		Timeout:   p.Timeout,
		Retries:   p.Retries,
	}
	switch strings.ToLower(strings.TrimSpace(p.Version)) {
	case "1", "v1":
		g.Version = gosnmp.Version1
	default:
		g.Version = gosnmp.Version2c
	}

	err := g.Connect()
	if err != nil {
		return SNMPGetResult{OK: false, Error: fmt.Sprintf("connect: %v", err)}
	}
	defer func() {
		if g.Conn != nil {
			_ = g.Conn.Close()
		}
	}()

	type resWrap struct {
		r *gosnmp.SnmpPacket
		e error
	}
	ch := make(chan resWrap, 1)
	go func() {
		r, e := g.Get(p.OIDs)
		ch <- resWrap{r, e}
	}()

	select {
	case <-ctx.Done():
		return SNMPGetResult{OK: false, Error: ctx.Err().Error()}
	case w := <-ch:
		if w.e != nil {
			return SNMPGetResult{OK: false, Error: w.e.Error()}
		}
		if w.r == nil {
			return SNMPGetResult{OK: false, Error: "resposta vazia"}
		}
		var vars []SNMPVar
		for _, v := range w.r.Variables {
			vars = append(vars, SNMPVar{
				OID:   NormalizeSNMPOID(v.Name),
				Type:  fmt.Sprintf("%v", v.Type),
				Value: snmpValueToString(v.Value),
			})
		}
		return SNMPGetResult{OK: true, Vars: vars, Note: "SNMP v2c/v1 síncrono; v3 em evolução"}
	}
}

var errSNMPWalkMaxRows = errors.New("snmp walk: limite de linhas")

const snmpWalkValueMaxLen = 512

// snmpValueToString converte valores PDU (OctetString em []byte, etc.) para texto legível.
func snmpValueToString(v any) string {
	switch x := v.(type) {
	case []byte:
		return truncateSNMPString(octetStringToUTF8(x))
	case string:
		return truncateSNMPString(strings.TrimSpace(x))
	default:
		return truncateSNMPString(strings.TrimSpace(fmt.Sprint(x)))
	}
}

func octetStringToUTF8(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return ""
	}
	if dt, ok := formatSNMPDateAndTime(b); ok {
		return dt
	}
	if ip, ok := bytesAsIPv4(b); ok {
		return ip
	}
	if ip, ok := bytesAsIPv6(b); ok {
		return ip
	}
	// Nomes de interface (ex.: «combo1», «ether1») podem ter exactamente 6 octetos ASCII —
	// tratar como texto antes de ifPhysAddress (também 6 octetos binários).
	if isPrintableASCII(b) {
		return string(b)
	}
	// ifPhysAddress e similares: 6 octetos (por vezes com 0x00 à frente).
	if mac, ok := bytesAsMAC(b); ok {
		return mac
	}
	s := string(b)
	if utf8.ValidString(s) && isMostlyPrintableUTF8(s) {
		return s
	}
	return formatOctetsHex(b)
}

func trimTrailingNulls(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return b
}

func bytesAsMAC(b []byte) (string, bool) {
	b = trimTrailingNulls(append([]byte(nil), b...))
	if len(b) == snmpMACOctets+1 && b[0] == 0 {
		b = b[1:]
	}
	if len(b) != snmpMACOctets {
		return "", false
	}
	parts := make([]string, snmpMACOctets)
	for i := 0; i < snmpMACOctets; i++ {
		parts[i] = fmt.Sprintf("%02x", b[i])
	}
	return strings.Join(parts, ":"), true
}

func bytesAsIPv4(b []byte) (string, bool) {
	b = trimTrailingNulls(append([]byte(nil), b...))
	if len(b) != 4 {
		return "", false
	}
	return net.IP(b).String(), true
}

func bytesAsIPv6(b []byte) (string, bool) {
	b = trimTrailingNulls(append([]byte(nil), b...))
	if len(b) != 16 {
		return "", false
	}
	ip := net.IP(b)
	if ip.IsUnspecified() {
		return "", false
	}
	return ip.String(), true
}

func isPrintableASCII(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c >= 32 && c < 127 {
			continue
		}
		if c == 9 || c == 10 || c == 13 {
			continue
		}
		return false
	}
	return true
}

func isMostlyPrintableUTF8(s string) bool {
	if s == "" {
		return false
	}
	printable := 0
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 32 && r < 127) {
			printable++
		}
	}
	return printable*2 >= len([]rune(s))
}

func formatOctetsHex(b []byte) string {
	if len(b) <= 32 {
		parts := make([]string, len(b))
		for i, c := range b {
			parts[i] = fmt.Sprintf("%02x", c)
		}
		return strings.Join(parts, ":")
	}
	var sb strings.Builder
	for i, c := range b {
		if i > 0 {
			if i%16 == 0 {
				sb.WriteString("\n")
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(fmt.Sprintf("%02x", c))
	}
	return sb.String()
}

// NormalizeIFLabel corrige rótulos já gravados como hex ASCII (ex.: 63:6f:6d:62:6f:31 → combo1).
func NormalizeIFLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if decoded, ok := TryDecodeColonHexASCII(s); ok {
		return decoded
	}
	return s
}

// TryDecodeColonHexASCII converte «63:6f:6d:62:6f:31» em texto quando todos os octetos são ASCII imprimível.
// Não altera MACs binários já formatados (ex.: contêm octetos > 0x7e).
func TryDecodeColonHexASCII(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":") {
		return "", false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 64 {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		if len(p) != 2 {
			return "", false
		}
		n, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return "", false
		}
		if n < 32 || n > 126 {
			return "", false
		}
		b.WriteByte(byte(n))
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", false
	}
	return out, true
}

func truncateSNMPString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > snmpWalkValueMaxLen {
		return s[:snmpWalkValueMaxLen] + "…"
	}
	return s
}

// SNMPWalkLimited faz Walk SNMP a partir de rootOID (ex.: 1.3.6.1.2.1 MIB-II) e pára em maxRows.
func SNMPWalkLimited(ctx context.Context, host, community, rootOID string, maxRows int, timeout time.Duration) ([]SNMPVar, bool, string) {
	return SNMPWalk(ctx, SNMPWalkParams{
		Host:      host,
		Port:      161,
		Community: community,
		RootOID:   rootOID,
		Version:   "2c",
		Timeout:   timeout,
		Retries:   0,
		MaxRows:   maxRows,
	})
}

// SNMPWalk faz Walk SNMP (v2c: GET-BULK em lotes; v1: GET-NEXT).
func SNMPWalk(ctx context.Context, p SNMPWalkParams) ([]SNMPVar, bool, string) {
	rootOID := strings.TrimSpace(p.RootOID)
	if rootOID == "" {
		return nil, false, "root_oid vazio"
	}
	if p.MaxRows <= 0 {
		p.MaxRows = 4000
	}
	const snmpWalkMaxRowsCap = 200000
	if p.MaxRows > snmpWalkMaxRowsCap {
		p.MaxRows = snmpWalkMaxRowsCap
	}

	g, _, err := setupGoSNMP(ctx, p)
	if err != nil {
		return nil, false, err.Error()
	}
	defer func() {
		if g.Conn != nil {
			_ = g.Conn.Close()
		}
	}()

	useBulk := g.Version == gosnmp.Version2c
	var out []SNMPVar
	truncated := false
	walkFn := func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(out) >= p.MaxRows {
			truncated = true
			return errSNMPWalkMaxRows
		}
		val := snmpValueToString(pdu.Value)
		out = append(out, SNMPVar{
			OID:   NormalizeSNMPOID(pdu.Name),
			Type:  fmt.Sprintf("%v", pdu.Type),
			Value: val,
		})
		if p.OnProgress != nil {
			p.OnProgress(len(out))
		}
		return nil
	}

	var walkErr error
	if useBulk {
		walkErr = g.BulkWalk(rootOID, walkFn)
	} else {
		walkErr = g.Walk(rootOID, walkFn)
	}

	switch {
	case walkErr == nil:
		return out, truncated, ""
	case errors.Is(walkErr, errSNMPWalkMaxRows):
		return out, true, ""
	case ctx.Err() != nil && errors.Is(walkErr, ctx.Err()):
		if len(out) > 0 {
			return out, true, ""
		}
		return out, true, ctx.Err().Error()
	default:
		// Mantém dados parciais se o walk já recolheu linhas (ex.: timeout a meio).
		if len(out) > 0 {
			return out, truncated, ""
		}
		return out, truncated, walkErr.Error()
	}
}

// SNMPWalkUntil executa walk até fn(v) retornar true (match encontrado).
func SNMPWalkUntil(ctx context.Context, p SNMPWalkParams, fn func(v SNMPVar) bool) (SNMPVar, bool, string) {
	rootOID := strings.TrimSpace(p.RootOID)
	if rootOID == "" {
		return SNMPVar{}, false, "root_oid vazio"
	}
	if p.MaxRows <= 0 {
		p.MaxRows = 50000
	}

	g, _, err := setupGoSNMP(ctx, p)
	if err != nil {
		return SNMPVar{}, false, err.Error()
	}
	defer func() {
		if g.Conn != nil {
			_ = g.Conn.Close()
		}
	}()

	useBulk := g.Version == gosnmp.Version2c
	var matched SNMPVar
	found := false
	walkFn := func(pdu gosnmp.SnmpPDU) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if found {
			return SNMPWalkStopSentinel
		}
		v := SNMPVar{
			OID:   NormalizeSNMPOID(pdu.Name),
			Type:  fmt.Sprintf("%v", pdu.Type),
			Value: snmpValueToString(pdu.Value),
		}
		if fn != nil && fn(v) {
			matched = v
			found = true
			return SNMPWalkStopSentinel
		}
		return nil
	}

	var walkErr error
	if useBulk {
		walkErr = g.BulkWalk(rootOID, walkFn)
	} else {
		walkErr = g.Walk(rootOID, walkFn)
	}

	switch {
	case found:
		return matched, true, ""
	case walkErr == nil, errors.Is(walkErr, SNMPWalkStopSentinel):
		return SNMPVar{}, false, ""
	case ctx.Err() != nil && errors.Is(walkErr, ctx.Err()):
		return SNMPVar{}, false, ctx.Err().Error()
	default:
		return SNMPVar{}, false, walkErr.Error()
	}
}
