package interfacealerts

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertnotify"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertstore"
	"github.com/netquasar/netquasar/quasar_backend/internal/alertthresholds"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snapshotwalk"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/rs/zerolog"
)

const alertTypeIfaceDown = "interface_down_transition"

// Params entrada para avaliação pós-snapshot de interfaces.
type Params struct {
	DeviceID    uuid.UUID
	Host        string
	DeviceDesc  string
	Category    string
	Brand       string
	Model       string
	Source      string
	PrevJSON    []byte // nil ou vazio = sem comparação de transição
	CurrJSON    []byte
}

// EvaluateAfterSnapshot aplica limiares SFP MikroTik e transições UP→DOWN de interface.
func EvaluateAfterSnapshot(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, p Params) {
	if pool == nil || len(p.CurrJSON) == 0 {
		return
	}
	host := strings.TrimSpace(p.Host)
	desc := strings.TrimSpace(p.DeviceDesc)
	currVars := snapshotwalk.VarsFromJSON(p.CurrJSON)

	if isMikrotik(p.Category, p.Brand, p.Model, p.DeviceDesc) {
		evaluateMikrotikSFP(ctx, pool, log, p.DeviceID, desc, host, currVars)
	}

	if len(p.PrevJSON) == 0 {
		return
	}
	prevVars := snapshotwalk.VarsFromJSON(p.PrevJSON)
	evaluateInterfaceDownTransitions(ctx, pool, log, p.DeviceID, desc, host, p.Category, p.Source, prevVars, currVars)
}

func isMikrotik(category, brand, model, description string) bool {
	hay := strings.ToLower(strings.TrimSpace(category) + " " + strings.TrimSpace(brand) + " " +
		strings.TrimSpace(model) + " " + strings.TrimSpace(description))
	return strings.Contains(hay, "mikrotik") || strings.Contains(hay, "routeros") ||
		strings.Contains(hay, "ccr") || strings.Contains(hay, "crs") || strings.Contains(hay, "rb") ||
		strings.Contains(hay, "chr")
}

func evaluateMikrotikSFP(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, devDesc, host string, vars []probing.SNMPVar,
) {
	if len(vars) == 0 {
		return
	}
	ifRows := snmpifparse.BuildIfTable(vars)
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, vars)
	sfpEval := make([]alertthresholds.SfpInterfaceRow, 0, len(ifRows))
	for _, r := range ifRows {
		op := optMap[r.IfIndex]
		disp := strings.TrimSpace(r.DisplayName)
		if disp == "" {
			disp = fmt.Sprintf("if%d", r.IfIndex)
		}
		sfpEval = append(sfpEval, alertthresholds.SfpInterfaceRow{
			IfIndex:     r.IfIndex,
			DisplayName: disp,
			Sfp:         snmpmikrotik.IsSfpPort(r.DisplayName, r.Descr, op),
			TxDBm:       copyFloatPtr(op.TxDBm),
			RxDBm:       copyFloatPtr(op.RxDBm),
		})
	}
	alertthresholds.EvaluateMikrotikSFPAfterSnapshot(ctx, pool, log, deviceID, devDesc, host, sfpEval)
}

func evaluateInterfaceDownTransitions(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger,
	deviceID uuid.UUID, devDesc, host, category, source string,
	prevVars, currVars []probing.SNMPVar,
) {
	th, _, ok := alertthresholds.LoadGlobalGteMetricForDevice(ctx, pool, "iface_down_count", category)
	if !ok {
		return
	}
	prevRows := snmpifparse.BuildIfTable(prevVars)
	currRows := snmpifparse.BuildIfTable(currVars)
	prevBy := map[int]snmpifparse.IfRow{}
	for _, r := range prevRows {
		prevBy[r.IfIndex] = r
	}
	src := strings.TrimSpace(source)
	if src == "" {
		src = "interface_snapshot"
	}
	for _, r := range currRows {
		p, hasPrev := prevBy[r.IfIndex]
		if !hasPrev {
			continue
		}
		prevUp := snmpifparse.OperStatusLabel(p.OperStatus) == "up"
		currUp := snmpifparse.OperStatusLabel(r.OperStatus) == "up"
		key := fmt.Sprintf("ifdown:%d", r.IfIndex)
		if prevUp && !currUp {
			sev := alertthresholds.EvalMetricSeverity(1, th)
			if sev == "ok" {
				continue
			}
			name := strings.TrimSpace(r.DisplayName)
			if name == "" {
				name = fmt.Sprintf("if%d", r.IfIndex)
			}
			msg := fmt.Sprintf("%s (%s): interface %s mudou de UP para DOWN.", devDesc, host, name)
			meta := alertnotify.WithStatusTransition(map[string]any{
				"source":       src,
				"if_index":     r.IfIndex,
				"display_name": name,
				"if_name":      name,
				"key":          key,
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
				log.Warn().Str("device", deviceID.String()).Int("if_index", r.IfIndex).Msg("alerta: interface UP→DOWN")
			}
		}
		if currUp {
			_, _, _ = alertstore.Close(ctx, pool, log, alertstore.CloseSpec{
				DeviceID: deviceID, AlertType: alertTypeIfaceDown,
				Match: alertstore.Match{Kind: alertstore.MatchMetaKey, MetaKey: key},
				Resolved: map[string]any{
					"resolved": "interface_up", "source": src, "key": key,
				},
			})
		}
	}
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
