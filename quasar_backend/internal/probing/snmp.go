package probing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
)

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
	MaxRows   int
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
				OID:   v.Name,
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
	s := string(b)
	if utf8.ValidString(s) {
		return s
	}
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c >= 32 && c < 127:
			sb.WriteByte(c)
		case c == 9 || c == 10 || c == 13:
			sb.WriteByte(' ')
		default:
			sb.WriteByte('.')
		}
	}
	return sb.String()
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

// SNMPWalk faz Walk SNMP com parâmetros equivalentes ao GET.
func SNMPWalk(ctx context.Context, p SNMPWalkParams) ([]SNMPVar, bool, string) {
	host := strings.TrimSpace(p.Host)
	rootOID := strings.TrimSpace(p.RootOID)
	if host == "" || rootOID == "" {
		return nil, false, "host ou root_oid vazio"
	}
	if p.Port == 0 {
		p.Port = 161
	}
	if p.MaxRows <= 0 {
		p.MaxRows = 4000
	}
	if p.MaxRows > 60000 {
		p.MaxRows = 60000
	}
	if p.Timeout <= 0 {
		p.Timeout = 60 * time.Second
	}
	if p.Timeout > 120*time.Second {
		p.Timeout = 120 * time.Second
	}
	if p.Retries < 0 {
		p.Retries = 0
	}
	if p.Retries > 3 {
		p.Retries = 3
	}
	comm := strings.TrimSpace(p.Community)
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
	if err := g.Connect(); err != nil {
		return nil, false, fmt.Sprintf("connect: %v", err)
	}
	defer func() {
		if g.Conn != nil {
			_ = g.Conn.Close()
		}
	}()

	var out []SNMPVar
	truncated := false
	walkErr := g.Walk(rootOID, func(pdu gosnmp.SnmpPDU) error {
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
			OID:   pdu.Name,
			Type:  fmt.Sprintf("%v", pdu.Type),
			Value: val,
		})
		return nil
	})

	switch {
	case walkErr == nil:
		return out, truncated, ""
	case errors.Is(walkErr, errSNMPWalkMaxRows):
		return out, true, ""
	case ctx.Err() != nil && errors.Is(walkErr, ctx.Err()):
		return out, true, ctx.Err().Error()
	default:
		return out, truncated, walkErr.Error()
	}
}
