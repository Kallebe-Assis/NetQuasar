package oltcollect

import "strings"

// ScopeOnu coleta rápida: ONUs/PONs sem IF-MIB completo nem estabilização.
const ScopeOnu = "onu"

// ScopeFull executa todos os passos activos do perfil.
const ScopeFull = "full"

// StepsForScope reduz passos para testes rápidos de ONUs (scope=onu).
func StepsForScope(steps []Step, scope string) []Step {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope != ScopeOnu && scope != "fast" {
		return steps
	}
	var hasVsol, hasSnapshot bool
	out := make([]Step, 0, len(steps))
	for _, s := range steps {
		switch s.Method {
		case MethodIfMibRefresh, MethodIfMibMergePons, MethodStabilizePons:
			continue
		case MethodOnuSNMPWalk:
			out = append(out, s)
			return out
		case MethodOnuMetricsCollect:
			out = append(out, s)
			return out
		case MethodIfMibSnapshot:
			hasSnapshot = true
			out = append(out, s)
		case MethodVsolOnuCollect:
			hasVsol = true
			out = append(out, s)
		case MethodSNMPWalk, MethodSNMPGet, MethodTelnet, MethodDatacomBuildPons:
			out = append(out, s)
		}
	}
	if hasVsol && !hasSnapshot {
		en := true
		out = append([]Step{{ID: "if_snap_auto", Method: MethodIfMibSnapshot, Enabled: &en}}, out...)
	}
	if len(out) == 0 {
		return steps
	}
	return out
}
