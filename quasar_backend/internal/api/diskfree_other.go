//go:build !linux

package api

// diskUsage não está disponível fora do Linux (desenvolvimento no Windows/macOS): a limpeza usa limites conservadores.
func diskUsage(path string) (free, total int64, ok bool) {
	return 0, 0, false
}
