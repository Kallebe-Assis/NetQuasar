package integrationconsumer

import "strings"

// IsLoginBusca indica pesquisa por login PPPoE/RADIUS (radusuarios / Hubsoft login_radius).
func IsLoginBusca(busca string) bool {
	switch strings.ToLower(strings.TrimSpace(busca)) {
	case "login", "login_radius":
		return true
	default:
		return false
	}
}
