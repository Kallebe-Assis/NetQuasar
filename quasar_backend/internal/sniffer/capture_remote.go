package sniffer

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket/pcapgo"
	"golang.org/x/crypto/ssh"
)

type remoteSSHParams struct {
	Host, Port, User, Password, Interface string
}

// captureRemoteSSH liga por SSH a um equipamento e corre `tcpdump` a escrever pcap para o
// stdout (-U força flush por pacote — sem isto o tcpdump só entrega dados quando o buffer
// enche, e a captura pareceria "parada"), lendo os pacotes à medida que chegam. Exclui a
// própria porta 22 para não capturar o tráfego da ligação SSH usada para a captura.
// Requer tcpdump instalado e privilégio de captura (root/CAP_NET_RAW) no equipamento —
// falha com uma mensagem clara caso contrário, em vez de ficar silenciosamente sem pacotes.
func captureRemoteSSH(ctx context.Context, p remoteSSHParams, onPacket func(raw []byte, ts time.Time) bool) error {
	host := strings.TrimSpace(p.Host)
	port := strings.TrimSpace(p.Port)
	if port == "" {
		port = "22"
	}
	cfg := &ssh.ClientConfig{
		User:            p.User,
		Auth:            []ssh.AuthMethod{ssh.Password(p.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, port), cfg)
	if err != nil {
		return fmt.Errorf("ligação SSH falhou: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("sessão SSH falhou: %w", err)
	}
	defer sess.Close()

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	var stderrBuf strings.Builder
	sess.Stderr = &stderrBuf

	iface := strings.TrimSpace(p.Interface)
	if iface == "" {
		iface = "any"
	}
	cmd := fmt.Sprintf("tcpdump -i %s -U -w - -s 0 not port 22", shellQuote(iface))
	if err := sess.Start(cmd); err != nil {
		return fmt.Errorf("não foi possível iniciar tcpdump remoto: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = sess.Signal(ssh.SIGINT)
			_ = client.Close()
		case <-stopped:
		}
	}()
	defer close(stopped)

	r, err := pcapgo.NewReader(bufio.NewReaderSize(stdout, 1<<16))
	if err != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("tcpdump remoto não iniciou (instalado? permissão de captura?): %s", msg)
	}
	for {
		data, ci, rerr := r.ReadPacketData()
		if rerr != nil {
			break
		}
		raw := make([]byte, len(data))
		copy(raw, data)
		if !onPacket(raw, ci.Timestamp) {
			_ = sess.Signal(ssh.SIGINT)
			break
		}
	}

	select {
	case werr := <-done:
		if werr != nil && ctx.Err() == nil {
			if msg := strings.TrimSpace(stderrBuf.String()); msg != "" {
				return fmt.Errorf("tcpdump remoto terminou com erro: %s", msg)
			}
		}
	case <-time.After(2 * time.Second):
	}
	return ctx.Err()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
