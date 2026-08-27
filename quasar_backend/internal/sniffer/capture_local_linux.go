//go:build linux

package sniffer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

// captureLocal captura frames Ethernet brutos via socket AF_PACKET (Linux), sem depender
// de libpcap/cgo — o contentor já corre como root e o Docker concede NET_RAW por omissão,
// por isso funciona sem alterar o docker-compose.yml. Só vê o tráfego de/para a interface
// de rede do próprio contentor (rede Docker), não o tráfego geral da LAN — ver captureRemoteSSH
// para capturar num equipamento remoto.
// ifaceName vazio ou "any" liga a todas as interfaces.
func captureLocal(ctx context.Context, ifaceName string, onPacket func(raw []byte, ts time.Time) bool) error {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		return fmt.Errorf("socket AF_PACKET (requer privilégio de root/NET_RAW no contentor): %w", err)
	}
	defer unix.Close(fd)

	ifIndex := 0
	if ifaceName != "" && ifaceName != "any" && ifaceName != "todas" {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return fmt.Errorf("interface %q não encontrada: %w", ifaceName, err)
		}
		ifIndex = iface.Index
	}
	addr := &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL), Ifindex: ifIndex}
	if err := unix.Bind(fd, addr); err != nil {
		return fmt.Errorf("bind na interface: %w", err)
	}

	// Timeout de leitura curto para reagir ao ctx.Done() sem bloquear indefinidamente num Recvfrom.
	tv := unix.Timeval{Sec: 1}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)

	buf := make([]byte, 65536)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			// EINTR ("interrupted system call") não é um erro real de captura — o runtime do Go
			// (preempção assíncrona de goroutines, desde a 1.14) manda SIGURG periodicamente, o
			// que interrompe uma syscall bloqueante como este Recvfrom. Sem tratar isto a captura
			// parava sozinha após alguns segundos. EAGAIN/EWOULDBLOCK são só o timeout de leitura
			// (SO_RCVTIMEO) — também não é erro, só reentra o loop para checar ctx.Done().
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
				continue
			}
			return err
		}
		if n <= 0 {
			continue
		}
		raw := make([]byte, n)
		copy(raw, buf[:n])
		if !onPacket(raw, time.Now()) {
			return nil // teto de segurança da sessão atingido (ver maxPacketsPerSession/maxBytesPerSession)
		}
	}
}
