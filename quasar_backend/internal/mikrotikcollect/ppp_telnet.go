package mikrotikcollect

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// PPPActiveEntry uma linha de "/ppp active print detail" — sessão PPPoE ligada agora.
type PPPActiveEntry struct {
	Name      string
	Service   string
	CallerID  string
	Address   string
	Uptime    string
	UptimeSec int64
	Encoding  string
	SessionID string
	Radius    bool
	Comment   string
}

// PPPSecretEntry uma linha de "/ppp secret print detail" — login configurado, online ou não.
// É a única fonte do comentário/rótulo do cliente (não existe por SNMP).
type PPPSecretEntry struct {
	Name          string
	Service       string
	CallerID      string
	Profile       string
	LocalAddress  string
	RemoteAddress string
	Comment       string
	Disabled      bool
}

var routerosUptimePart = regexp.MustCompile(`(\d+)(w|d|h|m|s)`)

// ParseRouterOSUptime converte o formato de duração do RouterOS (ex.: "5h3m22s", "3d5h2m",
// "1w2d", "9m52s") em segundos. Devolve ok=false se não reconhecer nada.
func ParseRouterOSUptime(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	matches := routerosUptimePart.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return 0, false
	}
	var total int64
	for _, m := range matches {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		switch m[2] {
		case "w":
			total += n * 7 * 24 * 3600
		case "d":
			total += n * 24 * 3600
		case "h":
			total += n * 3600
		case "m":
			total += n * 60
		case "s":
			total += n
		}
	}
	return total, true
}

func normalizeCallerID(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func truthyFlag(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return v == "yes" || v == "true" || v == "1"
}

// ParsePPPActiveDetail extrai as sessões de "/ppp active print detail without-paging".
func ParsePPPActiveDetail(output string) []PPPActiveEntry {
	recs := ParseRouterOSRecords(output)
	out := make([]PPPActiveEntry, 0, len(recs))
	for _, r := range recs {
		name := strings.TrimSpace(r["name"])
		if name == "" {
			continue
		}
		e := PPPActiveEntry{
			Name:      name,
			Service:   strings.TrimSpace(r["service"]),
			CallerID:  normalizeCallerID(r["caller-id"]),
			Address:   strings.TrimSpace(r["address"]),
			Uptime:    strings.TrimSpace(r["uptime"]),
			Encoding:  strings.TrimSpace(r["encoding"]),
			SessionID: strings.TrimSpace(r["session-id"]),
			// A flag "R" (radius) vem solta antes do primeiro key=value em modo detail (ver
			// legenda "Flags: R - radius" no topo da saída), não como "radius=yes" — capturada
			// por ParseRouterOSRecords em "flags".
			Radius:  strings.Contains(r["flags"], "R") || truthyFlag(r["radius"]),
			Comment: strings.TrimSpace(r["comment"]),
		}
		if sec, ok := ParseRouterOSUptime(e.Uptime); ok {
			e.UptimeSec = sec
		}
		out = append(out, e)
	}
	return out
}

// ParsePPPSecretDetail extrai o roster de "/ppp secret print detail without-paging" — todo
// login configurado, ligado ou não. É daqui que vem o comentário/rótulo do cliente.
func ParsePPPSecretDetail(output string) []PPPSecretEntry {
	recs := ParseRouterOSRecords(output)
	out := make([]PPPSecretEntry, 0, len(recs))
	for _, r := range recs {
		name := strings.TrimSpace(r["name"])
		if name == "" {
			continue
		}
		out = append(out, PPPSecretEntry{
			Name:          name,
			Service:       strings.TrimSpace(r["service"]),
			CallerID:      normalizeCallerID(r["caller-id"]),
			Profile:       strings.TrimSpace(r["profile"]),
			LocalAddress:  strings.TrimSpace(r["local-address"]),
			RemoteAddress: strings.TrimSpace(r["remote-address"]),
			Comment:       strings.TrimSpace(r["comment"]),
			// "X" (disabled) também pode vir solta em "flags" em vez de "disabled=yes" — ver
			// nota equivalente em ParsePPPActiveDetail.
			Disabled: strings.Contains(r["flags"], "X") || truthyFlag(r["disabled"]),
		})
	}
	return out
}

// CollectPPPoESessions liga por telnet e corre "/ppp active print detail" (sessões online agora)
// e "/ppp secret print detail" (roster completo, com comentário/rótulo do cliente — dado que só
// existe via CLI, não em SNMP nenhum). O roster é melhor-esforço: se falhar (ex.: sem permissão
// para /ppp secret), ainda devolve as sessões activas — só o roster/comentários fica incompleto.
func CollectPPPoESessions(ctx context.Context, host string, creds TelnetCredentials, timeout time.Duration) ([]PPPActiveEntry, []PPPSecretEntry, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, nil, errors.New("host em falta")
	}
	if strings.TrimSpace(creds.User) == "" || strings.TrimSpace(creds.Password) == "" {
		return nil, nil, errors.New("credenciais telnet não configuradas (Definições → Rede e SNMP, ou no próprio equipamento)")
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	const activeCmd = "/ppp active print detail without-paging"
	activeRes := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: creds.Port, Timeout: timeout,
		User: creds.User, Password: creds.Password, Enable: creds.Enable,
		Commands:     []string{activeCmd},
		MaxReadBytes: 4 * 1024 * 1024,
	})
	if !activeRes.OK {
		msg := strings.TrimSpace(activeRes.Error)
		if msg == "" {
			msg = "telnet falhou"
		}
		return nil, nil, errors.New(msg)
	}
	active := ParsePPPActiveDetail(stripTelnetScriptEcho(activeRes.Output, activeCmd))

	const secretCmd = "/ppp secret print detail without-paging"
	secretsRes := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: creds.Port, Timeout: timeout,
		User: creds.User, Password: creds.Password, Enable: creds.Enable,
		Commands:     []string{secretCmd},
		MaxReadBytes: 4 * 1024 * 1024,
	})
	var secrets []PPPSecretEntry
	if secretsRes.OK {
		secrets = ParsePPPSecretDetail(stripTelnetScriptEcho(secretsRes.Output, secretCmd))
	}
	return active, secrets, nil
}
