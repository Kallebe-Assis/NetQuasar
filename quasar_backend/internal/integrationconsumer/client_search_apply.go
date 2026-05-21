package integrationconsumer

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// ApplyClientSearchContext aplica parâmetros de consulta conforme o perfil do ERP (sem alterar auth).
func ApplyClientSearchContext(rc integrationhttp.RequestConfig, profile, busca, termo string, detailed bool, cfg ClientSearchConfig) integrationhttp.RequestConfig {
	profile = strings.ToLower(strings.TrimSpace(profile))
	busca = strings.TrimSpace(busca)
	termo = strings.TrimSpace(termo)
	if busca == "" {
		busca = "cpf_cnpj"
	}

	switch profile {
	case ProviderIXC:
		return PrepareIXCClientListRequest(rc, busca, termo, detailed, cfg)

	case ProviderHubsoft:
		if LooksLikeIXCRequest(rc, "") {
			return PrepareIXCClientListRequest(rc, busca, termo, detailed, cfg)
		}
		overrides := HubsoftSearchQueryOverrides(detailed)
		overrides["busca"] = busca
		overrides["termo_busca"] = termo
		return ApplyQueryOverrides(rc, overrides)

	default:
		if LooksLikeIXCRequest(rc, "") {
			return PrepareIXCClientListRequest(rc, busca, termo, detailed, cfg)
		}
		overrides := map[string]string{
			"busca":       busca,
			"termo_busca": termo,
			"query":       termo,
		}
		return ApplyQueryOverrides(rc, overrides)
	}
}
