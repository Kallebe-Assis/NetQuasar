//go:build linux

package api

import "syscall"

// diskUsage devolve o espaço livre/total (bytes) do sistema de arquivos que contém `path`.
// No Compose, o volume de dados do NetQuasar e o do Postgres ficam no MESMO disco do servidor, então isto reflete o disco real.
func diskUsage(path string) (free, total int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	bsize := int64(st.Bsize)
	return int64(st.Bavail) * bsize, int64(st.Blocks) * bsize, true
}
