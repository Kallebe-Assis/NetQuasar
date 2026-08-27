//go:build !linux

package sniffer

import (
	"context"
	"errors"
	"time"
)

// captureLocal só é suportado em Linux (é onde o NetQuasar corre, em produção — Docker/Debian).
// Este stub existe só para que o pacote continue a compilar em builds/dev noutros sistemas.
func captureLocal(_ context.Context, _ string, _ func(raw []byte, ts time.Time) bool) error {
	return errors.New("captura local só é suportada em Linux (o servidor NetQuasar corre em Linux/Docker)")
}
