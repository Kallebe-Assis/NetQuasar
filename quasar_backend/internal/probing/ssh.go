package probing

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHDialParams teste de autenticação SSH (senha).
type SSHDialParams struct {
	Host     string
	Port     string
	User     string
	Password string
	Timeout  time.Duration
}

// SSHDialResult resultado do handshake SSH.
type SSHDialResult struct {
	OK        bool   `json:"ok"`
	Remote    string `json:"remote_addr,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Note      string `json:"note"`
}

// SSHDialWithPassword abre conexão SSH com senha. HostKey não é verificado (somente ferramentas de diagnóstico).
func SSHDialWithPassword(ctx context.Context, p SSHDialParams) SSHDialResult {
	note := "HostKey do servidor não foi verificado — adequado só para laboratório/diagnóstico; em produção use known_hosts."
	host := strings.TrimSpace(p.Host)
	user := strings.TrimSpace(p.User)
	if host == "" || user == "" {
		return SSHDialResult{OK: false, Note: note, Error: "host e user obrigatórios"}
	}
	port := strings.TrimSpace(p.Port)
	if port == "" {
		port = "22"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return SSHDialResult{OK: false, Note: note, Error: "porta inválida"}
	}
	if p.Timeout <= 0 {
		p.Timeout = 10 * time.Second
	}
	if p.Timeout > 45*time.Second {
		p.Timeout = 45 * time.Second
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(p.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         p.Timeout,
	}

	addr := net.JoinHostPort(host, port)
	t0 := time.Now()

	type res struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan res, 1)
	go func() {
		c, e := ssh.Dial("tcp", addr, cfg)
		ch <- res{c, e}
	}()

	select {
	case <-ctx.Done():
		return SSHDialResult{OK: false, LatencyMs: time.Since(t0).Milliseconds(), Error: ctx.Err().Error(), Note: note}
	case r := <-ch:
		lat := time.Since(t0).Milliseconds()
		if r.err != nil {
			return SSHDialResult{OK: false, LatencyMs: lat, Error: r.err.Error(), Note: note}
		}
		remote := ""
		if r.client != nil {
			if a := r.client.RemoteAddr(); a != nil {
				remote = a.String()
			}
			_ = r.client.Close()
		}
		return SSHDialResult{OK: true, Remote: remote, LatencyMs: lat, Note: note}
	}
}
