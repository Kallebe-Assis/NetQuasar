package sniffer

import (
	"fmt"
	"net"
	"strings"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// DecodePacket decodifica um frame Ethernet bruto o suficiente para listagem/filtro
// (IPs, portas, protocolo, uma linha de resumo tipo Wireshark). Seq/Ts/Raw ficam a
// cargo do chamador (dependem de onde o pacote veio — captura ao vivo ou ficheiro pcap).
func DecodePacket(data []byte) Packet {
	p := Packet{Length: len(data)}
	pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	if netLayer := pkt.NetworkLayer(); netLayer != nil {
		src, dst := netLayer.NetworkFlow().Endpoints()
		p.SrcIP = src.String()
		p.DstIP = dst.String()
	}

	proto := ""
	var info []string

	if arpLayer := pkt.Layer(layers.LayerTypeARP); arpLayer != nil {
		arp, _ := arpLayer.(*layers.ARP)
		proto = "ARP"
		if arp != nil {
			p.SrcIP = net.IP(arp.SourceProtAddress).String()
			p.DstIP = net.IP(arp.DstProtAddress).String()
			if arp.Operation == layers.ARPReply {
				info = append(info, fmt.Sprintf("%s está em %s", p.SrcIP, net.HardwareAddr(arp.SourceHwAddress).String()))
			} else {
				info = append(info, fmt.Sprintf("Quem tem %s? Diga a %s", p.DstIP, p.SrcIP))
			}
		}
	}

	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		if tcp, ok := tcpLayer.(*layers.TCP); ok {
			proto = "TCP"
			p.SrcPort = int(tcp.SrcPort)
			p.DstPort = int(tcp.DstPort)
			info = append(info, fmt.Sprintf("%d → %d [%s] Seq=%d Ack=%d Win=%d Len=%d",
				p.SrcPort, p.DstPort, tcpFlags(tcp), tcp.Seq, tcp.Ack, tcp.Window, len(tcp.Payload)))
		}
	} else if udpLayer := pkt.Layer(layers.LayerTypeUDP); udpLayer != nil {
		if udp, ok := udpLayer.(*layers.UDP); ok {
			proto = "UDP"
			p.SrcPort = int(udp.SrcPort)
			p.DstPort = int(udp.DstPort)
			info = append(info, fmt.Sprintf("%d → %d Len=%d", p.SrcPort, p.DstPort, len(udp.Payload)))
		}
	} else if icmpLayer := pkt.Layer(layers.LayerTypeICMPv4); icmpLayer != nil {
		if icmp, ok := icmpLayer.(*layers.ICMPv4); ok {
			proto = "ICMP"
			info = append(info, icmp.TypeCode.String())
		}
	} else if _, ok := pkt.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6); ok {
		proto = "ICMPv6"
	}

	// DNS por cima de UDP/TCP — refina o protocolo e a info quando aplicável.
	if dnsLayer := pkt.Layer(layers.LayerTypeDNS); dnsLayer != nil {
		if dns, ok := dnsLayer.(*layers.DNS); ok {
			proto = "DNS"
			switch {
			case dns.QR && len(dns.Answers) > 0:
				info = []string{fmt.Sprintf("Resposta (%d registo(s))", len(dns.Answers))}
			case dns.QR:
				info = []string{"Resposta"}
			case len(dns.Questions) > 0:
				info = []string{"Consulta " + string(dns.Questions[0].Name)}
			}
		}
	}

	if proto == "" {
		switch {
		case pkt.NetworkLayer() != nil:
			proto = strings.ToUpper(pkt.NetworkLayer().LayerType().String())
		case pkt.LinkLayer() != nil:
			proto = strings.ToUpper(pkt.LinkLayer().LayerType().String())
		default:
			proto = "OUTRO"
		}
	}

	p.Protocol = proto
	p.Info = strings.Join(info, " ")
	if p.Info == "" {
		p.Info = fmt.Sprintf("%d bytes", len(data))
	}
	return p
}

func tcpFlags(t *layers.TCP) string {
	var f []string
	if t.SYN {
		f = append(f, "SYN")
	}
	if t.ACK {
		f = append(f, "ACK")
	}
	if t.FIN {
		f = append(f, "FIN")
	}
	if t.RST {
		f = append(f, "RST")
	}
	if t.PSH {
		f = append(f, "PSH")
	}
	if t.URG {
		f = append(f, "URG")
	}
	if len(f) == 0 {
		return "-"
	}
	return strings.Join(f, ",")
}
