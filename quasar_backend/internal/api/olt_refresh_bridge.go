package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
)

type serverOltManualRefresher struct {
	s *Server
}

func (r *serverOltManualRefresher) RefreshOLT(ctx context.Context, deviceID uuid.UUID, source string) monitorworker.OltCollectOutcome {
	if r == nil || r.s == nil {
		return monitorworker.OltCollectOutcome{Mode: "manual_refresh", Reason: "refresher indisponível"}
	}
	res, err := r.s.refreshOLTDeviceCore(ctx, deviceID, OltRefreshCoreOpts{Source: source})
	out := monitorworker.OltCollectOutcome{
		OK:       res.OK,
		PonCount: res.PonCount,
		Reason:   res.Reason,
		Mode:     res.Mode,
	}
	if err != nil && out.Reason == "" {
		out.Reason = err.Error()
	}
	return out
}

func registerOltManualRefresher(s *Server) {
	if s == nil {
		return
	}
	monitorworker.SetOltManualRefresher(&serverOltManualRefresher{s: s})
}
