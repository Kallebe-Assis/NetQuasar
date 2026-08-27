package sniffer

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// WritePcapFile grava os pacotes (com payload completo) num ficheiro .pcap real —
// pode ser aberto directamente no Wireshark.
func WritePcapFile(path string, packets []Packet) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		return err
	}
	for _, p := range packets {
		ci := gopacket.CaptureInfo{Timestamp: p.Ts, CaptureLength: len(p.Raw), Length: p.Length}
		if err := w.WritePacket(ci, p.Raw); err != nil {
			return err
		}
	}
	return nil
}

// PacketFilter é o filtro de pesquisa aplicado a uma captura guardada (IP, protocolo,
// tamanho, data, texto livre) — espelha os filtros pedidos na aba Sniffer.
type PacketFilter struct {
	IP       string
	Protocol string
	MinLen   int
	MaxLen   int
	From     *time.Time
	To       *time.Time
	Q        string
}

func (f PacketFilter) Match(p Packet) bool {
	if f.IP != "" && !strings.Contains(p.SrcIP, f.IP) && !strings.Contains(p.DstIP, f.IP) {
		return false
	}
	if f.Protocol != "" && !strings.EqualFold(p.Protocol, f.Protocol) {
		return false
	}
	if f.MinLen > 0 && p.Length < f.MinLen {
		return false
	}
	if f.MaxLen > 0 && p.Length > f.MaxLen {
		return false
	}
	if f.From != nil && p.Ts.Before(*f.From) {
		return false
	}
	if f.To != nil && p.Ts.After(*f.To) {
		return false
	}
	if f.Q != "" {
		q := strings.ToLower(f.Q)
		hay := strings.ToLower(p.SrcIP + " " + p.DstIP + " " + p.Protocol + " " + p.Info)
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

// ReadPcapFile aplica o filtro a todos os pacotes do ficheiro e devolve até `limit`
// correspondências, mais o total de correspondências (para paginação/contagem no UI).
func ReadPcapFile(path string, filter PacketFilter, limit int) ([]Packet, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		return nil, 0, err
	}
	var out []Packet
	total := 0
	seq := 0
	for {
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, total, err
		}
		seq++
		pk := DecodePacket(data)
		pk.Seq = seq
		pk.Ts = ci.Timestamp
		pk.Length = ci.Length
		if !filter.Match(pk) {
			continue
		}
		total++
		if len(out) < limit {
			out = append(out, pk)
		}
	}
	return out, total, nil
}

// ReadPcapPacketBySeq relê o ficheiro para obter um único pacote (com Raw, para hexdump).
// Aceitável porque as capturas são limitadas em tamanho (ver maxPacketsPerSession).
func ReadPcapPacketBySeq(path string, seq int) (*Packet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		return nil, err
	}
	i := 0
	for {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			return nil, err
		}
		i++
		if i == seq {
			pk := DecodePacket(data)
			pk.Seq = i
			pk.Ts = ci.Timestamp
			pk.Length = ci.Length
			raw := make([]byte, len(data))
			copy(raw, data)
			pk.Raw = raw
			return &pk, nil
		}
	}
}
