package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// vsolPonRowsFromIfMIB agrega ONUs por PON via IF-MIB (GPONxxONUyy). Walk direcionado + união com snapshot.
func (s *Server) vsolPonRowsFromIfMIB(ctx context.Context, deviceID uuid.UUID, host, community string, budget time.Duration, snapshotOnly bool) ([]map[string]any, map[string]any, []vsolparse.OnuRef, bool) {
	raw := s.loadIfSnapshotRaw(ctx, deviceID)
	var ds oltifderive.IfMibDataset
	if snapshotOnly && len(raw) > 0 {
		ds = oltifderive.DatasetFromSnapshot(raw)
		if CountOnuRefsFromDataset(ds) == 0 {
			ds = oltifderive.LoadIFMibDataset(ctx, host, community, budget, raw)
			if ds.Note != "" {
				ds.Note = "snapshot_sem_onus;" + ds.Note
			} else {
				ds.Note = "snapshot_sem_onus;walk_snmp"
			}
		}
	} else {
		ds = oltifderive.LoadIFMibDataset(ctx, host, community, budget, raw)
	}
	refs := vsolparse.OnuRefsFromIfRows(ds.Rows)
	meta := map[string]any{
		"if_mib_source":       ds.Source,
		"if_mib_onu_ifaces":   ds.OnuIfaces,
		"if_mib_pon_with_onu": ds.PonWithOnu,
		"if_mib_truncated":    ds.Truncated,
		"vsol_onu_refs":       len(refs),
	}
	if ds.Note != "" {
		meta["if_mib_note"] = ds.Note
	}
	if !okDataset(ds) {
		return nil, meta, refs, false
	}
	opt := snmpmikrotik.OpticalPowerByIfIndex(ds.Rows, ds.Vars)
	rows := oltifderive.BuildPonSnapshotFromIfMIB(ds.Rows, opt)
	meta["if_mib_pon_rows"] = len(rows)
	return rows, meta, refs, len(rows) > 0
}

func okDataset(ds oltifderive.IfMibDataset) bool {
	return len(ds.Rows) > 0 && (ds.OnuIfaces > 0 || ds.PonWithOnu > 0)
}

func CountOnuRefsFromDataset(ds oltifderive.IfMibDataset) int {
	return len(vsolparse.OnuRefsFromIfRows(ds.Rows))
}

func (s *Server) loadIfSnapshotRaw(ctx context.Context, deviceID uuid.UUID) []byte {
	pool := s.DB()
	if pool == nil {
		return nil
	}
	var ifRaw []byte
	if err := pool.QueryRow(ctx, `
		SELECT interfaces::text FROM interface_snapshots WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
	`, deviceID).Scan(&ifRaw); err != nil {
		return nil
	}
	return ifRaw
}
