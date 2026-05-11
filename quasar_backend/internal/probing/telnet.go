package probing

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// TelnetTestParams teste não interativo: abre TCP, lê banner opcional, envia credenciais em texto claro (somente diagnóstico).
type TelnetTestParams struct {
	Host     string
	Port     string
	Timeout  time.Duration
	User     string
	Password string
	// MaxReadBytes limita leitura após envio (evita bloqueio).
	MaxReadBytes int
}

// TelnetTestResult resultado do teste.
type TelnetTestResult struct {
	OK          bool   `json:"ok"`
	Banner      string `json:"banner,omitempty"`
	AfterLogin  string `json:"after_login_snippet,omitempty"`
	LatencyMs   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
	Note        string `json:"note"`
}

type TelnetRunParams struct {
	Host      string
	Port      string
	Timeout   time.Duration
	User      string
	Password  string
	Enable    string
	Command   string
	PreCommands []string
	MaxReadBytes int
}

type TelnetRunResult struct {
	OK        bool   `json:"ok"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
	Note      string `json:"note"`
}

// TelnetProbe abre sessão TCP (Telnet RFC mínimo: sem opção IAC completa; adequado para equipamentos que ecoam texto).
func TelnetProbe(ctx context.Context, p TelnetTestParams) TelnetTestResult {
	note := "Telnet é texto claro — use apenas em redes de gestão confiáveis; não é shell interativo completo."
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return TelnetTestResult{OK: false, Note: note, Error: "host obrigatório"}
	}
	port := strings.TrimSpace(p.Port)
	if port == "" {
		port = "23"
	}
	if p.Timeout <= 0 {
		p.Timeout = 8 * time.Second
	}
	if p.Timeout > 30*time.Second {
		p.Timeout = 30 * time.Second
	}
	maxRead := p.MaxReadBytes
	if maxRead <= 0 {
		maxRead = 2048
	}
	if maxRead > 16384 {
		maxRead = 16384
	}

	addr := net.JoinHostPort(host, port)
	t0 := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	lat := time.Since(t0).Milliseconds()
	if err != nil {
		return TelnetTestResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.Timeout))

	buf := make([]byte, 512)
	n, _ := conn.Read(buf)
	banner := sanitizeASCII(string(buf[:n]))

	if p.User != "" {
		_, err = fmt.Fprintf(conn, "%s\r\n", p.User)
		if err != nil {
			return TelnetTestResult{OK: false, Banner: banner, LatencyMs: lat, Error: err.Error(), Note: note}
		}
	}
	if p.Password != "" {
		_, err = fmt.Fprintf(conn, "%s\r\n", p.Password)
		if err != nil {
			return TelnetTestResult{OK: false, Banner: banner, LatencyMs: lat, Error: err.Error(), Note: note}
		}
	}

	after := make([]byte, maxRead)
	n2, _ := conn.Read(after)
	snippet := sanitizeASCII(string(after[:n2]))

	return TelnetTestResult{
		OK:         true,
		Banner:     trim(banner, 400),
		AfterLogin: trim(snippet, 400),
		LatencyMs:  lat,
		Note:       note,
	}
}

func trim(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func sanitizeASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(' ')
		}
	}
	return strings.TrimSpace(b.String())
}

func sanitizeASCIIKeepLines(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func TelnetRunCommand(ctx context.Context, p TelnetRunParams) TelnetRunResult {
	note := "Telnet é texto claro — use apenas em redes de gestão confiáveis; saída pode variar por firmware."
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return TelnetRunResult{OK: false, Note: note, Error: "host obrigatório"}
	}
	port := strings.TrimSpace(p.Port)
	if port == "" {
		port = "23"
	}
	if p.Timeout <= 0 {
		p.Timeout = 12 * time.Second
	}
	if p.Timeout > 45*time.Second {
		p.Timeout = 45 * time.Second
	}
	maxRead := p.MaxReadBytes
	if maxRead <= 0 {
		maxRead = 32768
	}
	if maxRead > 524288 {
		maxRead = 524288
	}
	cmd := strings.TrimSpace(p.Command)
	if cmd == "" {
		return TelnetRunResult{OK: false, Note: note, Error: "command obrigatório"}
	}

	addr := net.JoinHostPort(host, port)
	t0 := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	lat := time.Since(t0).Milliseconds()
	if err != nil {
		return TelnetRunResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.Timeout))

	readChunk := func(wait time.Duration) string {
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		tmp := make([]byte, 4096)
		n, err := conn.Read(tmp)
		if err != nil || n <= 0 {
			return ""
		}
		return sanitizeASCIIKeepLines(string(tmp[:n]))
	}
	readDrain := func(total time.Duration, maxBytes int) string {
		deadline := time.Now().Add(total)
		var out strings.Builder
		read := 0
		for time.Now().Before(deadline) && read < maxBytes {
			part := readChunk(350 * time.Millisecond)
			if strings.TrimSpace(part) == "" {
				time.Sleep(80 * time.Millisecond)
				continue
			}
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(part)
			read += len(part)
			low := strings.ToLower(part)
			if strings.Contains(low, "#") || strings.Contains(low, ">") {
				break
			}
		}
		return out.String()
	}

	_ = readDrain(900*time.Millisecond, 8192)

	writeLn := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		_, e := fmt.Fprintf(conn, "%s\r\n", s)
		if e != nil {
			return e
		}
		_ = readDrain(700*time.Millisecond, 8192)
		return nil
	}

	if err := writeLn(p.User); err != nil {
		return TelnetRunResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	if err := writeLn(p.Password); err != nil {
		return TelnetRunResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	if strings.TrimSpace(p.Enable) != "" {
		_ = writeLn("enable")
		_ = writeLn(p.Enable)
	}
	for _, pc := range p.PreCommands {
		_ = writeLn(pc)
	}
	if _, err := fmt.Fprintf(conn, "%s\r\n", cmd); err != nil {
		return TelnetRunResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	out := readDrain(8*time.Second, maxRead)
	_, _ = fmt.Fprintf(conn, "exit\r\n")
	return TelnetRunResult{
		OK:        true,
		Output:    trim(out, 120000),
		LatencyMs: lat,
		Note:      note,
	}
}
