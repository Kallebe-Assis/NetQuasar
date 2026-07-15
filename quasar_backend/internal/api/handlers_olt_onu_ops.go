package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

type oltOnuActionRequest struct {
	Pon     int    `json:"pon"`
	Onu     int    `json:"onu"`
	Serial  string `json:"serial"`
	IfIndex int    `json:"if_index"`
	IfName  string `json:"if_name"`
	OnuType string `json:"onu_type"`
}

func (s *Server) listOLTUnauthorizedOnus(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 3*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
		return
	}
	cmdTpl := strings.TrimSpace(sess.Cfg.UnauthorizedOnuQueryCommand)
	if cmdTpl == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure o comando de consulta de ONUs não autorizadas em Definições → Perfis OLT para "+sess.Brand+" / "+sess.Model, nil)
		return
	}
	if sess.Cfg.NeedsEnablePasswordForUnauthorized() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}
	ponIndexes := oltcollect.LoadOLTPonIndexesFromSnapshot(ctx, s.DB(), id)
	telCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	action := oltcollect.RunUnauthorizedOnuQueryMulti(telCtx, sess.Host, sess.User, sess.Password, sess.Enable, sess.Cfg, secrets, ponIndexes, 3*time.Minute)

	entries := oltcollect.ParseOnuListFromTelnetOutput(action.Output)
	matches := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		matches = append(matches, oltcollect.SerialSearchEntryToMap(e))
	}

	actor := s.actorFromRequest(r)
	s.appendAuditLog(ctx, "olt_device", id.String(), "unauthorized_onu_query", actor, nil, map[string]any{
		"olt_description": sess.Desc, "command": action.Command, "ok": action.OK, "matches": len(matches),
		"pons_queried": action.PonsQueried,
	})

	out := map[string]any{
		"ok":              action.OK,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"command":         action.Command,
		"output":          action.Output,
		"entries":         matches,
		"total":           len(matches),
		"pons_queried":    action.PonsQueried,
		"pon_descriptions": s.devicePonDescriptions(ctx, id),
		"pon_vlans":        s.devicePonVlans(ctx, id),
	}
	if len(action.Steps) > 0 {
		steps := make([]map[string]any, 0, len(action.Steps))
		for _, st := range action.Steps {
			steps = append(steps, map[string]any{
				"command": st.Command,
				"ok":      st.OK,
				"output":  st.Output,
				"error":   st.Error,
			})
		}
		out["steps"] = steps
	}
	if action.Error != "" {
		out["error"] = action.Error
	}
	writeJSON(w, http.StatusOK, out)
}

// previewAuthorizeOLTOnu resolve o próximo ID livre e a VLAN sem executar o comando de autorização.
func (s *Server) previewAuthorizeOLTOnu(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 2*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body oltOnuActionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Pon <= 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "PON obrigatória para pré-visualizar autorização", nil)
		return
	}
	serial := strings.TrimSpace(body.Serial)
	if serial == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "Serial obrigatório para pré-visualizar autorização", nil)
		return
	}

	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
		return
	}
	if strings.TrimSpace(sess.Cfg.OnuAuthorizeCommand) == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure o comando telnet de autorização de ONU no perfil OLT ("+sess.Brand+" / "+sess.Model+")", nil)
		return
	}
	if sess.Cfg.NeedsEnablePassword() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}
	allocatedOnu := body.Onu <= 0
	listCmd := ""
	if allocatedOnu {
		telAlloc, cancelAlloc := context.WithTimeout(ctx, 90*time.Second)
		next, cmd, allocErr := oltcollect.ResolveNextAvailableOnuID(
			telAlloc, s.DB(), id, sess.Host, sess.User, sess.Password, sess.Enable, sess.Brand,
			sess.Cfg, secrets, body.Pon, 90*time.Second,
		)
		cancelAlloc()
		if allocErr != nil {
			writeErr(w, http.StatusInternalServerError, "TELNET",
				"Não foi possível obter o próximo ID de ONU na PON: "+allocErr.Error(), nil)
			return
		}
		body.Onu = next
		listCmd = cmd
	}

	target := oltcollect.OnuReportTarget{
		Pon: body.Pon, Onu: body.Onu, Serial: serial,
		IfIndex: body.IfIndex, IfName: body.IfName,
		OnuType: strings.TrimSpace(body.OnuType),
	}
	// Sempre montar gpon_onu-1/1/{pon}:{onu} a partir do ID alocado (nunca gpon_olt da lista uncfg).
	target.GponOnu = ""
	target.GponOnu = oltcollect.ResolveGponOnu(target)
	vlan, vlanSource := s.resolveAuthorizeVlan(ctx, id, sess, body.Pon)
	target.Vlan = vlan
	target = oltcollect.ApplyAuthorizeTemplateDefaults(target, sess.Cfg)
	if target.Vlan == "" {
		writeErr(w, http.StatusBadRequest, "VLAN_MISSING",
			"VLAN não encontrada para a PON "+strconv.Itoa(body.Pon)+". Configure o mapa VLAN↔PON no perfil OLT (descoberta SNMP).", nil)
		return
	}

	out := map[string]any{
		"ok":              true,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"pon":             body.Pon,
		"onu":             body.Onu,
		"serial":          serial,
		"gpon_onu":        target.GponOnu,
		"vlan":            target.Vlan,
		"vlan_source":     vlanSource,
		"allocated_onu":   allocatedOnu,
		"onu_type":        target.OnuType,
		"name":            target.Name,
		"pon_description": s.devicePonDescription(ctx, id, body.Pon),
	}
	if listCmd != "" {
		out["list_command"] = listCmd
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) resolveAuthorizeVlan(ctx context.Context, id uuid.UUID, sess oltTelnetSession, pon int) (vlan string, sourceID string) {
	sourceID = ""
	if pon <= 0 {
		return "", sourceID
	}
	if v := s.devicePonVlan(ctx, id, pon); v != "" {
		return v, "device_pon_vlan"
	}
	if v, ok := oltcollect.ResolveAuthorizeVlanForPon(sess.Cfg, pon); ok {
		return v, "profile_vlan_catalog"
	}
	if strings.Contains(strings.ToUpper(sess.Brand), "ZTE") {
		comm := s.oltSNMPCommunity(ctx, id)
		if comm != "" {
			snmpCtx, snmpCancel := context.WithTimeout(ctx, 25*time.Second)
			v, src, _ := oltcollect.LookupZtePonVlanViaSNMP(
				snmpCtx, sess.Host, comm, pon,
				oltcollect.EffectiveAuthorizeVlanSnmpOID(sess.Cfg), 25*time.Second,
			)
			snmpCancel()
			if v != "" {
				return v, src
			}
		}
	}
	return "", sourceID
}

func (s *Server) devicePonDescriptions(ctx context.Context, id uuid.UUID) json.RawMessage {
	var raw []byte
	err := s.DB().QueryRow(ctx, `SELECT COALESCE(pon_descriptions::text, '{}') FROM devices WHERE id=$1`, id).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return normalizePonDescriptionsJSON(raw)
}

func (s *Server) devicePonVlans(ctx context.Context, id uuid.UUID) json.RawMessage {
	var raw []byte
	err := s.DB().QueryRow(ctx, `SELECT COALESCE(pon_vlans::text, '{}') FROM devices WHERE id=$1`, id).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return normalizePonVlansJSON(raw)
}

func (s *Server) devicePonVlan(ctx context.Context, id uuid.UUID, pon int) string {
	if pon <= 0 {
		return ""
	}
	raw := s.devicePonVlans(ctx, id)
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	vid, ok := parsePonVlanValue(m[strconv.Itoa(pon)])
	if !ok {
		return ""
	}
	return strconv.Itoa(vid)
}

func (s *Server) devicePonDescription(ctx context.Context, id uuid.UUID, pon int) string {
	if pon <= 0 {
		return ""
	}
	raw := s.devicePonDescriptions(ctx, id)
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return strings.TrimSpace(m[strconv.Itoa(pon)])
}

func (s *Server) authorizeOLTOnu(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 5*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body oltOnuActionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Pon <= 0 && strings.TrimSpace(body.Serial) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "informe pon e serial", nil)
		return
	}
	if body.Pon <= 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "PON obrigatória para autorizar ONU", nil)
		return
	}

	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
		return
	}
	cmdTpl := strings.TrimSpace(sess.Cfg.OnuAuthorizeCommand)
	if cmdTpl == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure o comando telnet de autorização de ONU no perfil OLT ("+sess.Brand+" / "+sess.Model+")", nil)
		return
	}
	if sess.Cfg.NeedsEnablePassword() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}
	allocatedOnu := false
	listCmd := ""
	if body.Onu <= 0 {
		telAlloc, cancelAlloc := context.WithTimeout(ctx, 90*time.Second)
		next, cmd, allocErr := oltcollect.ResolveNextAvailableOnuID(
			telAlloc, s.DB(), id, sess.Host, sess.User, sess.Password, sess.Enable, sess.Brand,
			sess.Cfg, secrets, body.Pon, 90*time.Second,
		)
		cancelAlloc()
		if allocErr != nil {
			writeErr(w, http.StatusInternalServerError, "TELNET",
				"Não foi possível obter o próximo ID de ONU na PON: "+allocErr.Error(), nil)
			return
		}
		body.Onu = next
		allocatedOnu = true
		listCmd = cmd
	}

	target := oltcollect.OnuReportTarget{
		Pon: body.Pon, Onu: body.Onu, Serial: body.Serial,
		IfIndex: body.IfIndex, IfName: body.IfName,
		OnuType: strings.TrimSpace(body.OnuType),
	}
	target.GponOnu = ""
	target.GponOnu = oltcollect.ResolveGponOnu(target)

	vlan, vlanSource := s.resolveAuthorizeVlan(ctx, id, sess, body.Pon)
	target.Vlan = vlan
	target = oltcollect.ApplyAuthorizeTemplateDefaults(target, sess.Cfg)
	if strings.TrimSpace(target.Vlan) == "" {
		writeErr(w, http.StatusBadRequest, "VLAN_MISSING",
			"VLAN não encontrada para a PON "+strconv.Itoa(body.Pon)+". Configure o mapa VLAN↔PON no perfil OLT (descoberta SNMP).", nil)
		return
	}

	telCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	s.Log.Info().
		Str("olt", sess.Desc).
		Str("host", sess.Host).
		Int("pon", body.Pon).
		Int("onu", body.Onu).
		Str("serial", strings.TrimSpace(body.Serial)).
		Str("vlan", target.Vlan).
		Str("onu_type", target.OnuType).
		Str("name", target.Name).
		Msg("[TEMP] a iniciar provisionamento ONU via telnet")
	action := oltcollect.RunOnuTelnetAction(telCtx, sess.Host, sess.User, sess.Password, sess.Enable, sess.Cfg, secrets, target, cmdTpl, 5*time.Minute)
	s.Log.Info().
		Bool("ok", action.OK).
		Str("error", action.Error).
		Strs("commands", action.Commands).
		Msg("[TEMP] fim provisionamento ONU via telnet")

	actor := s.actorFromRequest(r)
	s.appendAuditLog(ctx, "olt_device", id.String(), "onu_authorize", actor, nil, map[string]any{
		"olt_description": sess.Desc,
		"pon":             body.Pon, "onu": body.Onu, "serial": strings.TrimSpace(body.Serial),
		"vlan": target.Vlan, "vlan_source": vlanSource,
		"allocated_onu": allocatedOnu, "list_command": listCmd,
		"command": action.Command, "commands": action.Commands, "ok": action.OK,
	})

	out := map[string]any{
		"ok":              action.OK,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"command":         action.Command,
		"commands":        action.Commands,
		"output":          action.Output,
		"pon":             body.Pon,
		"onu":             body.Onu,
		"serial":          strings.TrimSpace(body.Serial),
		"vlan":            target.Vlan,
		"vlan_source":     vlanSource,
		"allocated_onu":   allocatedOnu,
	}
	if listCmd != "" {
		out["list_command"] = listCmd
	}
	if action.Error != "" {
		out["error"] = action.Error
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deauthorizeOLTOnu(w http.ResponseWriter, r *http.Request) {
	s.runOLTOnuMutation(w, r, "onu_deauthorize", func(cfg oltcollect.OnuReportConfig) string {
		return cfg.OnuDeauthorizeCommand
	}, "Configure o comando telnet de desautorização de ONU no perfil OLT")
}

func (s *Server) runOLTOnuMutation(
	w http.ResponseWriter,
	r *http.Request,
	auditAction string,
	commandFn func(oltcollect.OnuReportConfig) string,
	notConfiguredMsg string,
) {
	extendWriteDeadline(w, 2*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body oltOnuActionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Pon <= 0 && body.Onu <= 0 && strings.TrimSpace(body.Serial) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "informe pon/onu ou serial", nil)
		return
	}

	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
		return
	}
	cmdTpl := strings.TrimSpace(commandFn(sess.Cfg))
	if cmdTpl == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", notConfiguredMsg+" ("+sess.Brand+" / "+sess.Model+")", nil)
		return
	}
	if sess.Cfg.NeedsEnablePassword() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}
	target := oltcollect.OnuReportTarget{
		Pon: body.Pon, Onu: body.Onu, Serial: body.Serial,
		IfIndex: body.IfIndex, IfName: body.IfName,
	}
	target.GponOnu = oltcollect.ResolveGponOnu(target)
	if target.GponOnu == "" && sess.Cfg.NeedsGponOnu() && strings.TrimSpace(body.Serial) != "" {
		if g := s.lookupGponOnuBySerial(ctx, sess, target, secrets); g != "" {
			target.GponOnu = g
		}
	}

	telCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	action := oltcollect.RunOnuTelnetAction(telCtx, sess.Host, sess.User, sess.Password, sess.Enable, sess.Cfg, secrets, target, cmdTpl, 2*time.Minute)

	actor := s.actorFromRequest(r)
	s.appendAuditLog(ctx, "olt_device", id.String(), auditAction, actor, nil, map[string]any{
		"olt_description": sess.Desc,
		"pon":             body.Pon, "onu": body.Onu, "serial": strings.TrimSpace(body.Serial),
		"command": action.Command, "ok": action.OK,
	})

	out := map[string]any{
		"ok":              action.OK,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"command":         action.Command,
		"output":          action.Output,
		"pon":             body.Pon,
		"onu":             body.Onu,
		"serial":          strings.TrimSpace(body.Serial),
	}
	if action.Error != "" {
		out["error"] = action.Error
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) oltSNMPCommunity(ctx context.Context, deviceID uuid.UUID) string {
	var devComm *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM devices WHERE id=$1`, deviceID).Scan(&devComm)
	if devComm != nil {
		if c := strings.TrimSpace(*devComm); c != "" {
			return c
		}
	}
	var defComm *string
	_ = s.DB().QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&defComm)
	if defComm != nil {
		return strings.TrimSpace(*defComm)
	}
	return ""
}

func (s *Server) discoverOLTVlanCatalog(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 90*time.Second)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body struct {
		OID string `json:"oid"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	ctx := r.Context()
	var host, brand, model string
	var ip *string
	err = s.DB().QueryRow(ctx, `
		SELECT host(ip)::text, coalesce(brand,''), coalesce(model,'')
		FROM devices WHERE id=$1 AND lower(trim(category))='olt'
	`, id).Scan(&ip, &brand, &model)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	if host == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "OLT sem IP", nil)
		return
	}
	comm := s.oltSNMPCommunity(ctx, id)
	if comm == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", "snmp_community não configurada", nil)
		return
	}

	oid := strings.TrimSpace(body.OID)
	if oid == "" {
		var reportRaw []byte
		_ = s.DB().QueryRow(ctx, `
			SELECT coalesce(onu_report_commands::text, '{}')
			FROM olt_vendor_models
			WHERE upper(trim(brand)) = upper(trim($1)) AND upper(trim(model)) = upper(trim($2))
		`, brand, model).Scan(&reportRaw)
		cfg := oltcollect.ParseOnuReportConfig(reportRaw)
		oid = oltcollect.EffectiveAuthorizeVlanSnmpOID(cfg)
	}

	entries, usedOID, walkErr := oltcollect.DiscoverZteVlanCatalogViaSNMP(ctx, host, comm, oid, 45*time.Second)
	if walkErr != "" && len(entries) == 0 {
		writeErr(w, http.StatusBadGateway, "SNMP", walkErr, nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"olt_id":  id,
		"host":    host,
		"oid":     usedOID,
		"total":   len(entries),
		"entries": entries,
		"error":   nilIfBlankStr(walkErr),
	})
}
