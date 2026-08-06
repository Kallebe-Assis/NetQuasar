package alertnotify

import (
	"sync"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/scheduleutil"
)

const displayTimezone = "America/Sao_Paulo"

var (
	displayLocOnce sync.Once
	displayLoc     *time.Location
)

func alertDisplayLocation() *time.Location {
	displayLocOnce.Do(func() {
		displayLoc = scheduleutil.LoadTZ(displayTimezone)
	})
	if displayLoc == nil {
		return time.UTC
	}
	return displayLoc
}

// FormatAlertDateTime formata instante no fuso America/Sao_Paulo (ex.: 06/08/2026 15:04).
func FormatAlertDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(alertDisplayLocation()).Format("02/01/2006 15:04")
}
