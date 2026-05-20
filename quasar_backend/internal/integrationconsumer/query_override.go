package integrationconsumer

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// ApplyQueryOverrides substitui ou acrescenta parâmetros de query na requisição.
func ApplyQueryOverrides(rc integrationhttp.RequestConfig, overrides map[string]string) integrationhttp.RequestConfig {
	if len(overrides) == 0 {
		return rc
	}
	byKey := map[string]integrationhttp.ParamKV{}
	for _, p := range rc.QueryParams {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			continue
		}
		byKey[key] = p
	}
	for k, v := range overrides {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		en := true
		if existing, ok := byKey[k]; ok {
			existing.Value = v
			if existing.Enabled != nil {
				en = *existing.Enabled
			}
			byKey[k] = existing
		} else {
			byKey[k] = integrationhttp.ParamKV{Key: k, Value: v, Enabled: &en}
		}
	}
	out := make([]integrationhttp.ParamKV, 0, len(byKey))
	for _, p := range byKey {
		out = append(out, p)
	}
	rc.QueryParams = out
	return rc
}
