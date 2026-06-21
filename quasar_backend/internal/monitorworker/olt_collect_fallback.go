package monitorworker

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// tryPeriodicOltCollectFallback tenta coleta alternativa quando o perfil principal não produz PONs.
func tryPeriodicOltCollectFallback(
	ctx context.Context,
	pool *pgxpool.Pool,
	deviceID uuid.UUID,
	host, community, brand string,
	profile oltcollect.Profile,
	budget time.Duration,
	summary map[string]any,
) ([]map[string]any, map[string]any) {
	_ = summary
	if budget < 20*time.Second {
		return nil, nil
	}
	bl := strings.ToLower(strings.TrimSpace(brand))
	if !strings.Contains(bl, "vsol") {
		return nil, nil
	}

	oid := strings.TrimSpace(profile.OnuOnlineOID)
	if oid == "" {
		oid = vsolparse.DefaultVSOLOnuWalkOID
	}
	walkBudget := budget
	if walkBudget > 98*time.Second {
		walkBudget = 98 * time.Second
	}
	wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), walkBudget)
	sum, pons, _, _, _, err := vsolparse.WalkOnuTable(wctx, host, community, oid, walkBudget)
	wcancel()
	if err == nil && len(pons) > 0 {
		sum["olt_collect_fallback"] = "vsol_onu_snmp_walk"
		return pons, sum
	}

	if fbPons, fbSum := tryVsolIfMibCollectFallback(ctx, pool, deviceID, host, community, budget); len(fbPons) > 0 {
		return fbPons, fbSum
	}
	return nil, nil
}

func tryVsolIfMibCollectFallback(
	ctx context.Context,
	pool *pgxpool.Pool,
	deviceID uuid.UUID,
	host, community string,
	budget time.Duration,
) ([]map[string]any, map[string]any) {
	if pool == nil || deviceID == uuid.Nil || budget < 30*time.Second {
		return nil, nil
	}
	st := &oltWorkerExecState{
		Pool: pool, DeviceID: deviceID, Host: host, Community: community,
		Summary: map[string]any{},
	}
	ifBudget := budget / 3
	if ifBudget > 55*time.Second {
		ifBudget = 55 * time.Second
	}
	if ifBudget < 20*time.Second {
		ifBudget = 20 * time.Second
	}
	pons, meta, refs, ok := oltWorkerVsolPonFromIfMIB(ctx, st, ifBudget, true)
	if !ok || len(refs) == 0 {
		pons, meta, refs, ok = oltWorkerVsolPonFromIfMIB(ctx, st, ifBudget, false)
	}
	if !ok || len(pons) == 0 {
		return nil, nil
	}
	sum := map[string]any{"olt_collect_fallback": "vsol_if_mib_snapshot"}
	for k, v := range meta {
		sum[k] = v
	}
	if len(refs) == 0 {
		return pons, sum
	}

	left := budget - ifBudget
	if dl, ok := ctx.Deadline(); ok {
		if l := time.Until(dl) - 2*time.Second; l > 0 && l < left {
			left = l
		}
	}
	mibTO := vsolparse.CollectTimeout(len(refs), false)
	if mibTO > left {
		mibTO = left
	}
	if mibTO >= 20*time.Second {
		mibCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mibTO)
		coll := vsolparse.CollectOLT(mibCtx, host, community, refs, false)
		cancel()
		onBy, offBy := vsolparse.OnlineOfflineByPon(coll.Vars)
		if vsolparse.OnlineStepComplete(coll) {
			pons = vsolparse.AttachOnlineOfflineToIfPons(pons, onBy, offBy)
			sum["vsol_online_complete"] = true
		} else {
			sum["vsol_online_incomplete"] = true
		}
		sum["vsol_snmp_var_count"] = len(coll.Vars)
	}
	sum["vsol_onu_refs_count"] = len(refs)
	return pons, sum
}
