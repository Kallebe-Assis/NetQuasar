package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

type vsolIfPreload struct {
	Pons []map[string]any
	Refs []vsolparse.OnuRef
	Meta map[string]any
}

// collectVsolOLT: IF-MIB (opcional) + snmpwalk gOnuAuthList.
func (s *Server) collectVsolOLT(parentCtx context.Context, deviceID uuid.UUID, host, community string, _ time.Duration, fullTelemetry bool) ([]map[string]any, []vsolparse.OnuRef, map[string]any) {
	return s.collectVsolOLTWithPreload(parentCtx, deviceID, host, community, fullTelemetry, nil)
}

func (s *Server) collectVsolOLTWithPreload(parentCtx context.Context, deviceID uuid.UUID, host, community string, fullTelemetry bool, preload *vsolIfPreload) ([]map[string]any, []vsolparse.OnuRef, map[string]any) {
	sum := map[string]any{}
	var vPons []map[string]any
	var onuRefs []vsolparse.OnuRef
	var metaIF map[string]any

	if preload != nil && len(preload.Refs) > 0 {
		vPons = preload.Pons
		onuRefs = preload.Refs
		metaIF = preload.Meta
		for k, v := range preload.Meta {
			sum[k] = v
		}
		sum["if_mib_onu_counts"] = true
		sum["vsol_if_preloaded"] = true
	} else {
		ifTO := 55 * time.Second
		ifCtx, ifCancel := context.WithTimeout(parentCtx, ifTO)
		var ok bool
		if ponsIF, meta, refs, okIF := s.vsolPonRowsFromIfMIB(ifCtx, deviceID, host, community, ifTO, false); okIF {
			vPons = ponsIF
			onuRefs = refs
			metaIF = meta
			ok = true
			for k, v := range meta {
				sum[k] = v
			}
			sum["if_mib_onu_counts"] = true
		}
		ifCancel()
		if !ok {
			sum["vsol_get_note"] = "sem ONUs no IF-MIB — execute passo if_mib_refresh ou if_mib_snapshot"
		}
	}

	sum["vsol_onu_refs_count"] = len(onuRefs)
	if len(onuRefs) == 0 {
		if _, hasNote := sum["vsol_get_note"]; !hasNote {
			sum["vsol_get_note"] = "sem ONUs no IF-MIB — actualize interfaces antes do snapshot"
		}
		return vPons, onuRefs, sum
	}

	mibTO := vsolparse.CollectTimeout(len(onuRefs), fullTelemetry)
	mibCtx, mibCancel := context.WithTimeout(context.WithoutCancel(parentCtx), mibTO)
	defer mibCancel()

	coll := vsolparse.CollectOLT(mibCtx, host, community, onuRefs, fullTelemetry)
	onlineComplete := vsolparse.OnlineStepComplete(coll)
	sum["vsol_snmp_mode"] = "snmpwalk_gOnuAuthList"
	sum["vsol_online_complete"] = onlineComplete
	if coll.Truncated {
		sum["vsol_walk_truncated"] = true
	}
	sum["vsol_snmp_var_count"] = len(coll.Vars)
	sum["vsol_collect_steps"] = coll.Steps
	if fullTelemetry {
		sum["vsol_telemetry_vars"] = vsolparse.CountTelemetryVars(coll.Vars)
	}
	if coll.Note != "" {
		sum["vsol_get_note"] = coll.Note
	}
	if coll.Failed {
		sum["vsol_get_partial"] = true
	}

	var prevRows []map[string]any
	if !fullTelemetry && onlineComplete && s.DB() != nil {
		var prevSum []byte
		if err := s.DB().QueryRow(parentCtx, `SELECT summary::text FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&prevSum); err == nil {
			for _, it := range vsolparse.VsolOnuRowsFromSummaryBlob(prevSum) {
				if m, ok := it.(map[string]any); ok {
					prevRows = append(prevRows, m)
				}
			}
		}
	}

	mergeTelem := !fullTelemetry && onlineComplete
	onuRows := vsolparse.BuildOnuTable(onuRefs, coll.Vars, prevRows, mergeTelem)
	sum["vsol_onu_rows"] = vsolparse.OnuRowsToJSON(onuRows)
	sum["vsol_onu_table_count"] = len(onuRows)

	onBy, offBy := vsolparse.OnlineOfflineByPon(coll.Vars)
	finalPons := vPons
	if len(vPons) > 0 {
		if onlineComplete {
			finalPons = vsolparse.AttachOnlineOfflineToIfPons(vPons, onBy, offBy)
			vPons = finalPons
			vsolparse.ReconcileSummaryFromPons(sum, vPons)
		} else {
			sum["vsol_online_incomplete"] = true
			var prevPonsRaw []byte
			if err := s.DB().QueryRow(parentCtx, `SELECT COALESCE(pons::text,'[]') FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&prevPonsRaw); err == nil {
				prevMaps := oltifderive.PonsJSONToMaps(prevPonsRaw)
				prevByKey := map[string]map[string]any{}
				for _, p := range prevMaps {
					if k := oltifderive.StablePonRowKey(p); k != "" {
						prevByKey[k] = p
					}
				}
				for i := range vPons {
					key := oltifderive.StablePonRowKey(vPons[i])
					if prevP, ok := prevByKey[key]; ok {
						if v, ok := prevP["onu_online"]; ok {
							vPons[i]["onu_online"] = v
						}
						if v, ok := prevP["onu_offline"]; ok {
							vPons[i]["onu_offline"] = v
						}
						vPons[i]["online_source"] = "vsol_4.1.8_incomplete_carried"
					} else {
						vPons[i]["online_source"] = "vsol_4.1.8_incomplete"
					}
				}
			} else {
				for i := range vPons {
					vPons[i]["online_source"] = "vsol_4.1.8_incomplete"
				}
			}
			finalPons = vPons
		}
	}

	rep := vsolparse.BuildSnmpDebugReport(host, coll, metaIF, vPons, finalPons)
	sum["snmp_debug"] = vsolparse.DebugReportToMap(rep)
	return vPons, onuRefs, sum
}
