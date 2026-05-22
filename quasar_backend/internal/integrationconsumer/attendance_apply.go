package integrationconsumer

import (
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// ApplyAttendanceContext aplica parâmetros da consulta de atendimentos conforme o ERP.
func ApplyAttendanceContext(
	rc integrationhttp.RequestConfig,
	profile, busca, termo, apenasPendente string,
	attCfg ClientAttendanceConfig,
	searchCfg ClientSearchConfig,
) integrationhttp.RequestConfig {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == ProviderIXC || (profile == ProviderAuto && LooksLikeIXCAttendanceRequest(rc)) {
		return PrepareIXCAttendanceListRequest(rc, busca, termo, attCfg, searchCfg)
	}
	if LooksLikeIXCAttendanceRequest(rc) {
		return PrepareIXCAttendanceListRequest(rc, busca, termo, attCfg, searchCfg)
	}
	overrides := HubsoftAttendanceQueryOverrides(busca, termo, apenasPendente)
	return ApplyQueryOverrides(rc, overrides)
}

// DetectAttendanceProfile infere ERP para atendimentos.
func DetectAttendanceProfile(configured string, rc integrationhttp.RequestConfig, baseURL string) string {
	if LooksLikeIXCAttendanceRequest(rc) || LooksLikeIXCRequest(rc, baseURL) {
		return ProviderIXC
	}
	configured = strings.ToLower(strings.TrimSpace(configured))
	switch configured {
	case ProviderHubsoft, ProviderIXC, ProviderGeneric:
		return configured
	}
	if looksLikeHubsoftAttendanceRequest(rc) {
		return ProviderHubsoft
	}
	return ProviderGeneric
}

func looksLikeHubsoftAttendanceRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	return strings.Contains(path, "integracao/cliente/atendimento")
}

// LooksLikeIXCAttendanceRequest indica listagem IXC de tickets (su_ticket).
func LooksLikeIXCAttendanceRequest(rc integrationhttp.RequestConfig) bool {
	path := strings.ToLower(strings.TrimSpace(rc.Path))
	if strings.Contains(path, "su_ticket") {
		return true
	}
	for k, v := range rc.Headers {
		if strings.EqualFold(k, "ixcsoft") && strings.TrimSpace(v) != "" {
			body := strings.ToLower(rc.BodyTemplate)
			if strings.Contains(body, "su_ticket") || strings.Contains(path, "ticket") {
				return true
			}
		}
	}
	return false
}
