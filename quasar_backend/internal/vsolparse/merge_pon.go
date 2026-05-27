package vsolparse

import (
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
)

// MergeWithIfPonPorts acrescenta portas PON físicas (IF-MIB GPON0/N) sem alterar contagens VSOL existentes.
func MergeWithIfPonPorts(vsolPons []map[string]any, ifPonPorts []map[string]any) []map[string]any {
	return oltifderive.MergePonRowsForIfaceRefresh(vsolPons, ifPonPorts)
}
