package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const bngDeviceSQLFilter = `
	(
		lower(coalesce(d.category,'')) LIKE '%bng%'
		OR lower(coalesce(d.category,'')) LIKE '%concentrador%'
		OR lower(coalesce(d.brand,'')) LIKE '%mikrotik%'
		OR lower(coalesce(d.brand,'')) LIKE '%huawei%'
		OR lower(coalesce(d.description,'')) LIKE '%bng%'
	)
`

type bngSessionOut struct {
	DeviceID          uuid.UUID
	DeviceDescription string
	DeviceIP          string
	Login             string
	InterfaceName     string
	OperStatus        string
	InOctets          uint64
	OutOctets         uint64
	CollectedAt       time.Time
	Source            string
}

var pppoeLoginRe = regexp.MustCompile(`(?i)<pppoe[-_]?([^>]+)>|pppoe[-_]?(.+)`)

func pppoeLoginFromIface(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	if m := pppoeLoginRe.FindStringSubmatch(n); len(m) > 0 {
		for i := 1; i < len(m); i++ {
			if s := strings.TrimSpace(m[i]); s != "" {
				return strings.Trim(s, "<>")
			}
		}
	}
	if strings.Contains(strings.ToLower(n), "pppoe") {
		return n
	}
	return ""
}

func isPPPoEInterfaceName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(n, "pppoe") || strings.HasPrefix(n, "<pppoe")
}

func extractPPPoEFromInterfaceJSON(raw []byte) []map[string]any {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	tab, _ := doc["interface_table"].([]any)
	if len(tab) == 0 {
		return nil
	}
	out := make([]map[string]any, 0)
	for _, row := range tab {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(firstString(m, "display_name", "if_name", "descr", "description"))
		if !isPPPoEInterfaceName(name) {
			continue
		}
		out = append(out, map[string]any{
			"name":              name,
			"login":             pppoeLoginFromIface(name),
			"oper_status":       firstString(m, "oper_status", "oper_status_label"),
			"in_octets":         toUint64(m["in_octets"]),
			"out_octets":        toUint64(m["out_octets"]),
		})
	}
	return out
}

func extractPPPoEFromTelemetryJSON(raw []byte) []map[string]any {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	mc, _ := doc["mikrotik_collection"].(map[string]any)
	fields, _ := mc["fields"].(map[string]any)
	pppField, _ := fields["pppoe_active_sessions"].(map[string]any)
	sessions, _ := pppField["pppoe_sessions"].([]any)
	if len(sessions) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(firstString(m, "name"))
		out = append(out, map[string]any{
			"name":        name,
			"login":       pppoeLoginFromIface(name),
			"oper_status": firstString(m, "oper_status_label", "oper_status"),
			"in_octets":   toUint64(m["in_octets"]),
			"out_octets":  toUint64(m["out_octets"]),
			"if_index":    m["if_index"],
		})
	}
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case float64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case json.Number:
		n, _ := x.Int64()
		if n < 0 {
			return 0
		}
		return uint64(n)
	default:
		return 0
	}
}

func (s *Server) loadBngSessions(ctx context.Context, search string, deviceID *uuid.UUID) ([]bngSessionOut, []map[string]any, error) {
	q := `
		SELECT d.id, coalesce(d.description,''), coalesce(host(d.ip)::text,'')
		FROM devices d
		WHERE ` + bngDeviceSQLFilter
	args := []any{}
	n := 1
	if deviceID != nil {
		q += ` AND d.id = $` + strconv.Itoa(n)
		args = append(args, *deviceID)
		n++
	}
	q += ` ORDER BY d.description LIMIT 50`
	rows, err := s.DB().Query(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	search = strings.ToLower(strings.TrimSpace(search))
	var sessions []bngSessionOut
	devicesMeta := make([]map[string]any, 0)

	for rows.Next() {
		var id uuid.UUID
		var desc, ip string
		if err := rows.Scan(&id, &desc, &ip); err != nil {
			return nil, nil, err
		}

		var telAt *time.Time
		var telRaw []byte
		_ = s.DB().QueryRow(ctx, `
			SELECT collected_at, metrics::text FROM telemetry_samples
			WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
		`, id).Scan(&telAt, &telRaw)

		var ifAt *time.Time
		var ifRaw []byte
		_ = s.DB().QueryRow(ctx, `
			SELECT collected_at, interfaces::text FROM interface_snapshots
			WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
		`, id).Scan(&ifAt, &ifRaw)

		var parsed []map[string]any
		var collected time.Time
		source := ""
		if len(telRaw) > 0 && telAt != nil {
			parsed = extractPPPoEFromTelemetryJSON(telRaw)
			if len(parsed) > 0 {
				collected = *telAt
				source = "telemetry"
			}
		}
		if len(parsed) == 0 && len(ifRaw) > 0 && ifAt != nil {
			parsed = extractPPPoEFromInterfaceJSON(ifRaw)
			if len(parsed) > 0 {
				collected = *ifAt
				source = "interfaces"
			}
		}

		devicesMeta = append(devicesMeta, map[string]any{
			"device_id": id, "description": desc, "ip": ip,
			"sessions_count": len(parsed), "source": source, "collected_at": collected,
		})

		for _, row := range parsed {
			login := strings.TrimSpace(firstString(row, "login"))
			iface := strings.TrimSpace(firstString(row, "name"))
			if search != "" {
				hay := strings.ToLower(login + " " + iface + " " + desc + " " + ip)
				if !strings.Contains(hay, search) {
					continue
				}
			}
			sessions = append(sessions, bngSessionOut{
				DeviceID:          id,
				DeviceDescription: desc,
				DeviceIP:          ip,
				Login:             login,
				InterfaceName:     iface,
				OperStatus:        firstString(row, "oper_status"),
				InOctets:          toUint64(row["in_octets"]),
				OutOctets:         toUint64(row["out_octets"]),
				CollectedAt:       collected,
				Source:            source,
			})
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].DeviceDescription != sessions[j].DeviceDescription {
			return sessions[i].DeviceDescription < sessions[j].DeviceDescription
		}
		return sessions[i].Login < sessions[j].Login
	})
	return sessions, devicesMeta, nil
}

func bngSessionsToJSON(sessions []bngSessionOut) []map[string]any {
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, map[string]any{
			"device_id":          s.DeviceID,
			"device_description": s.DeviceDescription,
			"device_ip":          s.DeviceIP,
			"login":              s.Login,
			"interface_name":     s.InterfaceName,
			"oper_status":        s.OperStatus,
			"in_octets":          s.InOctets,
			"out_octets":         s.OutOctets,
			"collected_at":       s.CollectedAt,
			"source":             s.Source,
		})
	}
	return out
}

func (s *Server) bngSessions(w http.ResponseWriter, r *http.Request) {
	var deviceID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("device_id")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_QUERY", "device_id inválido", nil)
			return
		}
		deviceID = &id
	}
	sessions, devices, err := s.loadBngSessions(r.Context(), "", deviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": bngSessionsToJSON(sessions),
		"devices":  devices,
		"note":     "Sessões PPPoE activas a partir de telemetria/interface (MikroTik IF-MIB). Clientes offline cadastrados não aparecem.",
	})
}

func (s *Server) bngSessionsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "query q obrigatória", nil)
		return
	}
	sessions, devices, err := s.loadBngSessions(r.Context(), q, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"q":        q,
		"sessions": bngSessionsToJSON(sessions),
		"devices":  devices,
	})
}

func (s *Server) bngStatsSummary(w http.ResponseWriter, r *http.Request) {
	sessions, devices, err := s.loadBngSessions(r.Context(), "", nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	online := 0
	for _, s := range sessions {
		st := strings.ToLower(strings.TrimSpace(s.OperStatus))
		if st == "up" || st == "1" {
			online++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bng_devices":     len(devices),
		"sessions_total":  len(sessions),
		"sessions_online": online,
		"sessions_offline": len(sessions) - online,
	})
}

func (s *Server) bngTrafficUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	sessions, _, err := s.loadBngSessions(r.Context(), "", nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	sort.Slice(sessions, func(i, j int) bool {
		return (sessions[i].InOctets + sessions[i].OutOctets) > (sessions[j].InOctets + sessions[j].OutOctets)
	})
	if len(sessions) > limit {
		sessions = sessions[:limit]
	}
	rows := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, map[string]any{
			"login":          s.Login,
			"device_ip":      s.DeviceIP,
			"in_octets":      s.InOctets,
			"out_octets":     s.OutOctets,
			"total_octets":   s.InOctets + s.OutOctets,
			"collected_at":   s.CollectedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": rows})
}

func (s *Server) bngAuthLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, created_at, entity_type, entity_id, action, actor
		FROM ops_audit_log
		WHERE entity_type IN ('auth', 'client_connection')
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id int64
		var created time.Time
		var entityType, entityID, action string
		var actor *string
		if err := rows.Scan(&id, &created, &entityType, &entityID, &action, &actor); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "created_at": created, "entity_type": entityType,
			"entity_id": entityID, "action": action, "actor": actor,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs": list,
		"note": "Autenticações RADIUS externas requerem integração dedicada; aqui: auditoria de login da plataforma e alterações em conexões.",
	})
}
