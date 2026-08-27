package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/sniffer"
)

type packetOut struct {
	Seq      int    `json:"seq"`
	Ts       string `json:"ts"`
	Length   int    `json:"length"`
	Protocol string `json:"protocol"`
	SrcIP    string `json:"src_ip,omitempty"`
	DstIP    string `json:"dst_ip,omitempty"`
	SrcPort  int    `json:"src_port,omitempty"`
	DstPort  int    `json:"dst_port,omitempty"`
	Info     string `json:"info"`
}

func toPacketOut(p sniffer.Packet) packetOut {
	return packetOut{
		Seq: p.Seq, Ts: p.Ts.UTC().Format(time.RFC3339Nano), Length: p.Length, Protocol: p.Protocol,
		SrcIP: p.SrcIP, DstIP: p.DstIP, SrcPort: p.SrcPort, DstPort: p.DstPort, Info: p.Info,
	}
}

func packetDetailOut(p sniffer.Packet) map[string]any {
	return map[string]any{
		"seq": p.Seq, "ts": p.Ts.UTC().Format(time.RFC3339Nano), "length": p.Length,
		"protocol": p.Protocol, "src_ip": p.SrcIP, "dst_ip": p.DstIP,
		"src_port": p.SrcPort, "dst_port": p.DstPort, "info": p.Info,
		"hex": hexDump(p.Raw),
	}
}

func hexDump(raw []byte) string {
	var sb strings.Builder
	for i := 0; i < len(raw); i += 16 {
		end := i + 16
		if end > len(raw) {
			end = len(raw)
		}
		chunk := raw[i:end]
		sb.WriteString(fmt.Sprintf("%04x  ", i))
		for j := 0; j < 16; j++ {
			if j < len(chunk) {
				sb.WriteString(fmt.Sprintf("%02x ", chunk[j]))
			} else {
				sb.WriteString("   ")
			}
			if j == 7 {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(" ")
		for _, b := range chunk {
			if b >= 32 && b < 127 {
				sb.WriteByte(b)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// --- interfaces disponíveis (captura local) -------------------------------------------------

func (s *Server) snifferInterfaces(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{{"name": "any", "label": "Todas as interfaces"}}
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 {
				continue
			}
			out = append(out, map[string]any{"name": ifc.Name, "label": ifc.Name})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"interfaces": out})
}

// --- sessões ao vivo -------------------------------------------------------------------------

type snifferStartRequest struct {
	Source    string `json:"source"` // "local" | "device"
	Interface string `json:"interface"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
}

func (s *Server) snifferStart(w http.ResponseWriter, r *http.Request) {
	var body snifferStartRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Captura " + time.Now().Format("02/01 15:04:05")
	}

	switch strings.TrimSpace(body.Source) {
	case "", "local":
		sess, err := s.Sniffer.StartLocal(sniffer.StartLocalParams{Name: name, Interface: strings.TrimSpace(body.Interface)})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "SNIFFER", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": sess.ID.String()})

	case "device":
		devID, err := uuid.Parse(strings.TrimSpace(body.DeviceID))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "device_id inválido", nil)
			return
		}
		var ip, sshUser, sshPass *string
		err = s.DB().QueryRow(r.Context(),
			`SELECT host(ip)::text, ssh_user, ssh_password FROM devices WHERE id=$1`, devID,
		).Scan(&ip, &sshUser, &sshPass)
		if err != nil {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
			return
		}
		if ip == nil || strings.TrimSpace(*ip) == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "equipamento sem IP configurado", nil)
			return
		}
		if sshUser == nil || strings.TrimSpace(*sshUser) == "" || sshPass == nil {
			writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
				"configure utilizador/palavra-passe SSH deste equipamento em Equipamentos → editar", nil)
			return
		}
		sess, err := s.Sniffer.StartDevice(sniffer.StartDeviceParams{
			Name: name, DeviceID: devID, Host: strings.TrimSpace(*ip), Port: "22",
			User: strings.TrimSpace(*sshUser), Password: *sshPass, Interface: strings.TrimSpace(body.Interface),
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "SNIFFER", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": sess.ID.String()})

	default:
		writeErr(w, http.StatusBadRequest, "VALIDATION", "source inválido (use local ou device)", nil)
	}
}

func (s *Server) snifferSessionStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	sess, ok := s.Sniffer.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "sessão não encontrada (talvez já guardada/descartada)", nil)
		return
	}
	count, totalBytes := sess.Count()
	out := map[string]any{
		"id": sess.ID.String(), "name": sess.Name, "source": string(sess.Source),
		"interface": sess.Interface, "status": string(sess.Status),
		"started_at": sess.StartedAt.UTC().Format(time.RFC3339),
		"packet_count": count, "total_bytes": totalBytes,
	}
	if sess.DeviceID != nil {
		out["device_id"] = sess.DeviceID.String()
	}
	if sess.StoppedAt != nil {
		out["stopped_at"] = sess.StoppedAt.UTC().Format(time.RFC3339)
	}
	if sess.Err != "" {
		out["error"] = sess.Err
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) snifferSessionPackets(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	sess, ok := s.Sniffer.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "sessão não encontrada", nil)
		return
	}
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	pkts := sess.SnapshotSince(since)
	out := make([]packetOut, 0, len(pkts))
	for _, p := range pkts {
		out = append(out, toPacketOut(p))
	}
	count, totalBytes := sess.Count()
	writeJSON(w, http.StatusOK, map[string]any{
		"packets": out, "packet_count": count, "total_bytes": totalBytes, "status": string(sess.Status),
	})
}

func (s *Server) snifferSessionPacketDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	seq, err := strconv.Atoi(chi.URLParam(r, "seq"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_SEQ", "", nil)
		return
	}
	sess, ok := s.Sniffer.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "sessão não encontrada", nil)
		return
	}
	for _, p := range sess.All() {
		if p.Seq == seq {
			writeJSON(w, http.StatusOK, packetDetailOut(p))
			return
		}
	}
	writeErr(w, http.StatusNotFound, "NOT_FOUND", "pacote não encontrado", nil)
}

func (s *Server) snifferSessionStop(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	if err := s.Sniffer.Stop(id); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type snifferSaveRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) snifferSessionSave(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	sess, ok := s.Sniffer.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "sessão não encontrada", nil)
		return
	}
	var body snifferSaveRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = sess.Name
	}
	source, iface, startedAt, stoppedAt, deviceID := sess.Source, sess.Interface, sess.StartedAt, sess.StoppedAt, sess.DeviceID

	path, count, totalBytes, err := s.Sniffer.Save(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SNIFFER", err.Error(), nil)
		return
	}
	var newID uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO network_captures (name, description, source, device_id, interface, started_at, stopped_at, packet_count, total_bytes, file_path)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, name, strings.TrimSpace(body.Description), string(source), deviceID, iface, startedAt, stoppedAt, count, totalBytes, path,
	).Scan(&newID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": newID.String(), "packet_count": count, "total_bytes": totalBytes})
}

func (s *Server) snifferSessionDiscard(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	s.Sniffer.Discard(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- capturas guardadas ------------------------------------------------------------------------

func (s *Server) snifferCapturesList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, name, COALESCE(description,''), source, device_id, COALESCE(interface,''),
			started_at, stopped_at, packet_count, total_bytes
		FROM network_captures ORDER BY started_at DESC LIMIT 200
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var name, desc, source, iface string
		var deviceID *uuid.UUID
		var startedAt time.Time
		var stoppedAt *time.Time
		var count int
		var totalBytes int64
		if rows.Scan(&id, &name, &desc, &source, &deviceID, &iface, &startedAt, &stoppedAt, &count, &totalBytes) != nil {
			continue
		}
		item := map[string]any{
			"id": id.String(), "name": name, "description": desc, "source": source, "interface": iface,
			"started_at": startedAt.UTC().Format(time.RFC3339), "packet_count": count, "total_bytes": totalBytes,
		}
		if deviceID != nil {
			item["device_id"] = deviceID.String()
		}
		if stoppedAt != nil {
			item["stopped_at"] = stoppedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"captures": out})
}

func (s *Server) snifferCaptureFilePath(ctx context.Context, id uuid.UUID) (string, error) {
	var path string
	err := s.DB().QueryRow(ctx, `SELECT file_path FROM network_captures WHERE id=$1`, id).Scan(&path)
	return path, err
}

func (s *Server) snifferCapturePackets(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	path, err := s.snifferCaptureFilePath(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "captura não encontrada", nil)
		return
	}

	q := r.URL.Query()
	filter := sniffer.PacketFilter{
		IP:       strings.TrimSpace(q.Get("ip")),
		Protocol: strings.TrimSpace(q.Get("protocol")),
		Q:        strings.TrimSpace(q.Get("q")),
	}
	if v, err := strconv.Atoi(q.Get("min_len")); err == nil {
		filter.MinLen = v
	}
	if v, err := strconv.Atoi(q.Get("max_len")); err == nil {
		filter.MaxLen = v
	}
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &t
		}
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &t
		}
	}
	limit := 500
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 2000 {
		limit = v
	}

	pkts, total, err := sniffer.ReadPcapFile(path, filter, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "PCAP", err.Error(), nil)
		return
	}
	out := make([]packetOut, 0, len(pkts))
	for _, p := range pkts {
		out = append(out, toPacketOut(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"packets": out, "matched": total, "limit": limit})
}

func (s *Server) snifferCapturePacketDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	seq, err := strconv.Atoi(chi.URLParam(r, "seq"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_SEQ", "", nil)
		return
	}
	path, err := s.snifferCaptureFilePath(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "captura não encontrada", nil)
		return
	}
	p, err := sniffer.ReadPcapPacketBySeq(path, seq)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "pacote não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, packetDetailOut(*p))
}

func (s *Server) snifferCaptureDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	path, err := s.snifferCaptureFilePath(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "captura não encontrada", nil)
		return
	}
	if _, err := s.DB().Exec(r.Context(), `DELETE FROM network_captures WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	_ = os.Remove(path)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
