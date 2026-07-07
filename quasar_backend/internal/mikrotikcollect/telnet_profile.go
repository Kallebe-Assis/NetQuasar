package mikrotikcollect

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelnetProfile perfil nomeado de coleta telnet MikroTik.
type TelnetProfile struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Metrics     TelnetMetricsConfig `json:"metrics"`
	PreCommands []string          `json:"pre_commands"`
	IsDefault   bool              `json:"is_default"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

func ParseTelnetPreCommands(raw []byte) []string {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return nil
	}
	var cmds []string
	if json.Unmarshal(raw, &cmds) != nil {
		return nil
	}
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if t := strings.TrimSpace(c); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func LoadTelnetProfileByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (TelnetProfile, error) {
	var p TelnetProfile
	var metricsRaw, preRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM mikrotik_telnet_profiles WHERE id=$1
	`, id).Scan(&p.ID, &p.Name, &metricsRaw, &preRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		return TelnetProfile{}, err
	}
	p.Metrics = DefaultTelnetMetrics()
	if parsed := ParseTelnetMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	p.PreCommands = ParseTelnetPreCommands(preRaw)
	return p, nil
}

func LoadDefaultTelnetProfile(ctx context.Context, pool *pgxpool.Pool) TelnetProfile {
	var p TelnetProfile
	var metricsRaw, preRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM mikrotik_telnet_profiles
		WHERE is_default = true
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(&p.ID, &p.Name, &metricsRaw, &preRaw, &p.IsDefault, &p.UpdatedAt)
	if err != nil {
		p = TelnetProfile{
			Name:    "Padrão",
			Metrics: DefaultTelnetMetrics(),
		}
		return p
	}
	p.Metrics = DefaultTelnetMetrics()
	if parsed := ParseTelnetMetrics(metricsRaw); len(parsed) > 0 {
		p.Metrics = parsed.MergeWithDefaults()
	}
	p.PreCommands = ParseTelnetPreCommands(preRaw)
	return p
}

// LoadTelnetProfileForDevice resolve perfil telnet do equipamento ou o padrão.
func LoadTelnetProfileForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) TelnetProfile {
	if pool == nil {
		return TelnetProfile{Name: "Padrão", Metrics: DefaultTelnetMetrics()}
	}
	var profileID *uuid.UUID
	_ = pool.QueryRow(ctx, `
		SELECT mikrotik_telnet_profile_id FROM devices WHERE id=$1
	`, deviceID).Scan(&profileID)
	if profileID != nil {
		if p, err := LoadTelnetProfileByID(ctx, pool, *profileID); err == nil {
			return p
		}
	}
	return LoadDefaultTelnetProfile(ctx, pool)
}

// TelnetCredentials credenciais para sessão telnet.
type TelnetCredentials struct {
	User     string
	Password string
	Enable   string
	Port     string
}

func LoadTelnetCredentials(ctx context.Context, pool *pgxpool.Pool) TelnetCredentials {
	return LoadTelnetCredentialsForDevice(ctx, pool, uuid.Nil)
}

// LoadTelnetCredentialsForDevice usa credenciais do equipamento quando definidas; caso contrário, os padrões globais.
func LoadTelnetCredentialsForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) TelnetCredentials {
	c := TelnetCredentials{Port: "23"}
	if pool == nil {
		return c
	}
	var devUser, devPass, devEnable *string
	if deviceID != uuid.Nil {
		_ = pool.QueryRow(ctx, `
			SELECT telnet_user, telnet_password, telnet_enable
			FROM devices WHERE id=$1
		`, deviceID).Scan(&devUser, &devPass, &devEnable)
	}
	if devUser != nil && strings.TrimSpace(*devUser) != "" {
		c.User = strings.TrimSpace(*devUser)
	}
	if devPass != nil && strings.TrimSpace(*devPass) != "" {
		c.Password = strings.TrimSpace(*devPass)
	}
	if devEnable != nil && strings.TrimSpace(*devEnable) != "" {
		c.Enable = strings.TrimSpace(*devEnable)
	}
	if c.User != "" && c.Password != "" {
		return c
	}
	var user, pass, enable *string
	_ = pool.QueryRow(ctx, `
		SELECT telnet_user, telnet_password, telnet_enable
		FROM settings_connection_defaults WHERE id=1
	`).Scan(&user, &pass, &enable)
	if c.User == "" && user != nil {
		c.User = strings.TrimSpace(*user)
	}
	if c.Password == "" && pass != nil {
		c.Password = strings.TrimSpace(*pass)
	}
	if c.Enable == "" && enable != nil {
		c.Enable = strings.TrimSpace(*enable)
	}
	return c
}

func ListTelnetProfiles(ctx context.Context, pool *pgxpool.Pool) ([]TelnetProfile, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, metrics::text, pre_commands::text, is_default, updated_at
		FROM mikrotik_telnet_profiles
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
			p.Metrics = parsed.MergeWithDefaults()
		}
		p.PreCommands = ParseTelnetPreCommands(preRaw)
		out = append(out, p)
	}
	return out, rows.Err()
}

func ClearDefaultTelnetProfile(ctx context.Context, pool *pgxpool.Pool, except uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		UPDATE mikrotik_telnet_profiles SET is_default=false, updated_at=now()
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
		SELECT id FROM mikrotik_telnet_profiles
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
