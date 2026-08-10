package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/sysevents"
)

type mikrotikToolsBody struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Community string `json:"community"`
	Version   string `json:"version"`
	TimeoutMs int    `json:"timeout_ms"`
	Retries   int    `json:"retries"`
}

func (s *Server) resolveToolsSNMPCommunity(ctx context.Context, community string) string {
	community = strings.TrimSpace(community)
	if community != "" {
		return community
	}
	var def *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
	if def != nil {
		return strings.TrimSpace(*def)
	}
	return ""
}

func (s *Server) decodeMikrotikToolsBody(w http.ResponseWriter, r *http.Request) (mikrotikToolsBody, bool) {
	var body mikrotikToolsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return body, false
	}
	body.Host = strings.TrimSpace(body.Host)
	if body.Host == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "host (IP ou nome) obrigatório", nil)
		return body, false
	}
	body.Community = s.resolveToolsSNMPCommunity(r.Context(), body.Community)
	if body.Community == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "community não informada e sem padrão configurado", nil)
		return body, false
	}
	if body.Port <= 0 || body.Port > 65535 {
		body.Port = 161
	}
	if strings.TrimSpace(body.Version) == "" {
		body.Version = "2c"
	}
	if body.TimeoutMs <= 0 {
		body.TimeoutMs = 8000
	}
	if body.TimeoutMs > 120000 {
		body.TimeoutMs = 120000
	}
	return body, true
}

// toolsMikrotikQuickMetrics coleta escalares RouterOS (system/health) via SNMP.
func (s *Server) toolsMikrotikQuickMetrics(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeMikrotikToolsBody(w, r)
	if !ok {
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(r.Context(), to+2*time.Second)
	defer cancel()

	profile := mikrotikcollect.LoadGlobalProfile(ctx, s.DB())
	out := mikrotikcollect.CollectMetrics(ctx, body.Host, body.Community, profile, mikrotikcollect.CollectOpts{
		Timeout:     to,
		ScalarsOnly: true,
		Sections:    []string{"system", "health"},
	})

	s.auditNetworkTool(r.Context(), r, "mikrotik_quick_metrics", map[string]any{
		"host":      body.Host,
		"collected": out.Status.Collected,
		"failed":    out.Status.Failed,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"host":      body.Host,
		"port":      body.Port,
		"version":   body.Version,
		"fields":    out.Fields,
		"status":    out.Status,
		"note":      "Coleta RouterOS dedicada (escalares system/health via perfil MikroTik).",
	})
}

// toolsMikrotikInterfaces lista interfaces IF-MIB (nome + admin/oper status).
func (s *Server) toolsMikrotikInterfaces(w http.ResponseWriter, r *http.Request) {
	body, ok := s.decodeMikrotikToolsBody(w, r)
	if !ok {
		return
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(r.Context(), to+15*time.Second)
	defer cancel()

	const ifTable = "1.3.6.1.2.1.2.2.1"
	vars, truncated, walkNote := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host:      body.Host,
		Port:      uint16(body.Port),
		Community: body.Community,
		RootOID:   ifTable,
		Version:   body.Version,
		Timeout:   to,
		Retries:   body.Retries,
		MaxRows:   12000,
	})
	ifaces := mikrotikcollect.ParseIFMibInterfaces(vars)
	up, down := 0, 0
	for _, row := range ifaces {
		if row.OperStatus == 1 {
			up++
		} else if row.OperStatus == 2 {
			down++
		}
	}
	status := "ok"
	if walkNote != "" && len(ifaces) == 0 {
		status = "failed"
	}
	s.auditNetworkTool(r.Context(), r, "mikrotik_interfaces", map[string]any{
		"host":  body.Host,
		"count": len(ifaces),
		"ok":    status == "ok",
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"host":       body.Host,
		"port":       body.Port,
		"version":    body.Version,
		"status":     status,
		"root_oid":   ifTable,
		"truncated": truncated,
		"walk_note":  walkNote,
		"count":      len(ifaces),
		"up":         up,
		"down":       down,
		"interfaces": ifaces,
		"note":       "Interfaces MikroTik/IF-MIB (ifDescr + ifAdminStatus + ifOperStatus).",
	})
}

type deviceCheckRow struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	OK      *bool  `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Detail  any    `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
}

// deviceChecks executa a matriz unificada ICMP + SNMP + TCP/161 + Telnet/SSH (quando houver credenciais).
func (s *Server) deviceChecks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body struct {
		TimeoutMs int `json:"timeout_ms"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.TimeoutMs <= 0 {
		body.TimeoutMs = 5000
	}
	if body.TimeoutMs > 30000 {
		body.TimeoutMs = 30000
	}
	to := time.Duration(body.TimeoutMs) * time.Millisecond

	var (
		ipStr, accessMode                *string
		desc, category                   string
		snmpComm, telnetUser, telnetPass *string
		sshUser, sshPass                 *string
		pingEn, telemEn                  bool
	)
	err = s.DB().QueryRow(r.Context(), `
		SELECT host(ip)::text, description, category, access_mode,
			snmp_community, telnet_user, telnet_password, ssh_user, ssh_password,
			ping_enabled, telemetry_enabled
		FROM devices WHERE id=$1
	`, id).Scan(
		&ipStr, &desc, &category, &accessMode,
		&snmpComm, &telnetUser, &telnetPass, &sshUser, &sshPass,
		&pingEn, &telemEn,
	)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	host := ""
	if ipStr != nil {
		host = strings.TrimSpace(*ipStr)
	}
	community := ""
	if snmpComm != nil {
		community = strings.TrimSpace(*snmpComm)
	}
	community = s.resolveToolsSNMPCommunity(r.Context(), community)

	checks := make([]deviceCheckRow, 0, 5)
	boolPtr := func(v bool) *bool { return &v }

	// ICMP
	if host == "" {
		checks = append(checks, deviceCheckRow{ID: "icmp", Label: "ICMP ping", Skipped: true, Reason: "sem IP"})
	} else if !pingEn {
		checks = append(checks, deviceCheckRow{ID: "icmp", Label: "ICMP ping", Skipped: true, Reason: "ping_enabled=false"})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), to+200*time.Millisecond)
		out := probing.ICMPPing(ctx, host, to, 32)
		cancel()
		checks = append(checks, deviceCheckRow{
			ID: "icmp", Label: "ICMP ping", OK: boolPtr(out.OK), Detail: out, Error: out.Error,
		})
	}

	// TCP SNMP port
	if host == "" {
		checks = append(checks, deviceCheckRow{ID: "tcp_snmp", Label: "TCP :161 (SNMP)", Skipped: true, Reason: "sem IP"})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), to+200*time.Millisecond)
		okTCP, rtt, errTCP := probing.TCPProbe(ctx, host, "161", to)
		cancel()
		row := deviceCheckRow{ID: "tcp_snmp", Label: "TCP :161 (SNMP)", OK: boolPtr(okTCP), Detail: map[string]any{"rtt_ms": rtt}}
		if errTCP != nil {
			row.Error = errTCP.Error()
		}
		checks = append(checks, row)
	}

	// SNMP GET
	if host == "" {
		checks = append(checks, deviceCheckRow{ID: "snmp", Label: "SNMP get (sysDescr/sysUpTime)", Skipped: true, Reason: "sem IP"})
	} else if community == "" {
		checks = append(checks, deviceCheckRow{ID: "snmp", Label: "SNMP get (sysDescr/sysUpTime)", Skipped: true, Reason: "community em falta"})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
		res := probing.SNMPGet(ctx, probing.SNMPGetParams{
			Host: host, Port: 161, Community: community, Version: "2c", Timeout: to, Retries: 0,
			OIDs: []string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0"},
		})
		cancel()
		checks = append(checks, deviceCheckRow{
			ID: "snmp", Label: "SNMP get (sysDescr/sysUpTime/sysName)", OK: boolPtr(res.OK), Detail: res, Error: res.Error,
		})
	}

	tu := ""
	if telnetUser != nil {
		tu = strings.TrimSpace(*telnetUser)
	}
	tp := ""
	if telnetPass != nil {
		tp = *telnetPass
	}
	if tu == "" {
		var defUser, defPass *string
		_ = s.DB().QueryRow(r.Context(), `SELECT telnet_user, telnet_password FROM settings_connection_defaults WHERE id=1`).Scan(&defUser, &defPass)
		if defUser != nil {
			tu = strings.TrimSpace(*defUser)
		}
		if tp == "" && defPass != nil {
			tp = *defPass
		}
	}
	if host == "" || tu == "" {
		reason := "sem credenciais telnet"
		if host == "" {
			reason = "sem IP"
		}
		checks = append(checks, deviceCheckRow{ID: "telnet", Label: "Telnet login", Skipped: true, Reason: reason})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
		res := probing.TelnetProbe(ctx, probing.TelnetTestParams{
			Host: host, Port: "23", Timeout: to, User: tu, Password: tp, MaxReadBytes: 2048,
		})
		cancel()
		checks = append(checks, deviceCheckRow{
			ID: "telnet", Label: "Telnet login", OK: boolPtr(res.OK), Detail: map[string]any{
				"latency_ms": res.LatencyMs, "note": res.Note,
			}, Error: res.Error,
		})
	}

	su := ""
	if sshUser != nil {
		su = strings.TrimSpace(*sshUser)
	}
	sp := ""
	if sshPass != nil {
		sp = *sshPass
	}
	if host == "" || su == "" {
		reason := "sem credenciais SSH"
		if host == "" {
			reason = "sem IP"
		}
		checks = append(checks, deviceCheckRow{ID: "ssh", Label: "SSH login", Skipped: true, Reason: reason})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), to+time.Second)
		res := probing.SSHDialWithPassword(ctx, probing.SSHDialParams{
			Host: host, Port: "22", User: su, Password: sp, Timeout: to,
		})
		cancel()
		checks = append(checks, deviceCheckRow{
			ID: "ssh", Label: "SSH login", OK: boolPtr(res.OK), Detail: map[string]any{
				"latency_ms": res.LatencyMs, "note": res.Note,
			}, Error: res.Error,
		})
	}

	okN, failN, skipN := 0, 0, 0
	for _, c := range checks {
		if c.Skipped {
			skipN++
			continue
		}
		if c.OK != nil && *c.OK {
			okN++
		} else {
			failN++
		}
	}
	summary := map[string]any{
		"ok": failN == 0 && okN > 0, "passed": okN, "failed": failN, "skipped": skipN, "total": len(checks),
	}
	did := id
	_ = sysevents.Emit(r.Context(), s.DB(), sysevents.TypeDeviceChecks, "info", &did, map[string]any{
		"host": host, "summary": summary, "checks": checksBrief(checks),
	})
	s.appendAuditLog(r.Context(), "device", id.String(), "checks", s.actorFromRequest(r), nil, map[string]any{
		"summary": summary,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":          id,
		"host":               host,
		"description":        desc,
		"category":           category,
		"access_mode":        accessMode,
		"ping_enabled":       pingEn,
		"telemetry_enabled":  telemEn,
		"checks":             checks,
		"summary":            summary,
		"note":               "Matriz unificada de checks (ICMP, TCP SNMP, SNMP GET, Telnet, SSH).",
	})
}

func checksBrief(checks []deviceCheckRow) []map[string]any {
	out := make([]map[string]any, 0, len(checks))
	for _, c := range checks {
		row := map[string]any{"id": c.ID, "skipped": c.Skipped}
		if c.OK != nil {
			row["ok"] = *c.OK
		}
		if c.Reason != "" {
			row["reason"] = c.Reason
		}
		out = append(out, row)
	}
	return out
}
