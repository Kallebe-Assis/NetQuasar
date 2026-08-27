// Package sniffer implementa a captura de tráfego de rede (aba Sniffer, em Ferramentas):
// captura local (socket AF_PACKET, sem depender de libpcap/cgo — só Linux) ou remota via
// SSH + tcpdump num equipamento, com decodificação básica de camadas (Ethernet/ARP/IP/
// TCP/UDP/ICMP/DNS) ao estilo Wireshark, e leitura/escrita de ficheiros .pcap reais.
package sniffer

import "time"

// Packet é um pacote capturado, já decodificado o suficiente para listagem/filtro.
// Raw só é preenchido quando necessário (detalhe de um pacote, ou antes de gravar em .pcap) —
// evita manter o payload completo duplicado nas respostas de listagem.
type Packet struct {
	Seq      int
	Ts       time.Time
	Length   int
	Protocol string
	SrcIP    string
	DstIP    string
	SrcPort  int
	DstPort  int
	Info     string
	Raw      []byte
}
