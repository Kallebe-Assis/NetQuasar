package monitorworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Slugs estáveis para rotas dinâmicas POST /monitoring/cycles/{cycle}
const (
	CycleSlugLatency       = "latency"
	CycleSlugTelemetry     = "telemetry"
	CycleSlugInterfaces    = "interfaces"
	CycleSlugOltIfDerived  = "olt-if-derived"
	CycleSlugBng           = "bng"
)

// ErrUnknownCycle o slug não corresponde a nenhum ciclo suportado.
var ErrUnknownCycle = errors.New("monitor: tipo de ciclo desconhecido")

// ErrCycleIntervalNotElapsed chamada API antes do intervalo mínimo (use force=true para ignorar).
var ErrCycleIntervalNotElapsed = errors.New("monitor: intervalo mínimo entre coletas ainda não decorreu")

// CycleKindMeta descreve um tipo de ciclo para listagens e documentação.
type CycleKindMeta struct {
	Slug          string `json:"slug"`
	RequiresFull  bool   `json:"requires_full"`
	IntervalField string `json:"interval_field"`
	Description   string `json:"description"`
}

// ListCycleKinds devolve os ciclos suportados (fonte única para GET /monitoring/cycles/kinds).
func ListCycleKinds() []CycleKindMeta {
	return []CycleKindMeta{
		{Slug: CycleSlugLatency, RequiresFull: false, IntervalField: "ping_seconds", Description: "ICMP/TCP (latência, reach_ok, ping_history)"},
		{Slug: CycleSlugTelemetry, RequiresFull: true, IntervalField: "telemetry_seconds", Description: "Telemetria SNMP (CPU, memória, temperatura, uptime)"},
		{Slug: CycleSlugInterfaces, RequiresFull: true, IntervalField: "interface_snapshot_seconds", Description: "Snapshots IF-MIB (+ Mikrotik quando aplicável)"},
		{Slug: CycleSlugOltIfDerived, RequiresFull: true, IntervalField: "olt_if_derived_pon_seconds", Description: "Coleta ONU/PON SNMP por OLT (round-robin; VSOL/ZTE/Datacom por perfil)"},
		{Slug: CycleSlugBng, RequiresFull: true, IntervalField: "telemetry_seconds", Description: "Totais de logins BNG (PPPoE, IPv4, IPv6) e saúde SNMP"},
	}
}

// NormalizeCycleSlug aceita aliases e devolve o slug canónico.
func NormalizeCycleSlug(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch s {
	case "", "latency", "ping", "icmp", "reachability":
		return CycleSlugLatency, nil
	case "telemetry", "snmp", "snmp-telemetry", "metrics":
		return CycleSlugTelemetry, nil
	case "interfaces", "iface", "if-mib", "if_mib":
		return CycleSlugInterfaces, nil
	case "olt-if-derived", "olt_if_derived", "pon", "olt-pon", "olt_pon":
		return CycleSlugOltIfDerived, nil
	case "bng", "bng-subscribers", "bng_subscribers", "subscribers":
		return CycleSlugBng, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownCycle, raw)
	}
}

// EnforceAPICycleInterval bloqueia chamadas API (source=api) sem force=true até decorrer o intervalo
// configurado desde a última coleta global ou, com device_id, desde a última coleta desse equipamento.
func EnforceAPICycleInterval(ctx context.Context, pool *pgxpool.Pool, slug string, opts SweepOpts) error {
	if opts.Source != "api" || opts.Force {
		return nil
	}
	cfg, err := loadClampMonitoringIntervals(ctx, pool)
	if err != nil {
		return err
	}

	switch slug {
	case CycleSlugLatency:
		if opts.DeviceID != nil {
			var last *time.Time
			_ = pool.QueryRow(ctx, `SELECT checked_at FROM device_probe_cache WHERE device_id=$1`, *opts.DeviceID).Scan(&last)
			if last != nil && time.Since(*last) < time.Duration(cfg.PingSeconds)*time.Second {
				return fmt.Errorf("%w: equipamento aguardar %ds", ErrCycleIntervalNotElapsed, cfg.PingSeconds)
			}
			return nil
		}
		var last *time.Time
		_ = pool.QueryRow(ctx, `SELECT last_latency_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&last)
		if last != nil && time.Since(*last) < time.Duration(cfg.PingSeconds)*time.Second {
			return fmt.Errorf("%w: global aguardar %ds", ErrCycleIntervalNotElapsed, cfg.PingSeconds)
		}
	case CycleSlugTelemetry:
		if opts.DeviceID != nil {
			var last *time.Time
			_ = pool.QueryRow(ctx, `
				SELECT max(collected_at) FROM telemetry_samples WHERE device_id=$1
			`, *opts.DeviceID).Scan(&last)
			if last != nil && time.Since(*last) < time.Duration(cfg.TelemetrySeconds)*time.Second {
				return fmt.Errorf("%w: equipamento aguardar %ds", ErrCycleIntervalNotElapsed, cfg.TelemetrySeconds)
			}
			return nil
		}
		var last *time.Time
		_ = pool.QueryRow(ctx, `SELECT last_telemetry_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&last)
		if last != nil && time.Since(*last) < time.Duration(cfg.TelemetrySeconds)*time.Second {
			return fmt.Errorf("%w: global aguardar %ds", ErrCycleIntervalNotElapsed, cfg.TelemetrySeconds)
		}
	case CycleSlugInterfaces:
		if opts.DeviceID != nil {
			var last *time.Time
			_ = pool.QueryRow(ctx, `SELECT max(collected_at) FROM interface_snapshots WHERE device_id=$1`, *opts.DeviceID).Scan(&last)
			if last != nil && time.Since(*last) < time.Duration(cfg.IfaceSeconds)*time.Second {
				return fmt.Errorf("%w: equipamento aguardar %ds", ErrCycleIntervalNotElapsed, cfg.IfaceSeconds)
			}
			return nil
		}
		var last *time.Time
		_ = pool.QueryRow(ctx, `SELECT last_interface_snapshot_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&last)
		if last != nil && time.Since(*last) < time.Duration(cfg.IfaceSeconds)*time.Second {
			return fmt.Errorf("%w: global aguardar %ds", ErrCycleIntervalNotElapsed, cfg.IfaceSeconds)
		}
	case CycleSlugOltIfDerived:
		if opts.DeviceID != nil {
			var last *time.Time
			_ = pool.QueryRow(ctx, `SELECT updated_at FROM olt_snapshots WHERE device_id=$1`, *opts.DeviceID).Scan(&last)
			if last != nil && time.Since(*last) < time.Duration(cfg.OltDerivedSeconds)*time.Second {
				return fmt.Errorf("%w: equipamento aguardar %ds", ErrCycleIntervalNotElapsed, cfg.OltDerivedSeconds)
			}
			return nil
		}
		var last *time.Time
		_ = pool.QueryRow(ctx, `SELECT last_olt_if_derived_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&last)
		if last != nil && time.Since(*last) < time.Duration(cfg.OltDerivedSeconds)*time.Second {
			return fmt.Errorf("%w: global aguardar %ds", ErrCycleIntervalNotElapsed, cfg.OltDerivedSeconds)
		}
	case CycleSlugBng:
		if opts.DeviceID != nil {
			var last *time.Time
			_ = pool.QueryRow(ctx, `SELECT max(collected_at) FROM bng_stats_samples WHERE device_id=$1`, *opts.DeviceID).Scan(&last)
			if last != nil && time.Since(*last) < time.Duration(cfg.TelemetrySeconds)*time.Second {
				return fmt.Errorf("%w: equipamento aguardar %ds", ErrCycleIntervalNotElapsed, cfg.TelemetrySeconds)
			}
			return nil
		}
		var last *time.Time
		_ = pool.QueryRow(ctx, `SELECT last_bng_cycle_at FROM monitoring_runtime WHERE id=1`).Scan(&last)
		if last != nil && time.Since(*last) < time.Duration(cfg.TelemetrySeconds)*time.Second {
			return fmt.Errorf("%w: global aguardar %ds", ErrCycleIntervalNotElapsed, cfg.TelemetrySeconds)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnknownCycle, slug)
	}
	return nil
}

// RunMonitorCycleBySlug executa um ciclo já validado em termos de modo (full vs simple).
func RunMonitorCycleBySlug(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, slug, mode string, opts SweepOpts) error {
	if opts.Source == "" {
		opts.Source = "worker"
	}
	switch slug {
	case CycleSlugLatency:
		return RunLatencySweep(ctx, pool, log, mode, opts)
	case CycleSlugTelemetry:
		return RunTelemetrySweep(ctx, pool, log, mode, opts)
	case CycleSlugInterfaces:
		return RunInterfaceSnapshotSweep(ctx, pool, log, mode, opts)
	case CycleSlugOltIfDerived:
		return RunOltIfDerivedSweep(ctx, pool, log, mode, opts)
	case CycleSlugBng:
		return RunBngSweep(ctx, pool, log, mode, opts)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownCycle, slug)
	}
}
