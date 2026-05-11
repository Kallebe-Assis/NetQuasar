package monitorworker

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pingableDeviceRow equipamento elegível para sondagem (mesmos filtros do worker legado).
type pingableDeviceRow struct {
	id               uuid.UUID
	ip               string
	devComm          *string
	description      string
	telemetryEnabled bool
	category         string
	brand            string
	model            string
}

func loadPingableDevices(ctx context.Context, pool *pgxpool.Pool, only *uuid.UUID) ([]pingableDeviceRow, error) {
	base := `
		SELECT d.id, host(d.ip)::text, d.snmp_community, d.description, d.telemetry_enabled,
			coalesce(d.category, ''), coalesce(d.brand, ''), coalesce(d.model, '')
		FROM devices d
		WHERE d.ping_enabled AND d.ip IS NOT NULL AND trim(host(d.ip)::text) <> ''
		  AND trim(both from coalesce(d.network_status, '')) <> 'Bridge'
	`
	args := []any{}
	if only != nil {
		base += ` AND d.id = $1`
		args = append(args, *only)
	}
	base += ` ORDER BY d.description LIMIT 500`
	rows, err := pool.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pingableDeviceRow
	for rows.Next() {
		var r pingableDeviceRow
		if err := rows.Scan(&r.id, &r.ip, &r.devComm, &r.description, &r.telemetryEnabled, &r.category, &r.brand, &r.model); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if only != nil && len(out) == 0 {
		return nil, fmt.Errorf("equipamento não encontrado ou inelegível para monitorização")
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
