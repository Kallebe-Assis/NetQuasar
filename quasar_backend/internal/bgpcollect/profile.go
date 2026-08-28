package bgpcollect

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile perfil nomeado de coleta BGP (bgp_snmp_profiles) — mesmo padrão de
// mikrotikcollect.TelnetProfile, sem pre_commands (SNMP, não telnet) e sem associação por
// equipamento nesta entrega: a coleta periódica usa sempre o perfil is_default=true.
type Profile struct {
	ID        uuid.UUID     `json:"id"`
	Name      string        `json:"name"`
	Metrics   MetricsConfig `json:"metrics"`
	IsDefault bool          `json:"is_default"`
	UpdatedAt time.Time     `json:"updated_at,omitempty"`
}

func LoadProfileByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (Profile, error) {
	var p Profile
	var metricsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, is_default, updated_at
		FROM bgp_snmp_profiles WHERE id=$1
	`, id).Scan(&p.ID, &p.Name, &metricsRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return Profile{}, err
	}
	p.Metrics = DefaultMetrics()
	if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	return p, nil
}

// LoadDefaultProfile devolve o perfil is_default=true, ou um perfil "Padrão" em memória (com
// o catálogo pré-populado) se por algum motivo nenhum estiver marcado — nunca falha em vazio.
func LoadDefaultProfile(ctx context.Context, pool *pgxpool.Pool) Profile {
	var p Profile
	var metricsRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, is_default, updated_at
		FROM bgp_snmp_profiles
		WHERE is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&p.ID, &p.Name, &metricsRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return Profile{Name: "Padrão", Metrics: DefaultMetrics(), IsDefault: true}
	}
	p.Metrics = DefaultMetrics()
	if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	return p
}

func ListProfiles(ctx context.Context, pool *pgxpool.Pool) ([]Profile, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, metrics::text, is_default, updated_at
		FROM bgp_snmp_profiles
		ORDER BY is_default DESC, lower(trim(name))
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var metricsRaw []byte
		if err := rows.Scan(&p.ID, &p.Name, &metricsRaw, &p.IsDefault, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Metrics = DefaultMetrics()
		if parsed := ParseMetrics(metricsRaw); len(parsed) > 0 {
			p.Metrics = parsed.MergeWithDefaults()
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ClearDefaultProfile(ctx context.Context, pool *pgxpool.Pool, except uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		UPDATE bgp_snmp_profiles SET is_default=false, updated_at=now()
		WHERE is_default=true AND id <> $1
	`, except)
	return err
}

func IsProfileNameTaken(ctx context.Context, pool *pgxpool.Pool, name string, except uuid.UUID) (bool, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		SELECT id FROM bgp_snmp_profiles
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
