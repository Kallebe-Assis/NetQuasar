package switchcollect

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

// TelnetProfile perfil telnet para Switch (Cisco NX-OS).
type TelnetProfile = mikrotikcollect.TelnetProfile

func LoadTelnetProfileByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (TelnetProfile, error) {
	var p TelnetProfile
	var metricsRaw, preRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM switch_telnet_profiles WHERE id=$1
	`, id).Scan(&p.ID, &p.Name, &metricsRaw, &preRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return TelnetProfile{}, err
	}
	p.Metrics = DefaultTelnetMetrics()
	if parsed := ParseTelnetMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = MergeTelnetMetrics(parsed)
	}
	p.PreCommands = mikrotikcollect.ParseTelnetPreCommands(preRaw)
	if len(p.PreCommands) == 0 {
		p.PreCommands = DefaultTelnetPreCommands()
	}
	return p, nil
}

func LoadDefaultTelnetProfile(ctx context.Context, pool *pgxpool.Pool) TelnetProfile {
	var p TelnetProfile
	var metricsRaw, preRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM switch_telnet_profiles
		WHERE is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&p.ID, &p.Name, &metricsRaw, &preRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return TelnetProfile{Name: "Cisco NX-OS", Metrics: DefaultTelnetMetrics(), PreCommands: DefaultTelnetPreCommands()}
	}
	p.Metrics = DefaultTelnetMetrics()
	if parsed := ParseTelnetMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = MergeTelnetMetrics(parsed)
	}
	p.PreCommands = mikrotikcollect.ParseTelnetPreCommands(preRaw)
	if len(p.PreCommands) == 0 {
		p.PreCommands = DefaultTelnetPreCommands()
	}
	return p
}

func LoadTelnetProfileForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) TelnetProfile {
	if pool == nil {
		return TelnetProfile{Name: "Cisco NX-OS", Metrics: DefaultTelnetMetrics(), PreCommands: DefaultTelnetPreCommands()}
	}
	var profileID *uuid.UUID
	_ = pool.QueryRow(ctx, `
		SELECT switch_telnet_profile_id FROM devices WHERE id=$1
	`, deviceID).Scan(&profileID)
	if profileID != nil {
		if p, err := LoadTelnetProfileByID(ctx, pool, *profileID); err == nil {
			return p
		}
	}
	return LoadDefaultTelnetProfile(ctx, pool)
}

func ListTelnetProfiles(ctx context.Context, pool *pgxpool.Pool) ([]TelnetProfile, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM switch_telnet_profiles
		ORDER BY is_default DESC, lower(trim(name))
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TelnetProfile
	for rows.Next() {
		var p TelnetProfile
		var metricsRaw, preRaw []byte
		if err := rows.Scan(&p.ID, &p.Name, &metricsRaw, &preRaw, &p.IsDefault, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Metrics = DefaultTelnetMetrics()
		if parsed := ParseTelnetMetrics(metricsRaw); len(parsed) > 0 {
			p.Metrics = MergeTelnetMetrics(parsed)
		}
		p.PreCommands = mikrotikcollect.ParseTelnetPreCommands(preRaw)
		if len(p.PreCommands) == 0 {
			p.PreCommands = DefaultTelnetPreCommands()
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ClearDefaultTelnetProfile(ctx context.Context, pool *pgxpool.Pool, except uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		UPDATE switch_telnet_profiles SET is_default=false, updated_at=now()
		WHERE is_default=true AND id <> $1
	`, except)
	return err
}

func IsTelnetProfileNameTaken(ctx context.Context, pool *pgxpool.Pool, name string, except uuid.UUID) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		SELECT id FROM switch_telnet_profiles
		WHERE lower(trim(name))=lower(trim($1)) AND ($2::uuid IS NULL OR id <> $2)
		LIMIT 1
	`, name, except).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
