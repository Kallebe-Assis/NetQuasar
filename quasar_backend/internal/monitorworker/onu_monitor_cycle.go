package monitorworker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/onumonitor"
)

// lastOnuMonitorEval — cadência própria (60s) do avaliador de ONUs em monitoramento manual,
// sem coluna em monitoring_runtime (é barato: só lê onu_monitors + o snapshot já em cache).
var lastOnuMonitorEval atomic.Int64

// TryEvaluateOnuMonitors avalia as ONUs colocadas em monitoramento (tabela onu_monitors) e
// abre/fecha alertas de status e RX. Roda no máximo a cada 60s, numa goroutine própria.
func TryEvaluateOnuMonitors(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, opts SweepOpts) {
	if pool == nil {
		return
	}
	now := time.Now().Unix()
	if !opts.Force && now-lastOnuMonitorEval.Load() < 60 {
		return
	}
	if !lastOnuMonitorEval.CompareAndSwap(lastOnuMonitorEval.Load(), now) {
		return
	}
	go func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
		defer cancel()
		l := log.With().Str("cycle", "onu_monitor").Logger()
		if err := onumonitor.Evaluate(c, pool, &l); err != nil {
			l.Debug().Err(err).Msg("avaliação de ONUs em monitoramento falhou")
		}
	}()
}
