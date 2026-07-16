package monitorworker

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

// periodicCollectionSteps ajusta passos para coleta automática (ex.: VSOL métricas + walk inicial + telnet ONU).
func periodicCollectionSteps(profile oltcollect.Profile, brand, onuCollectMode string) []oltcollect.Step {
	steps := oltcollect.EffectiveCollectionSteps(profile)
	bl := strings.ToLower(strings.TrimSpace(brand))
	if strings.Contains(bl, "vsol") {
		if len(steps) == 1 && steps[0].Method == oltcollect.MethodOnuMetricsCollect {
			walk := oltcollect.Step{
				ID:       "onu_walk_first",
				Method:   oltcollect.MethodOnuSNMPWalk,
				OIDField: "onu_online_oid",
			}
			steps = append([]oltcollect.Step{walk}, steps...)
		}
	}
	if !oltcollect.IncludesTelnetOnuCollectMode(onuCollectMode) {
		return steps
	}
	return oltcollect.AppendPonTelnetCollectStep(
		oltcollect.AppendOnuTelnetReportStep(steps, profile),
		profile,
	)
}
