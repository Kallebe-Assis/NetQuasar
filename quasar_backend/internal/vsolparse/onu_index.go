package vsolparse

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

// OnuRef índice MIB VSOL (pon.onu) alinhado ao IF-MIB GPONxxONUyy.
type OnuRef struct {
	Pon int `json:"pon"`
	Onu int `json:"onu"`
}

// OidOnuField monta OID igual ao snmpget: …2.1.6.4.10 (modelo PON 4 ONU 10).
func OidOnuField(field string, pon, onu int) string {
	return fmt.Sprintf("%s.%s.%d.%d", OIDGOnuAuthList, field, pon, onu)
}

// PonMibIndex converte chave compacta IF ("04") → índice MIB (4).
func PonMibIndex(compact string) int {
	s := strings.TrimSpace(compact)
	if s == "" {
		return 0
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// OnuRefsFromIfRows lista (pon, onu) únicos a partir das interfaces ONU no IF-MIB.
func OnuRefsFromIfRows(rows []snmpifparse.IfRow) []OnuRef {
	seen := map[string]struct{}{}
	var out []OnuRef
	for _, r := range rows {
		disp := strings.TrimSpace(r.IfName)
		if disp == "" {
			disp = strings.TrimSpace(r.Descr)
		}
		compact, onu, ok := oltifderive.PonCompactFromOnuIface(disp, r.Descr)
		if !ok || onu < 1 {
			continue
		}
		pon := PonMibIndex(compact)
		if pon < 1 {
			continue
		}
		key := fmt.Sprintf("%d.%d", pon, onu)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, OnuRef{Pon: pon, Onu: onu})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pon != out[j].Pon {
			return out[i].Pon < out[j].Pon
		}
		return out[i].Onu < out[j].Onu
	})
	return out
}
