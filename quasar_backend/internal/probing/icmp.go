package probing

import (
	"context"
	"fmt"
	"math"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/go-ping/ping"
)

// ICMPOutcome resultado de tentativa ICMP (pode falhar por permissão SO).
type ICMPOutcome struct {
	OK           bool    `json:"ok"`
	RTTMs        int64   `json:"rtt_ms,omitempty"`
	PacketLoss   float64 `json:"packet_loss"`
	PacketsRecv  int     `json:"packets_recv"`
	PacketsSent  int     `json:"packets_sent"`
	Error        string  `json:"error,omitempty"`
	Privileged   bool    `json:"privileged_mode"`
	Note         string  `json:"note,omitempty"`
}

// ClampICMPPayloadBytes valida payload ICMP (≤0 ⇒ 32 B).
func ClampICMPPayloadBytes(n int) int {
	if n <= 0 {
		return 32
	}
	const maxICMPData = 65507
	if n > maxICMPData {
		return maxICMPData
	}
	return n
}

// ICMPPing envia eco ICMP (1 pacote por ciclo). icmpPayloadBytes: tamanho do pacote ICMP (campo Size do go-ping); ≤0 usa 32.
// Em Linux sem CAP_NET_RAW pode falhar; use TCP fallback na API.
func ICMPPing(ctx context.Context, host string, timeout time.Duration, icmpPayloadBytes int) ICMPOutcome {
	host = strings.TrimSpace(host)
	if host == "" {
		return ICMPOutcome{Error: "host vazio"}
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout > 15*time.Second {
		timeout = 15 * time.Second
	}
	size := ClampICMPPayloadBytes(icmpPayloadBytes)

	pinger, err := ping.NewPinger(host)
	if err != nil {
		return ICMPOutcome{Error: err.Error()}
	}
	// Um único pacote reduz custo por ciclo e evita falso negativo com timeout curto.
	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.Size = size
	// No Windows, go-ping precisa de raw socket (privileged=true) para ICMP funcionar de forma confiável.
	// Em Linux/macOS, mantemos false para evitar exigir CAP_NET_RAW/root desnecessariamente.
	pinger.SetPrivileged(runtime.GOOS == "windows")

	out := ICMPOutcome{Privileged: pinger.Privileged()}
	if pinger.Privileged() {
		out.Note = "modo privilegiado (raw ICMP); em Linux pode exigir capability CAP_NET_RAW ou root"
	} else {
		out.Note = "ICMP não privilegiado (UDP em Linux 3+ ou stack nativo em Windows)"
	}

	done := make(chan ICMPOutcome, 1)
	go func() {
		err := pinger.Run()
		stats := pinger.Statistics()
		o := ICMPOutcome{
			Privileged:  pinger.Privileged(),
			PacketsRecv: stats.PacketsRecv,
			PacketsSent: stats.PacketsSent,
			PacketLoss:  sanitizePacketLoss(stats.PacketLoss),
		}
		if err != nil {
			o.Error = err.Error()
			done <- o
			return
		}
		if stats.PacketsRecv > 0 && stats.AvgRtt > 0 {
			o.OK = true
			o.RTTMs = stats.AvgRtt.Milliseconds()
		} else if stats.PacketsRecv > 0 {
			o.OK = true
			o.RTTMs = 0
		} else {
			o.Error = "sem resposta ICMP"
		}
		done <- o
	}()

	select {
	case <-ctx.Done():
		pinger.Stop()
		return ICMPOutcome{Error: ctx.Err().Error(), Privileged: pinger.Privileged()}
	case r := <-done:
		r.Note = out.Note
		return r
	}
}

func sanitizePacketLoss(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 100
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// TCPProbe mede latência de conexão TCP (alternativa quando ICMP indisponível).
func TCPProbe(ctx context.Context, host, port string, timeout time.Duration) (ok bool, rttMs int64, err error) {
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(strings.TrimSpace(host), port)
	t0 := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	rtt := time.Since(t0).Milliseconds()
	if err != nil {
		return false, rtt, err
	}
	_ = conn.Close()
	return true, rtt, nil
}

// HostReachabilityICMPOnly executa apenas ICMP (sem fallback TCP). Útil para diagnóstico e botão "Ping" na UI.
func HostReachabilityICMPOnly(ctx context.Context, host string, icmpTimeout time.Duration, icmpPayloadBytes int) map[string]any {
	host = strings.TrimSpace(host)
	icmp := ICMPPing(ctx, host, icmpTimeout, icmpPayloadBytes)
	out := map[string]any{"host": host, "icmp": icmp}
	if icmp.OK {
		out["ok"] = true
		out["method"] = "icmp"
		out["latency_ms"] = icmp.RTTMs
	} else {
		out["ok"] = false
		out["method"] = "icmp"
		out["latency_ms"] = icmp.RTTMs
	}
	return out
}

// HostReachability tenta ICMP e, se falhar, TCP na porta indicada.
func HostReachability(ctx context.Context, host, tcpFallbackPort string, icmpTimeout, tcpTimeout time.Duration, icmpPayloadBytes int) map[string]any {
	icmp := ICMPPing(ctx, host, icmpTimeout, icmpPayloadBytes)
	out := map[string]any{
		"host": host,
		"icmp": icmp,
	}
	if icmp.OK {
		out["ok"] = true
		out["method"] = "icmp"
		out["latency_ms"] = icmp.RTTMs
		return out
	}
	ok, ms, err := TCPProbe(ctx, host, tcpFallbackPort, tcpTimeout)
	out["tcp_fallback"] = map[string]any{"ok": ok, "latency_ms": ms, "port": tcpFallbackPort, "error": errString(err)}
	if ok {
		out["ok"] = true
		out["method"] = "tcp_connect"
		out["latency_ms"] = ms
	} else {
		out["ok"] = false
		out["method"] = "none"
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ErrUnreachable combina erros para mensagem única.
func ErrUnreachable(host string, icmpErr, tcpErr error) string {
	return fmt.Sprintf("host %s inalcançável: icmp=%v; tcp=%v", host, icmpErr, tcpErr)
}
