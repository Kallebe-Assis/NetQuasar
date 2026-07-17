package monitorworker

import (
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
)

// periodicCollectionSteps ajusta passos para coleta automática (ex.: telnet ONU).
// VSOL: não prepend onu_snmp_walk quando já há onu_metrics_collect — o walk duplicado
// esgota o orçamento (timeout ~45s · walk vazio nas colunas). Fallback fica no worker.
func periodicCollectionSteps(profile oltcollect.Profile, brand, onuCollectMode string) []oltcollect.Step {
	steps := oltcollect.EffectiveCollectionSteps(profile)
	_ = brand
	if NormalizeOltOnuMode(onuCollectMode) == "baseline" && profile.OnuMetrics.HasAnyEnabled() {
		hasMetrics := false
		for _, step := range steps {
			if step.Method == oltcollect.MethodOnuMetricsCollect {
				hasMetrics = true
				break
			}
		}
		if !hasMetrics {
			enabled := true
			steps = append(steps, oltcollect.Step{
				ID: "monitoring_baseline_metrics", Method: oltcollect.MethodOnuMetricsCollect, Enabled: &enabled,
			})
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
