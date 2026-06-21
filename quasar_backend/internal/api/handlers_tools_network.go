package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

func (s *Server) toolsTracert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host            string `json:"host"`
		MaxHops         int    `json:"max_hops"`
		TimeoutPerHopMs int    `json:"timeout_per_hop_ms"`
		TimeoutMs       int    `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	total := probing.DefaultToolTimeout(body.TimeoutMs)
	if body.TimeoutPerHopMs <= 0 {
		body.TimeoutPerHopMs = 3000
	}
	ctx, cancel := context.WithTimeout(r.Context(), total)
	defer cancel()

	cmd, output, hops, err := probing.RunTracert(ctx, body.Host, body.MaxHops, body.TimeoutPerHopMs)
	resp := map[string]any{
		"host":    body.Host,
		"command": cmd,
		"output":  output,
		"hops":    hops,
		"ok":      err == nil,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	s.auditNetworkTool(r.Context(), r, "tracert", map[string]any{"host": body.Host, "ok": err == nil, "hops": len(hops)})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) toolsNmap(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string `json:"host"`
		ScanMode  string `json:"scan_mode"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), probing.DefaultToolTimeout(body.TimeoutMs))
	defer cancel()

	cmd, output, err := probing.RunNmap(ctx, body.Host, body.ScanMode)
	resp := map[string]any{
		"host":    body.Host,
		"command": cmd,
		"output":  output,
		"ok":      err == nil,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	s.auditNetworkTool(r.Context(), r, "nmap", map[string]any{"host": body.Host, "scan_mode": body.ScanMode, "ok": err == nil})
	writeJSON(w, http.StatusOK, resp)
}
