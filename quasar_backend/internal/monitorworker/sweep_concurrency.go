package monitorworker

import (
	"context"
	"os"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"
)

// DefaultSweepConcurrency: quantos equipamentos podem ser sondados em paralelo
// dentro do mesmo tipo de ciclo (ping, telemetria, interfaces, BNG).
// O mesmo device continua serializado via snmpdevicelock / WithDeviceProbeRowLock.
const DefaultSweepConcurrency = 6

func sweepConcurrency() int {
	if v := stringsTrimEnvInt("NETQUASAR_SWEEP_CONCURRENCY"); v > 0 {
		if v > 32 {
			return 32
		}
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
