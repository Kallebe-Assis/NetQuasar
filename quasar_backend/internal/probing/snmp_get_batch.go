package probing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const snmpGetBatchSizeDefault = 40

// SNMPValueUsable indica resposta SNMP válida (não NoSuchObject/timeout vazio).
func SNMPValueUsable(val string) bool {
	v := strings.TrimSpace(val)
	if v == "" {
		return false
	}
	low := strings.ToLower(v)
	if strings.Contains(low, "nosuch") || strings.Contains(low, "timeout") ||
		strings.Contains(low, "generr") || strings.Contains(low, "endofmib") {
		return false
	}
	return true
}

// SNMPGetMany executa GET em lotes (uma conexão) — equivalente a vários snmpget -v2c.
func SNMPGetMany(ctx context.Context, host, community, version string, timeout time.Duration, retries int, oids []string, batchSize int) ([]SNMPVar, string) {
	host = strings.TrimSpace(host)
	if host == "" || len(oids) == 0 {
		return nil, "host ou oids vazio"
	}
	if batchSize <= 0 {
		batchSize = snmpGetBatchSizeDefault
	}
	if batchSize > 60 {
		batchSize = 60
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if timeout > 120*time.Second {
		timeout = 120 * time.Second
	}
	comm := strings.TrimSpace(community)
	if comm == "" {
		comm = "public"
	}

	g := &gosnmp.GoSNMP{
		Target:    host,
		Port:      161,
		Community: comm,
		Timeout:   timeout,
		Retries:   retries,
	}
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "1", "v1":
		g.Version = gosnmp.Version1
	default:
		g.Version = gosnmp.Version2c
	}
	if err := g.Connect(); err != nil {
		return nil, fmt.Sprintf("connect: %v", err)
	}
	defer func() {
		if g.Conn != nil {
			_ = g.Conn.Close()
		}
	}()

	var all []SNMPVar
	var notes []string
	for i := 0; i < len(oids); i += batchSize {
		if ctx.Err() != nil {
			notes = append(notes, ctx.Err().Error())
			break
		}
		end := i + batchSize
		if end > len(oids) {
			end = len(oids)
		}
		chunk := oids[i:end]
		vars, errNote := snmpGetBatchOnce(ctx, g, chunk)
		all = append(all, vars...)
		if errNote != "" {
			notes = append(notes, errNote)
		}
	}
	return all, strings.TrimSpace(strings.Join(notes, "; "))
}

func snmpGetBatchOnce(ctx context.Context, g *gosnmp.GoSNMP, batch []string) ([]SNMPVar, string) {
	type resWrap struct {
		r *gosnmp.SnmpPacket
		e error
	}
	ch := make(chan resWrap, 1)
	go func(b []string) {
		r, e := g.Get(b)
		ch <- resWrap{r, e}
	}(batch)
	select {
	case <-ctx.Done():
		return nil, ctx.Err().Error()
	case w := <-ch:
		if w.e != nil {
			return nil, fmt.Sprintf("get:%v", w.e)
		}
		if w.r == nil {
			return nil, "get:resposta_vazia"
		}
		var out []SNMPVar
		for _, v := range w.r.Variables {
			oid := NormalizeSNMPOID(v.Name)
			val := snmpValueToString(v.Value)
			if !SNMPValueUsable(val) {
				continue
			}
			out = append(out, SNMPVar{
				OID:   oid,
				Type:  fmt.Sprintf("%v", v.Type),
				Value: val,
			})
		}
		return out, ""
	}
}

// NormalizeSNMPOID garante OID numérico completo para parsers (1.3.6.1.4.1.37950...).
func NormalizeSNMPOID(name string) string {
	oid := strings.TrimPrefix(strings.TrimSpace(name), ".")
	if strings.HasPrefix(oid, "1.3.6") {
		return oid
	}
	if strings.HasPrefix(oid, "4.1.37950") || strings.HasPrefix(oid, "37950") {
		return "1.3.6.1.4.1." + strings.TrimPrefix(oid, "4.1.")
	}
	return oid
}
