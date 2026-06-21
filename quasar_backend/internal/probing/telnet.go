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
	Host         string
	Port         string
	Timeout      time.Duration
	User         string
	Password     string
	Enable       string
	Command      string
	PreCommands  []string
	MaxReadBytes int
}

type TelnetRunScriptParams struct {
	Host           string
	Port           string
	Timeout        time.Duration
	User           string
	Password       string
	Enable         string
	PreCommands    []string
	RawPreCommands []string
	Commands       []string
	MaxReadBytes   int
}

type TelnetScriptStepResult struct {
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

type TelnetRunScriptResult struct {
	OK        bool                     `json:"ok"`
	Steps     []TelnetScriptStepResult `json:"steps,omitempty"`
	Output    string                   `json:"output,omitempty"`
	Error     string                   `json:"error,omitempty"`
	LatencyMs int64                    `json:"latency_ms"`
	Note      string                   `json:"note"`
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
	s = stripTelnetControlSequences(s)
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

func stripTelnetControlSequences(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		if s[i] == '[' && i+2 < len(s) {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			if j < len(s) && j > i+1 && ((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				i = j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func promptWantsPassword(out string) bool {
	low := strings.ToLower(out)
	return strings.Contains(low, "password:") || strings.Contains(low, "password :")
}

func isEnablePasswordPlaceholder(line string) bool {
	t := strings.TrimSpace(line)
	return t == "{enable}" || t == "{enable_password}" || t == "{telnet_enable}"
}

func (s *telnetSession) sendPassword(pass string) (string, error) {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return "", nil
	}
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", pass); err != nil {
		return "", err
	}
	return s.readUntilReady(2200 * time.Millisecond), nil
}

func (s *telnetSession) sendLineNoPasswordAuto(line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", line); err != nil {
		return "", err
	}
	return s.readUntilReady(2200 * time.Millisecond), nil
}

func isEnableCommand(line string) bool {
	low := strings.ToLower(strings.TrimSpace(line))
	return low == "enable" || low == "en" || strings.HasPrefix(low, "enable ")
}

func runPreCommands(sess *telnetSession, rawPre, renderedPre []string) error {
	for i := 0; i < len(renderedPre); i++ {
		rendered := strings.TrimSpace(renderedPre[i])
		if rendered == "" {
			continue
		}
		raw := ""
		if i < len(rawPre) {
			raw = strings.TrimSpace(rawPre[i])
		}
		if isEnablePasswordPlaceholder(raw) {
			if _, err := sess.sendPassword(rendered); err != nil {
				return err
			}
			continue
		}
		if isEnableCommand(raw) && i+1 < len(rawPre) && isEnablePasswordPlaceholder(rawPre[i+1]) {
			if _, err := sess.sendLineNoPasswordAuto(rendered); err != nil {
				return err
			}
			continue
		}
		if _, err := sess.sendLine(rendered); err != nil {
			return err
		}
	}
	return nil
}

func preCommandsIncludeEnable(pre []string) bool {
	for _, pc := range pre {
		if isEnableCommand(pc) {
			return true
		}
	}
	return false
}

type telnetSession struct {
	conn    net.Conn
	timeout time.Duration
	maxRead int
	user    string
	pass    string
	enable  string
}

func (s *telnetSession) readChunk(wait time.Duration) string {
	_ = s.conn.SetReadDeadline(time.Now().Add(wait))
	tmp := make([]byte, 4096)
	n, err := s.conn.Read(tmp)
	if err != nil || n <= 0 {
		return ""
	}
	return sanitizeASCIIKeepLines(string(tmp[:n]))
}

func (s *telnetSession) readUntilReady(total time.Duration) string {
	deadline := time.Now().Add(total)
	var out strings.Builder
	read := 0
	for time.Now().Before(deadline) && read < s.maxRead {
		part := s.readChunk(400 * time.Millisecond)
		if strings.TrimSpace(part) == "" {
			time.Sleep(80 * time.Millisecond)
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(part)
		read += len(part)
		chunk := strings.ToLower(part)
		if promptWantsPassword(part) {
			break
		}
		if strings.Contains(chunk, "#") || strings.Contains(chunk, ">") || strings.Contains(chunk, "(config)") {
			break
		}
	}
	return out.String()
}

func (s *telnetSession) sendLine(line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", line); err != nil {
		return "", err
	}
	out := s.readUntilReady(2200 * time.Millisecond)
	if promptWantsPassword(out) {
		pass := s.pass
		if isEnableCommand(line) && strings.TrimSpace(s.enable) != "" {
			pass = s.enable
		}
		if strings.TrimSpace(pass) != "" {
			if _, err := fmt.Fprintf(s.conn, "%s\r\n", pass); err != nil {
				return out, err
			}
			extra := s.readUntilReady(2200 * time.Millisecond)
			if extra != "" {
				if out != "" {
					out += "\n"
				}
				out += extra
			}
		}
	}
	return out, nil
}

func (s *telnetSession) login() error {
	_ = s.readUntilReady(900 * time.Millisecond)
	if _, err := s.sendLine(s.user); err != nil {
		return err
	}
	if _, err := s.sendLine(s.pass); err != nil {
		return err
	}
	return nil
}

func TelnetRunScript(ctx context.Context, p TelnetRunScriptParams) TelnetRunScriptResult {
	note := "Telnet é texto claro — use apenas em redes de gestão confiáveis; saída pode variar por firmware."
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return TelnetRunScriptResult{OK: false, Note: note, Error: "host obrigatório"}
	}
	port := strings.TrimSpace(p.Port)
	if port == "" {
		port = "23"
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	if timeout > 120*time.Second {
		timeout = 120 * time.Second
	}
	maxRead := p.MaxReadBytes
	if maxRead <= 0 {
		maxRead = 32768
	}
	if maxRead > 524288 {
		maxRead = 524288
	}
	if len(p.Commands) == 0 {
		return TelnetRunScriptResult{OK: false, Note: note, Error: "nenhum comando"}
	}

	addr := net.JoinHostPort(host, port)
	t0 := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	lat := time.Since(t0).Milliseconds()
	if err != nil {
		return TelnetRunScriptResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	sess := &telnetSession{
		conn: conn, timeout: timeout, maxRead: maxRead,
		user: strings.TrimSpace(p.User),
		pass: strings.TrimSpace(p.Password),
		enable: strings.TrimSpace(p.Enable),
	}
	if err := sess.login(); err != nil {
		return TelnetRunScriptResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}

	rawPre := p.RawPreCommands
	if len(rawPre) == 0 {
		rawPre = p.PreCommands
	}
	if strings.TrimSpace(sess.enable) != "" && !preCommandsIncludeEnable(rawPre) {
		if _, err := sess.sendLine("enable"); err != nil {
			return TelnetRunScriptResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
		}
	}
	if err := runPreCommands(sess, rawPre, p.PreCommands); err != nil {
		return TelnetRunScriptResult{OK: false, LatencyMs: lat, Error: err.Error(), Note: note}
	}

	var steps []TelnetScriptStepResult
	var combined strings.Builder
	allOK := true
	for _, cmd := range p.Commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if _, err := fmt.Fprintf(conn, "%s\r\n", cmd); err != nil {
			steps = append(steps, TelnetScriptStepResult{Command: cmd, OK: false, Error: err.Error()})
			allOK = false
			break
		}
		out := sess.readUntilReady(8 * time.Second)
		step := TelnetScriptStepResult{Command: cmd, OK: true, Output: trim(out, 120000)}
		steps = append(steps, step)
		if combined.Len() > 0 {
			combined.WriteString("\n\n---\n\n")
		}
		combined.WriteString("$ ")
		combined.WriteString(cmd)
		combined.WriteString("\n")
		combined.WriteString(step.Output)
	}
	_, _ = fmt.Fprintf(conn, "exit\r\n")

	return TelnetRunScriptResult{
		OK:        allOK,
		Steps:     steps,
		Output:    combined.String(),
		LatencyMs: lat,
		Note:      note,
	}
}

func TelnetRunCommand(ctx context.Context, p TelnetRunParams) TelnetRunResult {
	cmd := strings.TrimSpace(p.Command)
	if cmd == "" {
		return TelnetRunResult{OK: false, Error: "command obrigatório", Note: "Telnet é texto claro."}
	}
	script := TelnetRunScript(ctx, TelnetRunScriptParams{
		Host: p.Host, Port: p.Port, Timeout: p.Timeout,
		User: p.User, Password: p.Password, Enable: p.Enable,
		PreCommands: p.PreCommands, Commands: []string{cmd},
		MaxReadBytes: p.MaxReadBytes,
	})
	res := TelnetRunResult{
		OK: script.OK, Output: script.Output, Error: script.Error,
		LatencyMs: script.LatencyMs, Note: script.Note,
	}
	if len(script.Steps) > 0 && script.Steps[0].Error != "" {
		res.Error = script.Steps[0].Error
		res.OK = false
	}
	return res
}
