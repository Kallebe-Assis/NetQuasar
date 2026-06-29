package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/bngcollect"
)

const bngDeviceSQLFilter = `COALESCE(d.bng_enabled, false) = true`

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

type bngDeviceRow struct {
	ID          uuid.UUID
	Description string
	IP          string
	Brand       string
	Model       string
	Category    string
}

func (s *Server) resolveBngDevice(ctx context.Context, id uuid.UUID) (bngDeviceRow, string, error) {
	var row bngDeviceRow
	err := s.DB().QueryRow(ctx, `
		SELECT d.id, coalesce(d.description,''), coalesce(host(d.ip)::text,''),
			coalesce(d.brand,''), coalesce(d.model,''), coalesce(d.category,'')
		FROM devices d
		WHERE d.id=$1 AND `+bngDeviceSQLFilter, id,
	).Scan(&row.ID, &row.Description, &row.IP, &row.Brand, &row.Model, &row.Category)
	if err != nil {
		return row, "", err
	}
	var devComm *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM devices WHERE id=$1`, id).Scan(&devComm)
	var defComm *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
	comm := ""
	if devComm != nil && strings.TrimSpace(*devComm) != "" {
		comm = strings.TrimSpace(*devComm)
	} else if defComm != nil {
		comm = strings.TrimSpace(*defComm)
	}
	return row, comm, nil
}

func (s *Server) bngListDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, coalesce(d.description,''), coalesce(host(d.ip)::text,''),
			coalesce(d.brand,''), coalesce(d.model,''), coalesce(d.category,'')
		FROM devices d
		WHERE `+bngDeviceSQLFilter+`
		ORDER BY d.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, ip, brand, model, cat string
		if err := rows.Scan(&id, &desc, &ip, &brand, &model, &cat); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, map[string]any{
			"id": id, "description": desc, "ip": ip,
			"brand": brand, "model": model, "category": cat,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}

func (s *Server) bngDeviceOverview(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	dev, _, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}

	var telAt *time.Time
	var telRaw []byte
	_ = s.DB().QueryRow(r.Context(), `
		SELECT collected_at, metrics::text FROM telemetry_samples
		WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, id).Scan(&telAt, &telRaw)

	fields := map[string]any{}
	if len(telRaw) > 0 {
		var doc map[string]any
		if json.Unmarshal(telRaw, &doc) == nil {
			if bc, ok := doc["bng_collection"].(map[string]any); ok {
				if f, ok := bc["fields"].(map[string]any); ok {
					for k, v := range f {
						if fm, ok := v.(map[string]any); ok && fm["ok"] == true {
							fields[k] = fm["value"]
						}
					}
				}
			}
		}
	}

	var statsAt *time.Time
	var total, pppoe, ipv4, ipv6, dual *int
	_ = s.DB().QueryRow(r.Context(), `
		SELECT collected_at, total_online, pppoe_online, ipv4_online, ipv6_online, dual_stack_online
		FROM bng_stats_samples WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, id).Scan(&statsAt, &total, &pppoe, &ipv4, &ipv6, &dual)

	writeJSON(w, http.StatusOK, map[string]any{
		"device": map[string]any{
			"id": dev.ID, "description": dev.Description, "ip": dev.IP,
			"brand": dev.Brand, "model": dev.Model, "category": dev.Category,
		},
		"telemetry_collected_at": telAt,
		"fields":                 fields,
		"latest_stats": map[string]any{
			"collected_at":       statsAt,
			"total_online":       total,
			"pppoe_online":       pppoe,
			"ipv4_online":        ipv4,
			"ipv6_online":        ipv6,
			"dual_stack_online":  dual,
		},
	})
}

func (s *Server) bngStatsHistory(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "query device_id obrigatório", nil)
		return
	}
	did, err := uuid.Parse(deviceID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "device_id inválido", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 120
	}
	rows, err := s.DB().Query(r.Context(), `
		SELECT collected_at, total_online, pppoe_online, ipv4_online, ipv6_online, dual_stack_online
		FROM bng_stats_samples
		WHERE device_id=$1
		ORDER BY collected_at DESC
		LIMIT $2
	`, did, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var samples []map[string]any
	for rows.Next() {
		var ts time.Time
		var total, pppoe, ipv4, ipv6, dual *int
		if err := rows.Scan(&ts, &total, &pppoe, &ipv4, &ipv6, &dual); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		samples = append(samples, map[string]any{
			"collected_at": ts, "total_online": total, "pppoe_online": pppoe,
			"ipv4_online": ipv4, "ipv6_online": ipv6, "dual_stack_online": dual,
		})
	}
	for i, j := 0, len(samples)-1; i < j; i, j = i+1, j-1 {
		samples[i], samples[j] = samples[j], samples[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": samples})
}

func (s *Server) bngDeviceSessions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	if _, _, err := s.resolveBngDevice(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	sessions, capturedAt, source, note := s.loadCachedBngSessions(r.Context(), id)
	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	sessions = normalizeCachedSessionLogins(sessions, profile.Options.PPPoELoginStripSuffix)
	sessions = bngcollect.EnrichSessionMaps(sessions)
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":    id,
		"sessions":     sessions,
		"captured_at":  capturedAt,
		"source":       source,
		"note":         note,
		"count":        len(sessions),
	})
}

func (s *Server) loadCachedBngSessions(ctx context.Context, deviceID uuid.UUID) ([]map[string]any, *time.Time, string, string) {
	var capturedAt time.Time
	var label string
	var raw []byte
	err := s.DB().QueryRow(ctx, `
		SELECT captured_at, label, data::text FROM bng_session_snapshots
		WHERE device_id=$1 ORDER BY captured_at DESC LIMIT 1
	`, deviceID).Scan(&capturedAt, &label, &raw)
	if err != nil {
		return nil, nil, "", "Nenhuma consulta completa guardada. Execute «Consulta completa SNMP» na aba Sessões."
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return nil, &capturedAt, label, "Snapshot inválido."
	}
	sess, _ := doc["sessions"].([]any)
	out := make([]map[string]any, 0, len(sess))
	for _, row := range sess {
		if m, ok := row.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, &capturedAt, label, ""
}

func (s *Server) bngDeviceSessionsCollect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}
	if !s.bngCollectProgress.start(id) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "already_running",
			"message": "Já existe uma consulta completa em curso neste equipamento.",
			"status":  s.bngCollectProgress.get(id),
		})
		return
	}
	host := strings.TrimSpace(dev.IP)
	go s.runBngSessionsCollect(id, host, comm)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":  true,
		"device_id": id,
		"message":   "Consulta SNMP iniciada. Acompanhe o progresso em /sessions/collect/status.",
	})
}

func (s *Server) bngDeviceSessionsCollectStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.bngCollectProgress.get(id))
}

func (s *Server) runBngSessionsCollect(deviceID uuid.UUID, host, comm string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	profile := bngcollect.LoadGlobalProfile(ctx, s.DB())
	reporter := &bngcollect.CollectProgressReporter{
		OnLoginsLoaded: func(n int) {
			s.bngCollectProgress.update(deviceID, func(j *bngCollectJob) {
				j.LoginsLoaded = n
				j.Phase = "login"
				j.Message = fmt.Sprintf("%d logins carregados…", n)
			})
		},
		OnSessionsLoaded: func(enriched, total int) {
			s.bngCollectProgress.update(deviceID, func(j *bngCollectJob) {
				j.SessionsEnriched = enriched
				j.SessionsTotal = total
				j.Phase = "details"
				j.Message = fmt.Sprintf("%d / %d sessões detalhadas…", enriched, total)
			})
		},
		OnPhase: func(key, label string) {
			s.bngCollectProgress.update(deviceID, func(j *bngCollectJob) {
				j.Phase = key
				j.Message = label
			})
		},
	}

	out, sessions := bngcollect.CollectSessionsWalk(ctx, host, comm, profile, 5*time.Minute, reporter)
	if ctx.Err() != nil {
		s.bngCollectProgress.finish(deviceID, 0, ctx.Err().Error())
		return
	}
	if len(sessions) == 0 && out.Status.Message != "" {
		s.bngCollectProgress.finish(deviceID, 0, out.Status.Message)
		return
	}
	if err := bngcollect.StoreSessionSnapshot(ctx, s.DB(), deviceID, sessions, "snmp_access_table"); err != nil {
		s.bngCollectProgress.finish(deviceID, 0, err.Error())
		return
	}
	infra := bngcollect.CollectInfrastructure(ctx, host, comm, 2*time.Minute)
	_ = bngcollect.StoreInfrastructureSnapshot(ctx, s.DB(), deviceID, infra)
	_ = out
	s.bngCollectProgress.finish(deviceID, len(sessions), "")
}

func (s *Server) bngDeviceCollect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, err := bngcollect.CollectAndStorePeriodic(ctx, s.DB(), id, strings.TrimSpace(dev.IP), comm, 45*time.Second)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "COLLECT", err.Error(), nil)
		return
	}
	st := bngcollect.ExtractStatsTotals(out)
	alertthresholds.EvaluateBngSubscriberDropAlerts(ctx, s.DB(), &s.Log, id, strings.TrimSpace(dev.Description), strings.TrimSpace(dev.IP), "bng_manual_collect")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "device_id": id, "collection": out, "latest_stats": st,
	})
}

func sessionRowToJSON(row bngcollect.SessionRow) map[string]any {
	return bngcollect.EnrichSessionRow(row)
}

func (s *Server) bngDeviceSessionReport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	if _, _, err := s.resolveBngDevice(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	sessions, capturedAt, source, note := s.loadCachedBngSessions(r.Context(), id)
	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	sessions = normalizeCachedSessionLogins(sessions, profile.Options.PPPoELoginStripSuffix)
	sessions = bngcollect.EnrichSessionMaps(sessions)
	report := bngcollect.BuildSessionReportFromMaps(sessions)
	infra, infraAt, hasInfra := bngcollect.LoadLatestInfrastructureSnapshot(r.Context(), s.DB(), id)
	resp := map[string]any{
		"device_id":     id,
		"captured_at":   capturedAt,
		"source":        source,
		"note":          note,
		"session_count": report.SessionCount,
		"report":        report,
	}
	if hasInfra {
		resp["infrastructure"] = infra
		resp["infrastructure_captured_at"] = infraAt
	} else {
		resp["infrastructure_note"] = "Execute a coleta completa SNMP ou «Coletar totais agora» para obter pools, RADIUS, CGN e energia."
	}
	writeJSON(w, http.StatusOK, resp)
}

func normalizeCachedSessionLogins(sessions []map[string]any, stripSuffix string) []map[string]any {
	for _, s := range sessions {
		if l, ok := s["login"]; ok {
			s["login"] = bngcollect.NormalizeSNMPLoginValue(fmt.Sprint(l), stripSuffix)
		}
	}
	return sessions
}

func (s *Server) bngDeviceSessionLookup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("login"))
	}
	if q == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "query q ou login obrigatória", nil)
		return
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}

	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	hintIndex := bngcollect.FindSessionIndexInLatestSnapshot(r.Context(), s.DB(), id, q, profile.Options.PPPoELoginStripSuffix)
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	row, found, err := bngcollect.LookupSessionByLogin(ctx, strings.TrimSpace(dev.IP), comm, q, profile, 25*time.Second, hintIndex)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SNMP", err.Error(), nil)
		return
	}

	if !found {
		writeJSON(w, http.StatusOK, map[string]any{
			"found": false, "source": "snmp_live", "query": q,
			"session": sessionRowToJSON(row),
			"note":    "Utilizador não encontrado online no BNG.",
		})
		return
	}
	if err := bngcollect.UpsertSessionInLatestSnapshot(r.Context(), s.DB(), id, row, profile.Options.PPPoELoginStripSuffix); err != nil {
		s.Log.Warn().Err(err).Str("device_id", id.String()).Str("login", q).Msg("bng session lookup: falha ao actualizar snapshot")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"found": true, "source": "snmp_live", "query": q,
		"session": sessionRowToJSON(row), "list_updated": true,
	})
}

func (s *Server) bngDeviceSessionAuthLogs(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("login"))
	}
	if q == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "query q ou login obrigatória", nil)
		return
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}

	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	authLogs := bngcollect.FetchAuthAttemptsForLogin(ctx, strings.TrimSpace(dev.IP), comm, q, 30*time.Second, profile.Options.PPPoELoginStripSuffix)

	if sessions, _, _, _ := s.loadCachedBngSessions(r.Context(), id); len(sessions) > 0 {
		targets := bngcollect.PPPoELoginLookupTargets(q, profile.Options.PPPoELoginStripSuffix)
		for _, sm := range sessions {
			loginVal := strings.TrimSpace(fmt.Sprint(sm["login"]))
			matched := false
			for _, t := range targets {
				if bngcollect.MatchPPPoELogin(t, loginVal, profile.Options.PPPoELoginStripSuffix) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			row := bngcollect.SessionRowFromMap(sm)
			authLogs = append(bngcollect.AuthLogsFromSession(row, profile.Options.PPPoELoginStripSuffix), authLogs...)
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query": q, "auth_attempts": authLogs,
	})
}

func (s *Server) bngDeviceSessionTrafficRate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	idx := strings.TrimSpace(r.URL.Query().Get("index"))
	if idx == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "index obrigatório", nil)
		return
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}

	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	rate, err := bngcollect.MeasureSessionFlow64Rate(ctx, strings.TrimSpace(dev.IP), comm, profile, idx, 1500*time.Millisecond)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "SNMP", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, rate)
}

func (s *Server) bngDeviceAuthRecords(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_QUERY", "id inválido", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	dev, comm, err := s.resolveBngDevice(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento BNG não encontrado", nil)
		return
	}
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "community SNMP não configurada", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	profile := bngcollect.LoadGlobalProfile(r.Context(), s.DB())
	records := bngcollect.FetchRecentBngAuthRecordsCached(ctx, id, strings.TrimSpace(dev.IP), comm, limit, profile.Options.PPPoELoginStripSuffix)
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":  id,
		"count":      len(records),
		"records":    records,
		"fetched_at": time.Now().UTC().Format(time.RFC3339),
		"note":       "Log de autenticação em tempo real via SNMP (falhas AAA + novos logins online). Equivalente aproximado ao log RADIUS.",
	})
}
