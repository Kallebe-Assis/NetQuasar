package monitorworker

import (
	"context"
	"sync"
)

var (
	activeRunMu sync.Mutex
	activeRun   context.Context
	activeCancel context.CancelFunc
)

// BeginActiveRun cria um contexto cancelável para ciclos em curso (filho de parent).
func BeginActiveRun(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	activeRunMu.Lock()
	defer activeRunMu.Unlock()
	if activeCancel != nil {
		activeCancel()
	}
	activeRun, activeCancel = context.WithCancel(parent)
}

// EndActiveRun cancela ciclos em curso (ex.: POST /monitoring/stop).
func EndActiveRun() {
	activeRunMu.Lock()
	defer activeRunMu.Unlock()
	if activeCancel != nil {
		activeCancel()
		activeCancel = nil
		activeRun = nil
	}
}

// ActiveRunContext devolve o contexto activo dos ciclos ou fallback.
func ActiveRunContext(fallback context.Context) context.Context {
	activeRunMu.Lock()
	ac := activeRun
	activeRunMu.Unlock()
	if ac == nil {
		return fallback
	}
	return ac
}

// EnsureActiveRun garante contexto activo quando monitoramento está ligado (ex.: restart do servidor).
func EnsureActiveRun(parent context.Context) {
	activeRunMu.Lock()
	has := activeRun != nil
	activeRunMu.Unlock()
	if !has {
		BeginActiveRun(parent)
	}
}
