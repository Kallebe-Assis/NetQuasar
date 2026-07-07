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
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

type oltOnuActionRequest struct {
	Pon     int    `json:"pon"`
	Onu     int    `json:"onu"`
	Serial  string `json:"serial"`
	IfIndex int    `json:"if_index"`
	IfName  string `json:"if_name"`
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

func (s *Server) authorizeOLTOnu(w http.ResponseWriter, r *http.Request) {
	s.runOLTOnuMutation(w, r, "onu_authorize", func(cfg oltcollect.OnuReportConfig) string {
		return cfg.OnuAuthorizeCommand
	}, "Configure o comando telnet de autorização de ONU no perfil OLT")
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
