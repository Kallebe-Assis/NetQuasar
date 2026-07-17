package monitorworker

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NormalizeOltOnuMode normaliza o modo de coleta ONU do pipeline.
func NormalizeOltOnuMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "baseline":
		return "baseline"
	case "pon_status":
		return "pon_status"
	case "onu_counts":
		return "onu_counts"
	case "status_only":
		return "status_only"
	case "status_rx":
		return "status_rx"
	case "full", "":
		return "full"
	default:
		return "full"
	}
}

func oltTierRuntimeColumn(mode string) string {
	switch NormalizeOltOnuMode(mode) {
	case "pon_status":
		return "last_olt_pon_status_at"
	case "baseline", "onu_counts", "status_only", "status_rx":
		return "last_olt_onu_counts_at"
	default:
		return "last_olt_full_collect_at"
	}
}

func oltTierIntervalSeconds(cfg intervalConfig, mode string) int {
	switch NormalizeOltOnuMode(mode) {
	case "pon_status":
		return cfg.OltPonStatusSeconds
	case "baseline", "onu_counts", "status_only", "status_rx":
		return cfg.OltOnuCountsSeconds
	default:
		return cfg.OltFullCollectSeconds
	}
}

// oltOnuStepDue indica se o passo olt_onu deve correr neste ciclo do pipeline.
// Coleta full: intervalo > 0, ou horário agendado (HH:MM), ou Force.
func oltOnuStepDue(ctx context.Context, pool *pgxpool.Pool, cfg intervalConfig, mode string, force bool) bool {
	if force {
		return true
	}
	mode = NormalizeOltOnuMode(mode)
	if mode == "baseline" {
		// A linha-base faz parte de todo ciclo completo, independentemente dos
		// intervalos dos tiers opcionais.
		return true
	}
	col := oltTierRuntimeColumn(mode)
	var last *time.Time
	_ = pool.QueryRow(ctx, `SELECT `+col+` FROM monitoring_runtime WHERE id=1`).Scan(&last)

	if mode == "full" {
		interval := cfg.OltFullCollectSeconds
		schedule := strings.TrimSpace(cfg.OltFullCollectSchedule)
		if interval <= 0 && schedule == "" {
			// Só manual / Force — não corre no pipeline automático.
			return false
		}
		if interval > 0 {
			if last == nil || time.Since(*last) >= time.Duration(interval)*time.Second {
				return true
			}
			return false
		}
		return oltFullScheduleDue(schedule, last, time.Now())
	}

	sec := oltTierIntervalSeconds(cfg, mode)
	if sec < 30 {
		sec = 60
	}
	if last == nil {
		return true
	}
	return time.Since(*last) >= time.Duration(sec)*time.Second
}

func markOltOnuTierRan(ctx context.Context, pool *pgxpool.Pool, mode string) {
	if pool == nil {
		return
	}
	col := oltTierRuntimeColumn(mode)
	_, _ = pool.Exec(ctx, `UPDATE monitoring_runtime SET `+col+` = now(), updated_at = now() WHERE id=1`)
}

// MarkOltOnuTierRan regista a última execução de um tier (ex.: refresh manual = full).
func MarkOltOnuTierRan(ctx context.Context, pool *pgxpool.Pool, mode string) {
	markOltOnuTierRan(ctx, pool, mode)
}

// oltFullScheduleDue — true se a hora local actual >= HH:MM e ainda não correu hoje após esse horário.
func oltFullScheduleDue(schedule string, last *time.Time, now time.Time) bool {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return false
	}
	parts := strings.Split(schedule, ":")
	if len(parts) < 2 {
		return false
	}
	hh, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	mm, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return false
	}
	loc := now.Location()
	target := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, loc)
	if now.Before(target) {
		return false
	}
	if last == nil {
		return true
	}
	lastLocal := last.In(loc)
	if lastLocal.Year() == now.Year() && lastLocal.YearDay() == now.YearDay() && !lastLocal.Before(target) {
		return false
	}
	return true
}
