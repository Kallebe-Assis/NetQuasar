package integrationconsumer

import (
	"net"
	"strings"
)

var ipv4FieldKeys = []string{
	"ipv4",
	"ip",
	"ip_fixo",
	"ip_atribuido",
	"endereco_ip",
	"endereco_ipv4",
	"framed_ip_address",
	"framedipaddress",
	"ip_wan",
	"wan_ip",
	"ip_conexao",
	"ip_atual",
	"ip_autenticacao",
	"ip_cliente",
	"ipv4_fixo",
	"ipv4_atribuido",
}

var ipv4NestedKeys = []string{
	"ultima_conexao",
	"conexao",
	"autenticacao",
	"radius",
	"pppoe",
	"conexao_radius",
	"pacote",
	"servico",
	"cliente_servico",
}

func isValidIPv4(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func pickIPv4FromMap(m map[string]any) string {
	for _, k := range ipv4FieldKeys {
		if s := pickStr(m, k); isValidIPv4(s) {
			return s
		}
	}
	for _, nest := range ipv4NestedKeys {
		sub, ok := m[nest].(map[string]any)
		if !ok {
			continue
		}
		if ip := pickIPv4FromMap(sub); ip != "" {
			return ip
		}
	}
	return ""
}

func collectIPv4FromServices(m map[string]any) []string {
	var raw []any
	for _, key := range []string{"servicos", "services", "planos", "cliente_servico"} {
		if arr, ok := m[key].([]any); ok {
			raw = arr
			break
		}
	}
	var ips []string
	seen := map[string]struct{}{}
	for _, it := range raw {
		sm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ip := pickIPv4FromMap(sm)
		if ip == "" {
			continue
		}
		if _, dup := seen[ip]; dup {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	return ips
}

func extractClientIPv4(m map[string]any) string {
	if ip := pickIPv4FromMap(m); ip != "" {
		return ip
	}
	ips := collectIPv4FromServices(m)
	return strings.Join(ips, ", ")
}
