package monitorworker

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

// periodicCollectionSteps ajusta passos para coleta automática (ex.: VSOL métricas + walk inicial).
func periodicCollectionSteps(profile oltcollect.Profile, brand string) []oltcollect.Step {
	steps := oltcollect.EffectiveCollectionSteps(profile)
	bl := strings.ToLower(strings.TrimSpace(brand))
	if !strings.Contains(bl, "vsol") {
		return steps
	}
	if len(steps) == 1 && steps[0].Method == oltcollect.MethodOnuMetricsCollect {
		walk := oltcollect.Step{
			ID:       "onu_walk_first",
			Method:   oltcollect.MethodOnuSNMPWalk,
			OIDField: "onu_online_oid",
		}
		return append([]oltcollect.Step{walk}, steps...)
	}
	return steps
}
