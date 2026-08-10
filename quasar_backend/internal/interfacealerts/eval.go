// Package interfacealerts avalia limiares e transições após snapshots IF-MIB.
// Inclui: UP→DOWN com confirmação, potência/temperatura SFP MikroTik, queda PPPoE.
package interfacealerts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/ifaceoptical"
	"github.com/netquasar/netquasar/quasar_backend/internal/ifacemeta"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snapshotwalk"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/rs/zerolog"
)

const (
	alertTypeIfaceDown         = "interface_down_transition"
	alertTypeMikrotikPPPoEDrop = "mikrotik_pppoe_drop"
	metaKeyMikrotikPPPoEDrop   = "mikrotik_pppoe_drop"
)

// Params entrada para avaliação pós-snapshot de interfaces.
type Params struct {
	DeviceID   uuid.UUID
	Host       string
	Community  string // para 2.º teste SNMP imediato de ifOperStatus
	DeviceDesc string
	Category   string
	Brand      string
	Model      string
	Source     string
	PrevJSON   []byte // nil ou vazio = sem comparação de transição
	OlderJSON  []byte // snapshot anterior a Prev (confirmação em 2 ciclos)
	CurrJSON   []byte
}

// EvaluateAfterSnapshot aplica limiares SFP (SNMP MikroTik + Telnet Switch/MikroTik) e transições UP→DOWN.
func EvaluateAfterSnapshot(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, p Params) {
	if pool == nil || len(p.CurrJSON) == 0 {
		return
	}
	host := strings.TrimSpace(p.Host)
	desc := strings.TrimSpace(p.DeviceDesc)
	currVars := snapshotwalk.VarsFromJSON(p.CurrJSON)

	mk := isMikrotik(p.Category, p.Brand, p.Model, p.DeviceDesc)
	sw := strings.EqualFold(strings.TrimSpace(p.Category), "switch")
	customs := ifacemeta.LoadCustomDescriptionsByIndex(ctx, pool, p.DeviceID)
	if mk || sw {
		evaluateOpticalSFP(ctx, pool, log, p.DeviceID, desc, host, p.CurrJSON, currVars, customs)
	}

	if len(p.PrevJSON) == 0 {
		return
	}
	prevVars := snapshotwalk.VarsFromJSON(p.PrevJSON)
	var olderVars []probing.SNMPVar
	if len(p.OlderJSON) > 0 {
		olderVars = snapshotwalk.VarsFromJSON(p.OlderJSON)
	}
	evaluateInterfaceDownTransitions(ctx, pool, log, p.DeviceID, desc, host, p.Community, p.Category, p.Source, mk,
		olderVars, prevVars, currVars, customs, snapshotwalk.Truncated(p.CurrJSON))
}

func isMikrotik(category, brand, model, description string) bool {
	if strings.EqualFold(strings.TrimSpace(category), "switch") {
		return false
	}
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") ||
		strings.Contains(hay, "ccr") || strings.Contains(hay, "crs") || strings.Contains(hay, "rb") ||
		strings.Contains(hay, "chr")
}

func isPPPoEIfaceName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(n, "pppoe") || strings.HasPrefix(n, "<pppoe")
}

func ifaceDisplayName(r snmpifparse.IfRow, customs map[int]string) (baseName, label string) {
	baseName = snmpifparse.PreferIfaceName(r.IfName, r.DisplayName, r.Descr, r.IfIndex)
	custom := customs[r.IfIndex]
	label = snmpifparse.FormatAlertIfaceLabel(baseName, r.IfAlias, custom)
	if label == "" {
		label = fmt.Sprintf("if%d", r.IfIndex)
	}
	return baseName, label
}

func evaluateOpticalSFP(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, devDesc, host string, currJSON []byte, vars []probing.SNMPVar, customs map[int]string,
) {
	ifRows := snmpifparse.BuildIfTable(vars)
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, vars)
	metaPorts := ifaceoptical.ParseMetaFromWalkJSON(currJSON)
	if len(metaPorts) > 0 {
		metaPorts = ifaceoptical.ResolveIfIndexes(metaPorts, ifRows)
		optMap = ifaceoptical.MergeIntoOpticalMap(optMap, metaPorts)
	}
	if len(ifRows) == 0 && len(optMap) == 0 {
		return
	}
	sfpEval := make([]alertthresholds.SfpInterfaceRow, 0, len(ifRows)+len(metaPorts))
	seen := map[int]struct{}{}
	for _, r := range ifRows {
		op := optMap[r.IfIndex]
		baseName := snmpifparse.PreferIfaceName(r.IfName, r.DisplayName, r.Descr, r.IfIndex)
		disp := snmpifparse.FormatAlertIfaceLabel(baseName, r.IfAlias, customs[r.IfIndex])
		if disp == "" {
			disp = fmt.Sprintf("if%d", r.IfIndex)
		}
		sfp := snmpmikrotik.IsSfpPort(r.DisplayName, r.Descr, op)
		if !sfp && (op.TxDBm != nil || op.RxDBm != nil) {
			sfp = true
		}
		sfpEval = append(sfpEval, alertthresholds.SfpInterfaceRow{
			IfIndex:           r.IfIndex,
			DisplayName:       disp,
			IfName:            baseName,
			IfAlias:           strings.TrimSpace(r.IfAlias),
			CustomDescription: customs[r.IfIndex],
			Sfp:               sfp,
			TxDBm:             copyFloatPtr(op.TxDBm),
			RxDBm:             copyFloatPtr(op.RxDBm),
			TemperatureC:      copyFloatPtr(op.TemperatureC),
		})
		seen[r.IfIndex] = struct{}{}
	}
	for _, p := range metaPorts {
		if p.IfIndex <= 0 {
			continue
		}
		if _, ok := seen[p.IfIndex]; ok {
			continue
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = fmt.Sprintf("if%d", p.IfIndex)
		}
		disp := snmpifparse.FormatAlertIfaceLabel(name, "", customs[p.IfIndex])
		sfpEval = append(sfpEval, alertthresholds.SfpInterfaceRow{
			IfIndex:           p.IfIndex,
			DisplayName:       disp,
			IfName:            name,
			CustomDescription: customs[p.IfIndex],
			Sfp:               true,
			TxDBm:             copyFloatPtr(p.TxDBm),
			RxDBm:             copyFloatPtr(p.RxDBm),
			TemperatureC:      copyFloatPtr(p.TemperatureC),
		})
	}
	alertthresholds.EvaluateMikrotikSFPAfterSnapshot(ctx, pool, log, deviceID, devDesc, host, sfpEval)
}

func evaluateInterfaceDownTransitions(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, devDesc, host, community, category, source string, mikrotik bool,
	olderVars, prevVars, currVars []probing.SNMPVar, customs map[int]string, currTruncated bool,
) {
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "iface_down_count", category)
	prevRows := snmpifparse.BuildIfTable(prevVars)
	currRows := snmpifparse.BuildIfTable(currVars)
	olderRows := snmpifparse.BuildIfTable(olderVars)
	prevBy := map[int]snmpifparse.IfRow{}
	for _, r := range prevRows {
		prevBy[r.IfIndex] = r
	}
	currBy := map[int]snmpifparse.IfRow{}
	for _, r := range currRows {
		currBy[r.IfIndex] = r
	}
	olderBy := map[int]snmpifparse.IfRow{}
	for _, r := range olderRows {
		olderBy[r.IfIndex] = r
	}
	hasOlder := len(olderBy) > 0
	src := strings.TrimSpace(source)
	if src == "" {
		src = "interface_snapshot"
	}

	skipDownAlerts := currTruncated || suspectMassInterfaceDrop(prevBy, currBy)
	if skipDownAlerts {
		reason := "mass_drop_suspect"
		if currTruncated {
			reason = "walk_truncated"
		}
		logIfaceDownSkip(log, deviceID.String(), reason, map[string]any{
			"prev_ifaces": len(prevBy), "curr_ifaces": len(currBy), "truncated": currTruncated,
		})
	}

	pppoeDrops := 0
	if mikrotik && !skipDownAlerts {
		// UP→DOWN ainda presente
		for _, r := range currRows {
			base, _ := ifaceDisplayName(r, customs)
			if !isPPPoEIfaceName(base) && !isPPPoEIfaceName(r.Descr) && !isPPPoEIfaceName(r.DisplayName) {
				continue
			}
			if !ifaceIsDownKnown(r) {
				continue
			}
			p, hasPrev := prevBy[r.IfIndex]
			if !hasPrev || !ifaceIsUp(p) {
				continue
			}
			pppoeDrops++
		}
		// Sessões que desapareceram do IF-MIB (comum no RouterOS)
		for _, p := range prevRows {
			base, _ := ifaceDisplayName(p, customs)
			if !isPPPoEIfaceName(base) && !isPPPoEIfaceName(p.Descr) && !isPPPoEIfaceName(p.DisplayName) {
				continue
			}
			if !ifaceIsUp(p) {
				continue
			}
			if _, stillThere := currBy[p.IfIndex]; !stillThere {
				pppoeDrops++
			}
		}
		evaluateMikrotikPPPoEDropBatch(ctx, pool, log, deviceID, devDesc, host, src, category, pppoeDrops)
	}

	if !ok {
		return
	}

	for _, r := range currRows {
		baseName, label := ifaceDisplayName(r, customs)
		if mikrotik && (isPPPoEIfaceName(baseName) || isPPPoEIfaceName(r.Descr) || isPPPoEIfaceName(r.DisplayName)) {
			// PPPoE MikroTik: só o alerta agregado (não um por sessão).
			continue
		}
		p, hasPrev := prevBy[r.IfIndex]
		if !hasPrev {
			continue
		}
		key := fmt.Sprintf("ifdown:%d", r.IfIndex)

		// Resolução: só fecha com oper UP conhecido (não com status em falta).
		if ifaceIsUp(r) {
			_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
				DeviceID: deviceID, AlertType: alertTypeIfaceDown,
				Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
				Resolved: map[string]any{
					"resolved": "interface_up", "source": src, "key": key,
				},
			})
			continue
		}

		if skipDownAlerts {
			continue
		}
		if !ifaceIsDownKnown(r) {
			// Walk parcial sem ifOperStatus — não tratar como DOWN.
			continue
		}

		older, hasOlderRow := olderBy[r.IfIndex]
		confirmed := shouldConfirmIfaceDownAfterStreak(older, p, r, hasOlder && hasOlderRow)

		if !confirmed {
			// 1.ª observação UP→DOWN: 2.º teste SNMP imediato só para descartar falso positivo
			// (walk com lixo). Mesmo confirmando DOWN via GET, espera a 2.ª coleta (como latência).
			if ifaceIsUp(p) {
				downOK, known, operLabel := confirmIfaceOperDown(ctx, host, community, r.IfIndex, 4*time.Second)
				if known && !downOK {
					logIfaceDownSkip(log, deviceID.String(), "snmp_reconfirm_up", map[string]any{
						"if_index": r.IfIndex, "oper": operLabel,
					})
					continue
				}
				logIfaceDownSkip(log, deviceID.String(), "awaiting_second_cycle", map[string]any{
					"if_index": r.IfIndex, "snmp_reconfirm_down": known && downOK, "oper": operLabel,
				})
			}
			continue
		}

		// Ciclo de confirmação: novo GET antes de abrir o alerta.
		if downOK, known, operLabel := confirmIfaceOperDown(ctx, host, community, r.IfIndex, 4*time.Second); known && !downOK {
			logIfaceDownSkip(log, deviceID.String(), "confirm_cycle_snmp_up", map[string]any{
				"if_index": r.IfIndex, "oper": operLabel,
			})
			continue
		}

		sev := alertthresholds.EvalMetricSeverity(1, th)
		if sev == "ok" {
			continue
		}
		custom := customs[r.IfIndex]
		msg := fmt.Sprintf("%s (%s): interface %s mudou de UP para DOWN (confirmado em %d coletas).",
			devDesc, host, label, MinConsecutiveIfaceDown)
		meta := alertnotify.WithStatusTransition(map[string]any{
			"source":             src,
			"if_index":           r.IfIndex,
			"display_name":       label,
			"if_name":            baseName,
			"if_alias":           strings.TrimSpace(r.IfAlias),
			"custom_description": custom,
			"key":                key,
			"confirmed_cycles":   MinConsecutiveIfaceDown,
		}, "interface_up", "interface_down", nil)
		res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
			DeviceID: deviceID, Severity: sev, AlertType: alertTypeIfaceDown,
			Message: msg, IP: host, DeviceName: devDesc, Meta: meta,
			Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
		}, &alertstore.NotifyCreate{
			Log: log, Level: strings.ToUpper(sev), Headline: "Interface DOWN (mudança de estado)",
		})
		if err != nil && log != nil {
			log.Error().Err(err).Str("device", deviceID.String()).Msg("interface_down_transition")
		} else if res.Created && log != nil {
			log.Warn().Str("device", deviceID.String()).Int("if_index", r.IfIndex).Msg("alerta: interface UP→DOWN confirmado")
		}
	}
}

func evaluateMikrotikPPPoEDropBatch(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, devDesc, host, source, category string, dropCount int,
) {
	th, metricLabel, enabled := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "mikrotik_pppoe_drop_count", category)
	if !enabled {
		closeMikrotikPPPoEDrop(ctx, pool, log, deviceID)
		return
	}
	if dropCount <= 0 {
		closeMikrotikPPPoEDrop(ctx, pool, log, deviceID)
		return
	}
	sev := alertthresholds.EvalMetricSeverity(float64(dropCount), th)
	if sev == "ok" {
		closeMikrotikPPPoEDrop(ctx, pool, log, deviceID)
		return
	}
	if strings.TrimSpace(metricLabel) == "" {
		metricLabel = "Sessões PPPoE"
	}
	label := strings.TrimSpace(devDesc)
	if label == "" {
		label = strings.TrimSpace(host)
	}
	msg := fmt.Sprintf("%s (%s): %d sessões PPPoE desconectaram entre coletas (limiar %s).",
		label, host, dropCount, metricLabel)
	meta := alertnotify.WithStatusTransition(map[string]any{
		"source":     source,
		"key":        metaKeyMikrotikPPPoEDrop,
		"drop_count": dropCount,
		"metric_id":  "mikrotik_pppoe_drop_count",
	}, "pppoe_stable", "pppoe_drop_batch", nil)
	res, err := alertstore.OpenOrUpdate(ctx, pool, alertstore.OpenSpec{
		DeviceID: deviceID, Severity: sev, AlertType: alertTypeMikrotikPPPoEDrop,
		Message: msg, IP: host, DeviceName: devDesc, Meta: meta,
		Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: metaKeyMikrotikPPPoEDrop},
	}, &alertstore.NotifyCreate{
		Log: log, Level: strings.ToUpper(sev), Headline: "Queda de sessões PPPoE MikroTik",
	})
	if err != nil && log != nil {
		log.Error().Err(err).Str("device", deviceID.String()).Msg("mikrotik_pppoe_drop")
	} else if res.Created && log != nil {
		log.Warn().Str("device", deviceID.String()).Int("drops", dropCount).Msg("alerta: queda PPPoE MikroTik em lote")
	}
}

func closeMikrotikPPPoEDrop(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID) {
	_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
		DeviceID: deviceID, AlertType: alertTypeMikrotikPPPoEDrop,
		Match:    alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: metaKeyMikrotikPPPoEDrop},
		Resolved: map[string]any{"resolved": "pppoe_stable", "key": metaKeyMikrotikPPPoEDrop},
	})
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
