package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/telemetryengine"
)

// listMikrotikDevices — visão geral de todos os equipamentos MikroTik (mesmo padrão de
// listOLTDevices em handlers_olt_snmp.go): uma linha resumida por equipamento, para a tabela de
// overview do MikroTik (antes só existia a visão individual, sempre no primeiro equipamento).
func (s *Server) listMikrotikDevices(w http.ResponseWriter, r *http.Request) {
	// Localidade do MikroTik vem do POP a que está vinculado (devices.pop_id -> pops.locality_id),
	// não de devices.locality_id directamente — pedido explícito, diferente do padrão usado para
	// as OLTs (listOLTDevices), que lêem locality_id directo do equipamento.
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, host(d.ip)::text, d.brand, d.model, p.locality_id, l.name,
			c.ok, c.checked_at, c.snmp_health_status, c.snmp_health_reason, c.snmp_health_checked_at
		FROM devices d
		LEFT JOIN pops p ON p.id = d.pop_id
		LEFT JOIN commercial_localities l ON l.id = p.locality_id
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE lower(trim(d.category)) LIKE '%mikrotik%' OR lower(coalesce(d.brand, '')) LIKE '%mikrotik%'
		ORDER BY d.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	type row struct {
		id                   uuid.UUID
		desc, ip, brand, mdl *string
		locID                *uuid.UUID
		locName              *string
		ok                   *bool
		checkedAt            *time.Time
		hStatus, hReason     *string
		hAt                  *time.Time
	}
	var list []row
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.id, &it.desc, &it.ip, &it.brand, &it.mdl, &it.locID, &it.locName,
			&it.ok, &it.checkedAt, &it.hStatus, &it.hReason, &it.hAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	rows.Close()

	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		item := map[string]any{
			"id": it.id, "description": it.desc, "ip": it.ip, "brand": it.brand, "model": it.mdl,
			"locality_id": it.locID, "locality_name": it.locName,
			"online": it.ok,
		}
		if it.checkedAt != nil {
			item["checked_at"] = it.checkedAt.UTC().Format(time.RFC3339)
		}
		item["snmp_health_status"] = it.hStatus
		if it.hReason != nil {
			item["snmp_health_reason"] = *it.hReason
		}
		if it.hAt != nil {
			item["snmp_health_checked_at"] = it.hAt.UTC().Format(time.RFC3339)
		}

		// Devolve o metrics cru (não campos soltos): a extracção de CPU/memória/uptime tem lógica
		// de fallback SNMP↔telnet já implementada e testada no frontend (buildMikrotikNocKpis em
		// mikrotikNocData.ts, a mesma usada pela tela individual) — duplicar isso em Go arriscava
		// divergir da lógica real (ex.: campos aninhados em metrics.mikrotik_collection.fields.*,
		// não soltos como metrics.cpu_load).
		collectedAt, metrics, _, terr := telemetryengine.LoadLatestMergedTelemetry(r.Context(), s.DB(), it.id)
		if terr == nil {
			item["telemetry_collected_at"] = collectedAt.UTC().Format(time.RFC3339)
			item["metrics"] = metrics
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"mikrotiks": out})
}
