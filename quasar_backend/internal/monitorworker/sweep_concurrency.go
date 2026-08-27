package monitorworker

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// DefaultSweepConcurrency: quantos equipamentos podem ser sondados em paralelo
// dentro do mesmo tipo de ciclo (ping, telemetria, interfaces, BNG, OLT).
// O mesmo device continua serializado via snmpdevicelock / WithDeviceProbeRowLock.
//
// Valor revisto de 6 para 12 com base num benchmark de simulação (ver
// DIAGNOSTICO-PERFORMANCE-ARQUITETURA.md): com poucos equipamentos (~75) o ganho
// de 6->12 já é perceptível, e com centenas de equipamentos o ganho é substancial
// sem sobrecarregar equipamentos individuais (cada um continua serializado).
const DefaultSweepConcurrency = 12
const maxSweepConcurrency = 64

// dbSweepConcurrency é actualizado a cada tick do worker a partir de
// monitoring_intervals.sweep_concurrency (ver SetSweepConcurrencyFromConfig em
// interval_config.go). 0 = usar o default. Isto permite tornar a concorrência
// configurável (futuramente pela UI de Configurações) sem ter de propagar o valor
// por todos os pontos que chamam sweepConcurrency().
var dbSweepConcurrency int32

// SetSweepConcurrencyFromConfig regista a concorrência vinda de monitoring_intervals.
// Chamado uma vez por tick do worker (loadClampMonitoringIntervals). A variável de
// ambiente NETQUASAR_SWEEP_CONCURRENCY, quando definida, continua a ter prioridade
// (útil como válvula de escape operacional sem mexer em configuração persistida).
func SetSweepConcurrencyFromConfig(v int) {
	if v < 0 {
		v = 0
	}
	if v > maxSweepConcurrency {
		v = maxSweepConcurrency
	}
	atomic.StoreInt32(&dbSweepConcurrency, int32(v))
}

func sweepConcurrency() int {
	if v := stringsTrimEnvInt("NETQUASAR_SWEEP_CONCURRENCY"); v > 0 {
		if v > maxSweepConcurrency {
			return maxSweepConcurrency
		}
		return v
	}
	if v := int(atomic.LoadInt32(&dbSweepConcurrency)); v > 0 {
		return v
	}
	return DefaultSweepConcurrency
}

func stringsTrimEnvInt(key string) int {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// forEachLimited corre fn(i) para i in [0,n) com no máximo limit goroutines.
// Erros de fn são ignorados (cada equipamento trata o seu); cancela quando ctx acaba.
func forEachLimited(ctx context.Context, n, limit int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if limit < 1 {
		limit = 1
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i := 0; i < n; i++ {
		i := i
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			fn(i)
			return nil
		})
	}
	_ = g.Wait()
}

// counter helpers for concurrent sweeps
type sweepCounters struct {
	mu                                 sync.Mutex
	ok, fail, skip, eligible, processed int
}

func (c *sweepCounters) addOK() {
	c.mu.Lock()
	c.ok++
	c.mu.Unlock()
}
func (c *sweepCounters) addFail() {
	c.mu.Lock()
	c.fail++
	c.mu.Unlock()
}
func (c *sweepCounters) addSkip() {
	c.mu.Lock()
	c.skip++
	c.mu.Unlock()
}
func (c *sweepCounters) addEligible() {
	c.mu.Lock()
	c.eligible++
	c.mu.Unlock()
}
func (c *sweepCounters) addProcessed() {
	c.mu.Lock()
	c.processed++
	c.mu.Unlock()
}

type atomicInt struct {
	mu sync.Mutex
	n  int
}

func (a *atomicInt) inc() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.n++
	return a.n
}
