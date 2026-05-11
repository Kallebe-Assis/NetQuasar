package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// IsSupabaseDirectDBHost identifica o host de ligação direta (porta 5432 típica), não o pooler.
func IsSupabaseDirectDBHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return strings.HasPrefix(h, "db.") && strings.HasSuffix(h, ".supabase.co")
}

func looksLikeTCPDialFailure(msg string) bool {
	s := strings.ToLower(msg)
	return strings.Contains(s, "dial tcp") || strings.Contains(s, "dial tcp6")
}

func looksLikeIPv6EgressBlocked(msg string) bool {
	s := strings.ToLower(msg)
	if strings.Contains(s, "network is unreachable") {
		return true
	}
	if strings.Contains(s, "no route to host") {
		return true
	}
	if strings.Contains(s, "i/o timeout") || strings.Contains(s, "connection timed out") {
		// Endereços IPv6 aparecem entre '[' e ']:' nas mensagens do net; IPv4 não usa '['.
		return looksLikeTCPDialFailure(msg) && strings.Contains(msg, "[") && strings.Contains(msg, "]:")
	}
	return false
}

func supabaseDirectHostIPKinds(ctx context.Context, host string) (hasV4, hasV6 bool) {
	var r net.Resolver
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if ips, err := r.LookupIP(ctx, "ip4", host); err == nil {
		for _, ip := range ips {
			if ip.To4() != nil {
				hasV4 = true
				break
			}
		}
	}
	if ips, err := r.LookupIP(ctx, "ip6", host); err == nil && len(ips) > 0 {
		hasV6 = true
	}
	return hasV4, hasV6
}

// SupabaseDirectDBDockerHint devolve texto curto quando o host db.*.supabase.co só tem AAAA
// e a falha parece ser de rede IPv6 (comum em Docker sem saída IPv6).
func SupabaseDirectDBDockerHint(ctx context.Context, dsn string, connectErr string) string {
	if connectErr == "" {
		return ""
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if !IsSupabaseDirectDBHost(host) {
		return ""
	}
	if !looksLikeTCPDialFailure(connectErr) || !looksLikeIPv6EgressBlocked(connectErr) {
		return ""
	}
	hasV4, hasV6 := supabaseDirectHostIPKinds(ctx, host)
	if hasV4 || !hasV6 {
		return ""
	}
	return `O hostname db.….supabase.co (portas 5432 ou 6543) resolve em geral só para IPv6. Use o Session pooler do painel (Connect → Session): host aws-0- ou aws-1-REGIÃO.pooler.supabase.com (a numeração varia por projeto), porta 5432, utilizador postgres.PROJECTREF. Ver: https://supabase.com/docs/guides/database/connecting-to-postgres`
}

// ErrDetailsWithSupabaseHint devolve um mapa com campo hint quando aplicável (para JSON em writeErr).
func ErrDetailsWithSupabaseHint(ctx context.Context, dsn string, connectErr string) map[string]string {
	h := SupabaseDirectDBDockerHint(ctx, dsn, connectErr)
	if h == "" {
		return nil
	}
	return map[string]string{"hint": h}
}

// FormatConnectErrWithSupabaseHint acrescenta o hint ao texto do erro (ex.: stderr do dbping).
func FormatConnectErrWithSupabaseHint(ctx context.Context, dsn string, connectErr error) string {
	if connectErr == nil {
		return ""
	}
	msg := connectErr.Error()
	h := SupabaseDirectDBDockerHint(ctx, dsn, msg)
	if h == "" {
		return msg
	}
	return fmt.Sprintf("%s\n\n%s", msg, h)
}
