package monitorworker

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxMonitorDevicesPerSweep é um teto de segurança (não um limite operacional): protege
// contra um cenário patológico (ex.: filtro corrompido a devolver a tabela inteira sem
// WHERE), mas não deve nunca ser atingido por um inventário real de ISP. Antes desta
// constante, estas queries tinham LIMIT 500/100 fixos que truncavam silenciosamente o
// ciclo de monitorização — equipamentos além do limite simplesmente deixavam de ser
// sondados, sem erro nem alerta. Ver DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md (achado #1).
const maxMonitorDevicesPerSweep = 20000

// pingableDeviceRow equipamento elegível para sondagem (mesmos filtros do worker legado).
type pingableDeviceRow struct {
	id                uuid.UUID
	ip                string
	devComm           *string
	description       string
	telemetryEnabled  bool
	bngEnabled        bool
	bgpEnabled        bool
	category          string
	brand             string
	model             string
	maxPons           *int
	offlineAlertLogic string
}

func loadPingableDevices(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(d.ip)::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.bng_enabled, false), coalesce(d.bgp_enabled, false),
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, ''), d.max_pons,
			coalesce(d.offline_alert_logic, 'any')
		FROM devices d
		WHERE d.ping_enabled AND d.ip IS NOT NULL AND trim(host(d.ip)::text) <> ''
		  AND trim(both from coalesce(d.network_status, '')) = 'Normal'
		  AND trim(both from coalesce(d.operational_mode, '')) = 'Ativo'
	`
	return scanPingableDevices(ctx, pool, base, only, "equipamento não encontrado ou inelegível para monitorização")
}

// loadTelemetryDevices lista equipamentos com telemetria activa (KPIs SNMP no ciclo paralelo).
// Não depende só de ping_enabled: telemetria ligada + IP + Normal/Ativo. Usa o IP marcado
// for_telemetry em device_ips quando existir (equipamento com múltiplos IPs) — senão cai no
// IP primário (d.ip), como sempre.
func loadTelemetryDevices(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(COALESCE(dip.ip, d.ip))::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.bng_enabled, false), coalesce(d.bgp_enabled, false),
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, ''), d.max_pons,
			coalesce(d.offline_alert_logic, 'any')
		FROM devices d
		LEFT JOIN device_ips dip ON dip.device_id = d.id AND dip.for_telemetry = true
		WHERE d.telemetry_enabled = true
		  AND COALESCE(dip.ip, d.ip) IS NOT NULL AND trim(host(COALESCE(dip.ip, d.ip))::text) <> ''
		  AND trim(both from coalesce(d.network_status, '')) = 'Normal'
		  AND trim(both from coalesce(d.operational_mode, '')) = 'Ativo'
	`
	return scanPingableDevices(ctx, pool, base, only, "equipamento não encontrado ou inelegível para telemetria")
}

func scanPingableDevices(ctx context.Context, pool *pgxpool.Pool, base string, only *uuid.UUID, notFoundMsg string) ([]pingableDeviceRow, error) {
	args := []any{}
	if only != nil {
		base += ` AND d.id = $1`
		args = append(args, *only)
	}
	base += fmt.Sprintf(" ORDER BY d.description LIMIT %d", maxMonitorDevicesPerSweep)
	rows, err := pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pingableDeviceRow
	for rows.Next() {
		var r pingableDeviceRow
		if err := rows.Scan(&r.id, &r.ip, &r.devComm, &r.description, &r.telemetryEnabled, &r.bngEnabled, &r.bgpEnabled, &r.category, &r.brand, &r.model, &r.maxPons, &r.offlineAlertLogic); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if only != nil && len(out) == 0 {
		return nil, fmt.Errorf("%s", notFoundMsg)
	}
	return out, nil
}

// loadOltDevicesForCollect lista OLTs com IP para coleta SNMP periódica de ONUs/PON.
// Mesmos critérios da listagem OLT na API (sem exigir ping, network_status nem operational_mode).
func loadOltDevicesForCollect(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(d.ip)::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.bng_enabled, false), coalesce(d.bgp_enabled, false),
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, ''), d.max_pons
		FROM devices d
		WHERE lower(trim(coalesce(d.category, ''))) = 'olt'
		  AND d.ip IS NOT NULL AND trim(host(d.ip)::text) <> ''
	`
	args := []any{}
	if only != nil {
		base += ` AND d.id = $1`
		args = append(args, *only)
	}
	base += fmt.Sprintf(" ORDER BY d.description LIMIT %d", maxMonitorDevicesPerSweep)
	rows, err := pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pingableDeviceRow
	for rows.Next() {
		var r pingableDeviceRow
		if err := rows.Scan(&r.id, &r.ip, &r.devComm, &r.description, &r.telemetryEnabled, &r.bngEnabled, &r.bgpEnabled, &r.category, &r.brand, &r.model, &r.maxPons); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if only != nil && len(out) == 0 {
		return nil, fmt.Errorf("OLT não encontrada ou inelegível para coleta periódica")
	}
	return out, nil
}

// loadBngDevicesForCollect lista BNGs com coleta SNMP activa (bng_enabled). Usa o IP marcado
// for_bng em device_ips quando existir (equipamento com múltiplos IPs, ex.: um Huawei com um
// virtual-system de BGP noutro IP) — senão cai no IP primário (d.ip), como sempre.
func loadBngDevicesForCollect(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(COALESCE(dip.ip, d.ip))::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.bng_enabled, false), coalesce(d.bgp_enabled, false),
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, ''), d.max_pons
		FROM devices d
		LEFT JOIN device_ips dip ON dip.device_id = d.id AND dip.for_bng = true
		WHERE coalesce(d.bng_enabled, false) = true
		  AND COALESCE(dip.ip, d.ip) IS NOT NULL AND trim(host(COALESCE(dip.ip, d.ip))::text) <> ''
		  AND trim(both from coalesce(d.network_status, '')) = 'Normal'
		  AND trim(both from coalesce(d.operational_mode, '')) = 'Ativo'
	`
	args := []any{}
	if only != nil {
		base += ` AND d.id = $1`
		args = append(args, *only)
	}
	base += fmt.Sprintf(" ORDER BY d.description LIMIT %d", maxMonitorDevicesPerSweep)
	rows, err := pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pingableDeviceRow
	for rows.Next() {
		var r pingableDeviceRow
		if err := rows.Scan(&r.id, &r.ip, &r.devComm, &r.description, &r.telemetryEnabled, &r.bngEnabled, &r.bgpEnabled, &r.category, &r.brand, &r.model, &r.maxPons); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if only != nil && len(out) == 0 {
		return nil, fmt.Errorf("BNG não encontrado ou inelegível para coleta periódica")
	}
	return out, nil
}

// loadBgpDevicesForCollect lista equipamentos com coleta BGP activa (bgp_enabled) — mesmo
// padrão de loadBngDevicesForCollect, usando o IP marcado for_bgp em device_ips quando existir.
func loadBgpDevicesForCollect(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(COALESCE(dip.ip, d.ip))::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.bng_enabled, false), coalesce(d.bgp_enabled, false),
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, ''), d.max_pons
		FROM devices d
		LEFT JOIN device_ips dip ON dip.device_id = d.id AND dip.for_bgp = true
		WHERE coalesce(d.bgp_enabled, false) = true
		  AND COALESCE(dip.ip, d.ip) IS NOT NULL AND trim(host(COALESCE(dip.ip, d.ip))::text) <> ''
		  AND trim(both from coalesce(d.network_status, '')) = 'Normal'
		  AND trim(both from coalesce(d.operational_mode, '')) = 'Ativo'
	`
	args := []any{}
	if only != nil {
		base += ` AND d.id = $1`
		args = append(args, *only)
	}
	base += fmt.Sprintf(" ORDER BY d.description LIMIT %d", maxMonitorDevicesPerSweep)
	rows, err := pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pingableDeviceRow
	for rows.Next() {
		var r pingableDeviceRow
		if err := rows.Scan(&r.id, &r.ip, &r.devComm, &r.description, &r.telemetryEnabled, &r.bngEnabled, &r.bgpEnabled, &r.category, &r.brand, &r.model, &r.maxPons); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if only != nil && len(out) == 0 {
		return nil, fmt.Errorf("equipamento BGP não encontrado ou inelegível para coleta periódica")
	}
	return out, nil
}

func resolveSNMPCommunity(row pingableDeviceRow, defCommunity *string) string {
	if row.devComm != nil && strings.TrimSpace(*row.devComm) != "" {
		return strings.TrimSpace(*row.devComm)
	}
	if defCommunity != nil && strings.TrimSpace(*defCommunity) != "" {
		return strings.TrimSpace(*defCommunity)
	}
	return ""
}
