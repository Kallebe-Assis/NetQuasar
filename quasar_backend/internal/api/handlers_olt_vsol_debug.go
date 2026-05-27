package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdevicelock"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// getOLTSnmpDebug devolve último snmp_debug do snapshot ou coleta ao vivo (?live=1, admin POST).
func (s *Server) getOLTSnmpDebug(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	live := strings.TrimSpace(r.URL.Query().Get("live")) == "1"
	if r.Method == http.MethodPost || live {
		s.collectOLTSnmpDebug(w, r, id)
		return
	}
	var sumRaw []byte
	err = s.DB().QueryRow(r.Context(), `SELECT summary::text FROM olt_snapshots WHERE device_id=$1`, id).Scan(&sumRaw)
	if err != nil || len(sumRaw) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"snmp_debug": nil, "message": "Sem snapshot OLT. Use POST ou ?live=1 para coletar."})
		return
	}
	var sum map[string]any
	if json.Unmarshal(sumRaw, &sum) != nil {
		writeErr(w, http.StatusInternalServerError, "JSON", "", nil)
		return
	}
	dbg, _ := sum["snmp_debug"].(map[string]any)
	writeJSON(w, http.StatusOK, map[string]any{"snmp_debug": dbg, "from_snapshot": true, "snapshot_summary_keys": keysOf(sum)})
}

func (s *Server) postOLTSnmpDebug(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	s.collectOLTSnmpDebug(w, r, id)
}

func (s *Server) collectOLTSnmpDebug(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	unlock := snmpdevicelock.Acquire(id)
	defer unlock()

	var ip *string
	var comm *string
	var brand, model string
	if err := s.DB().QueryRow(r.Context(), `
		SELECT host(d.ip)::text, d.snmp_community, lower(coalesce(trim(d.brand), '')), lower(coalesce(trim(d.model), ''))
		FROM devices d WHERE d.id=$1
	`, id).Scan(&ip, &comm, &brand, &model); err != nil {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	host, c := "", ""
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	if comm != nil {
		c = strings.TrimSpace(*comm)
	}
	if c == "" {
		var dc *string
		_ = s.DB().QueryRow(r.Context(), `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&dc)
		if dc != nil {
			c = strings.TrimSpace(*dc)
		}
	}
	m := strings.ToLower(model)
	isVsol := strings.Contains(brand, "vsol") || strings.Contains(model, "vsol") ||
		strings.Contains(m, "v1600") || strings.Contains(m, "1600g")
	if !isVsol || host == "" || c == "" {
		writeErr(w, http.StatusUnprocessableEntity, "NOT_VSOL", "Dispositivo não é OLT VSOL ou falta IP/community", nil)
		return
	}

	to := s.loadCollectionTimeouts(r.Context()).OltRefreshTotal()
	if to < 300*time.Second {
		to = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to)
	defer cancel()

	ifTO := to / 4
	if ifTO > 90*time.Second {
		ifTO = 90 * time.Second
	}

	var ifPons []map[string]any
	var onuRefs []vsolparse.OnuRef
	ifMeta := map[string]any{}
	ifCtx, ifCancel := context.WithTimeout(ctx, ifTO)
	if pons, meta, refs, ok := s.vsolPonRowsFromIfMIB(ifCtx, id, host, c, ifTO, false); ok {
		ifPons = pons
		ifMeta = meta
		onuRefs = refs
	}
	ifCancel()

	mibTO := vsolparse.CollectTimeout(len(onuRefs), false)
	mibCtx, mibCancel := context.WithTimeout(context.WithoutCancel(ctx), mibTO)
	defer mibCancel()
	coll := vsolparse.CollectOLT(mibCtx, host, c, onuRefs, false)

	onBy, offBy := vsolparse.OnlineOfflineByPon(coll.Vars)
	final := vsolparse.AttachOnlineOfflineToIfPons(ifPons, onBy, offBy)

	rep := vsolparse.BuildSnmpDebugReport(host, coll, ifMeta, ifPons, final)
	writeJSON(w, http.StatusOK, map[string]any{
		"snmp_debug":      vsolparse.DebugReportToMap(rep),
		"from_snapshot":   false,
		"live_collection": true,
		"host":            host,
	})
}

func keysOf(m map[string]any) []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
