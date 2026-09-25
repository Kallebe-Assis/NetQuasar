package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorview"
)


func anyString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}




type metricsProfile struct {
	CPUPrimaryOID   string `json:"cpu_primary_oid"`
	CPUAvailableOID string `json:"cpu_available_oid"`
	MemoryUsedOID   string `json:"memory_used_oid"`
	MemorySizeOID   string `json:"memory_size_oid"`
	TempPrimaryOID  string `json:"temp_primary_oid"`
	UptimeOID       string `json:"uptime_oid"`
}











// GET /monitoring/active-equipment
// Lista equipamentos com rede Normal (não Bridge), ping ligado e operação Ativo; inclui última sondagem + última telemetria para campos SNMP conhecidos.
func (s *Server) monitoringActiveEquipment(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, d.category::text,
			COALESCE(NULLIF(TRIM(BOTH FROM d.brand), ''), '')::text,
			host(d.ip)::text,
			c.checked_at, c.latency_ms, c.ok, c.reach_ok, COALESCE(c.ping_fail_streak, 0),
			c.detail::text
		FROM devices d
		LEFT JOIN device_probe_cache c ON c.device_id = d.id
		WHERE TRIM(BOTH FROM COALESCE(d.network_status, '')) = 'Normal'
			AND COALESCE(TRIM(BOTH FROM d.network_status), '') <> ''
			AND d.ping_enabled = true
			AND TRIM(BOTH FROM COALESCE(d.operational_mode, '')) = 'Ativo'
			AND d.ip IS NOT NULL
			AND TRIM(BOTH FROM host(d.ip)::text) <> ''
		ORDER BY d.description
		LIMIT 600
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, cat, brand, ip string
		var ca any
		var lat *int64
		var probeOK *bool
		var reachOK *bool
		var pingFailStreak int
		var detail *string

		if err := rows.Scan(&id, &desc, &cat, &brand, &ip, &ca, &lat, &probeOK, &reachOK, &pingFailStreak, &detail); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}

		var detB []byte
		if detail != nil {
			detB = []byte(*detail)
		}
		kpis, hasKpis := monitorview.KPIsFromProbeDetail(detB)
		var cpu, mem, temp *float64
		uptime := "—"
		if hasKpis {
			cpu = kpis.CPUPercent
			mem = kpis.MemoryPercent
			temp = kpis.TemperatureC
			if strings.TrimSpace(kpis.Uptime) != "" {
				uptime = kpis.Uptime
			}
		}

		row := map[string]any{
			"id":               id,
			"description":      desc,
			"category":         cat,
			"brand":            brand,
			"ip":               ip,
			"checked_at":       ca,
			"latency_ms":       nil,
			"probe_ok":         probeOK,
			"ping_fail_streak": pingFailStreak,
			"cpu_percent":      cpu,
			"memory_percent":   mem,
			"uptime":           uptime,
			"temperature_c":    temp,
			"ping_reachable":   nil,
		}
		if reachOK != nil {
			row["ping_reachable"] = *reachOK
		}
		if lat != nil {
			row["latency_ms"] = *lat
		}
		if hasKpis && strings.TrimSpace(kpis.CollectedAt) != "" {
			row["telemetry_collected_at"] = kpis.CollectedAt
		}
		list = append(list, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}
