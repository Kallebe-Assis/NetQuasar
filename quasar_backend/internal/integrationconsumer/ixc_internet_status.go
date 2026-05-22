package integrationconsumer

import "strings"

func looksLikeIXCInternetStatusCode(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "A", "D", "CM", "CA", "FA", "AA":
		return true
	default:
		return false
	}
}

// IsLoginBusca indica pesquisa por login PPPoE/RADIUS (radusuarios / Hubsoft login_radius).
func IsLoginBusca(busca string) bool {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "login", "login_radius":
		return true
	default:
		return false
	}
}
