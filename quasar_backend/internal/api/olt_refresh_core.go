package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
)

// OltRefreshCoreOpts espelha o refresh manual (POST /devices/{id}/refresh) sem parâmetros extra na query.
type OltRefreshCoreOpts struct {
	Source        string
	Scope         string
	FullTelemetry bool
}

// OltRefreshCoreResult resultado da coleta (mesma lógica do botão manual).
type OltRefreshCoreResult struct {
	OK        bool
	PonCount  int
	Reason    string
	Mode      string
	TimeoutMs int64
}

// refreshOLTDeviceCore executa exactamente o mesmo fluxo que refreshOLTDevice (sem HTTP nem lock SNMP).
func (s *Server) refreshOLTDeviceCore(ctx context.Context, id uuid.UUID, opts OltRefreshCoreOpts) (OltRefreshCoreResult, error) {
	out := OltRefreshCoreResult{Mode: "manual_refresh"}
	pool := s.DB()
	if pool == nil {
		return out, fmt.Errorf("base de dados indisponível")
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "olt_refresh"
	}
	summary := map[string]any{
		"source":     source,
		"status":     "updated",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	pons := []any{}

	var ip *string
	var comm *string
	var brand, model, devDesc string
	var maxPons *int
	if err := pool.QueryRow(ctx, `
		SELECT host(d.ip)::text, d.snmp_community,
			coalesce(trim(d.brand), ''), coalesce(trim(d.model), ''),
			coalesce(trim(d.description), ''), d.max_pons
		FROM devices d WHERE d.id=$1
	`, id).Scan(&ip, &comm, &brand, &model, &devDesc, &maxPons); err != nil {
		return out, err
	}
	host := ""
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	c := ""
	if comm != nil {
		c = strings.TrimSpace(*comm)
	}
	if c == "" {
		_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&comm)
		if comm != nil {
			c = strings.TrimSpace(*comm)
		}
	}
	if host == "" || c == "" {
		out.Reason = "IP e community SNMP obrigatórios para refresh OLT"
		return out, fmt.Errorf("%s", out.Reason)
	}
	if strings.TrimSpace(model) == "" {
		out.Reason = "modelo OLT obrigatório"
		return out, fmt.Errorf("%s", out.Reason)
	}

	profile, profErr := loadOltCollectionProfile(ctx, pool, brand, model)
	if profErr != nil {
		out.Reason = fmt.Sprintf("perfil OLT não encontrado para %s / %s", brand, model)
		return out, profErr
	}

	collTO := s.loadCollectionTimeouts(ctx)
	telnetTO := collTO.OltOnuTelnetTimeout()
	oltRefreshTotal := collTO.OltRefreshTotal()
	scope := strings.TrimSpace(strings.ToLower(opts.Scope))
	if scope == "" {
		scope = oltcollect.ScopeFull
	}
	if scope == "fast" {
		scope = oltcollect.ScopeOnu
	}
	snmpBudget := oltRefreshTotal
	if oltcollect.IsSimpleOnuCollect(profile.Steps) || scope == oltcollect.ScopeOnu {
		snmpBudget = 300 * time.Second
	}
	needsTelnet := profile.OnuReport.MonitorEnabled() || profile.PonTelnet.MonitorEnabled()
	if needsTelnet {
		oltRefreshTotal = snmpBudget + telnetTO
	} else if oltcollect.IsSimpleOnuCollect(profile.Steps) || scope == oltcollect.ScopeOnu {
		oltRefreshTotal = snmpBudget
	}
	out.TimeoutMs = oltRefreshTotal.Milliseconds()
	oltCtx, oltCancel := context.WithTimeout(context.WithoutCancel(ctx), oltRefreshTotal)
	defer oltCancel()

	refreshT0 := time.Now()
	maxPonsVal := 0
	if maxPons != nil && *maxPons > 0 {
		maxPonsVal = *maxPons
	}
	execSt := &oltCollectExecState{
		DeviceID: id, Host: host, Community: c,
		Brand: brand, Model: model, DevDesc: devDesc,
		MaxPons: maxPonsVal,
		Profile: profile, Summary: summary, Pons: pons,
		FullTelemetry: opts.FullTelemetry, TelnetTO: telnetTO,
		Scope: scope,
	}
	if err := s.executeOltProfile(oltCtx, execSt); err != nil {
		summary["olt_profile_exec_error"] = err.Error()
	}
	if oltCtx.Err() != nil {
		summary["olt_refresh_timeout"] = true
		summary["olt_refresh_timeout_reason"] = oltCtx.Err().Error()
		if _, ok := summary["olt_refresh_cancelled"]; !ok {
			summary["olt_refresh_cancelled"] = oltCtx.Err().Error()
		}
	}
	pons = execSt.Pons
	pons = applyMaxPonsLimitAnyRows(pons, maxPons)
	for k, v := range execSt.Summary {
		summary[k] = v
	}
	summary["olt_refresh_elapsed_ms"] = time.Since(refreshT0).Milliseconds()

	curMaps := oltifderive.PonsAnySliceToMaps(pons)
	incomplete := oltcollect.IsOltSnapshotIncomplete(summary)
	var prevSnapPons, prevSnapSum []byte
	_ = pool.QueryRow(ctx, `SELECT COALESCE(pons::text,'[]'), COALESCE(summary::text,'{}') FROM olt_snapshots WHERE device_id=$1`, id).Scan(&prevSnapPons, &prevSnapSum)
	prevMaps := oltifderive.PonsJSONToMaps(prevSnapPons)
	if incomplete && len(prevMaps) > 0 {
		var carryPatch map[string]any
		curMaps, carryPatch = oltifderive.PreservePonCountsOnIncomplete(prevMaps, curMaps)
		for k, v := range carryPatch {
			summary[k] = v
		}
		summary["onu_delta_alerts_skipped"] = "coleta SNMP incompleta ou truncada"
	}
	oltifderive.ApplyPonOperStatusAll(curMaps)
	pons = oltifderive.PonsMapsToAny(curMaps)
	alertSource := source
	if alertSource == "olt_refresh" {
		alertSource = "olt_refresh"
	} else {
		alertSource = "monitor_worker"
	}
	if !incomplete {
		alertthresholds.EvaluateOltOnuQuantityDeltaAlerts(ctx, pool, &s.Log, id, devDesc, host, prevMaps, curMaps, alertSource)
	}

	sb, _ := json.Marshal(summary)
	pb, _ := json.Marshal(pons)
	if _, err := pool.Exec(ctx, `
		INSERT INTO olt_snapshots (device_id, summary, pons) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET summary = excluded.summary, pons = excluded.pons, updated_at = now()
	`, id, sb, pb); err != nil {
		out.Reason = err.Error()
		return out, err
	}
	recordOLTOnuSample(ctx, pool, id, sb, pb)

	out.PonCount = len(curMaps)
	out.OK = out.PonCount > 0
	if !out.OK {
		out.Reason = "coleta por perfil não produziu segmentos PON"
		if r := strings.TrimSpace(fmt.Sprint(summary["olt_profile_exec_error"])); r != "" && r != "<nil>" {
			out.Reason = r
		}
		if note := strings.TrimSpace(fmt.Sprint(summary["onu_metrics_note"])); note != "" && note != "<nil>" {
			out.Reason = note
		}
	}
	_ = oltparse.SnapshotComputed(sb, pb)
	return out, nil
}
