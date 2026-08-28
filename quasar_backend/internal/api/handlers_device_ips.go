package api

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handlers_device_ips.go — IPs extra de um equipamento (device_ips), editados junto do
// cadastro/edição do próprio equipamento (ver deviceDTO.ExtraIPs em handlers_devices.go).
// Substituição total a cada gravação (mesmo padrão de putDeviceInterfaceMetadata) — mais
// simples que CRUD incremental para uma lista pequena editada inteira de uma vez no formulário.

// validateDeviceExtraIPs: cada IP extra tem de ser válido, não pode repetir o IP primário nem
// outro IP extra, e — só quando o total (primário + extras) for 2 ou mais — precisa de
// descrição (pedido explícito: "se tiver um único IP comum não precisa de descrição").
func validateDeviceExtraIPs(primaryIP *string, extra []deviceIPDTO) error {
	if len(extra) == 0 {
		return nil
	}
	primary := ""
	if primaryIP != nil {
		primary = strings.TrimSpace(*primaryIP)
	}
	total := len(extra)
	if primary != "" {
		total++
	}
	seen := map[string]bool{}
	if primary != "" {
		seen[primary] = true
	}
	for i, ip := range extra {
		v := strings.TrimSpace(ip.IP)
		if v == "" {
			return fmt.Errorf("IP extra #%d: endereço obrigatório", i+1)
		}
		if net.ParseIP(v) == nil {
			// aceita também CIDR (ex.: só o endereço é esperado aqui, mas alguns campos
			// de IP no sistema já toleram host/prefixo — validar só o host quando houver "/").
			host, _, err := net.ParseCIDR(v)
			if err != nil || host == nil {
				return fmt.Errorf("IP extra #%d: %q não é um endereço IP válido", i+1, v)
			}
		}
		if seen[v] {
			return fmt.Errorf("IP extra #%d: %q repetido (já usado pelo equipamento)", i+1, v)
		}
		seen[v] = true
		if total >= 2 && strings.TrimSpace(ip.Description) == "" {
			return fmt.Errorf("IP extra #%d (%s): descrição obrigatória quando há 2 ou mais IPs", i+1, v)
		}
	}
	return nil
}

func loadDeviceExtraIPs(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]deviceIPDTO, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, host(ip)::text, COALESCE(description,''), monitored, for_telemetry, for_bng, for_bgp
		FROM device_ips WHERE device_id=$1 ORDER BY sort_order, created_at
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]deviceIPDTO, 0, 2)
	for rows.Next() {
		var d deviceIPDTO
		var id uuid.UUID
		if err := rows.Scan(&id, &d.IP, &d.Description, &d.Monitored, &d.ForTelemetry, &d.ForBng, &d.ForBgp); err != nil {
			return nil, err
		}
		d.ID = &id
		out = append(out, d)
	}
	return out, rows.Err()
}

// loadDeviceExtraIPsBulk anexa extra_ips a vários equipamentos de uma vez (usado por
// listDevices, que devolve a tela de Equipamentos inteira — evita N+1 queries).
func loadDeviceExtraIPsBulk(ctx context.Context, pool *pgxpool.Pool, deviceIDs []uuid.UUID) (map[uuid.UUID][]deviceIPDTO, error) {
	out := map[uuid.UUID][]deviceIPDTO{}
	if len(deviceIDs) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT device_id, id, host(ip)::text, COALESCE(description,''), monitored, for_telemetry, for_bng, for_bgp
		FROM device_ips WHERE device_id = ANY($1) ORDER BY device_id, sort_order, created_at
	`, deviceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID uuid.UUID
		var d deviceIPDTO
		var id uuid.UUID
		if err := rows.Scan(&deviceID, &id, &d.IP, &d.Description, &d.Monitored, &d.ForTelemetry, &d.ForBng, &d.ForBgp); err != nil {
			return nil, err
		}
		d.ID = &id
		out[deviceID] = append(out[deviceID], d)
	}
	return out, rows.Err()
}

// replaceDeviceExtraIPs substitui por inteiro os IPs extra do equipamento — apaga e reinsere
// numa transação, como putDeviceInterfaceMetadata já faz para metadata de interfaces.
func replaceDeviceExtraIPs(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ips []deviceIPDTO) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM device_ips WHERE device_id=$1`, deviceID); err != nil {
		return err
	}
	for i, ip := range ips {
		v := strings.TrimSpace(ip.IP)
		if v == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO device_ips (device_id, ip, description, monitored, for_telemetry, for_bng, for_bgp, sort_order)
			VALUES ($1, $2::inet, $3, $4, $5, $6, $7, $8)
		`, deviceID, v, strings.TrimSpace(ip.Description), ip.Monitored, ip.ForTelemetry, ip.ForBng, ip.ForBgp, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
