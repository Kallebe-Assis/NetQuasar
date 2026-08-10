package interfacealerts

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/rs/zerolog"
)

// MinConsecutiveIfaceDown confirma DOWN em 2 coletas consecutivas (como latência alta).
const MinConsecutiveIfaceDown = 2

// massDropMinPrevUp e massDropRatio: queda em massa sugere walk incompleto — não alarmar.
const (
	massDropMinPrevUp = 5
	massDropRatio     = 0.5
)

func ifaceIsUp(r snmpifparse.IfRow) bool {
	return snmpifparse.HasKnownOperStatus(r.OperStatus) && snmpifparse.IsOperUp(r.OperStatus)
}

func ifaceIsDownKnown(r snmpifparse.IfRow) bool {
	return snmpifparse.HasKnownOperStatus(r.OperStatus) && !snmpifparse.IsOperUp(r.OperStatus)
}

// suspectMassInterfaceDrop detecta UP→DOWN (ou DOWN sem status conhecido) em massa vs. snapshot anterior.
func suspectMassInterfaceDrop(prevBy, currBy map[int]snmpifparse.IfRow) bool {
	prevUp := 0
	lostOrDown := 0
	for idx, p := range prevBy {
		if !ifaceIsUp(p) {
			continue
		}
		prevUp++
		c, ok := currBy[idx]
		if !ok || !snmpifparse.HasKnownOperStatus(c.OperStatus) || !ifaceIsUp(c) {
			lostOrDown++
		}
	}
	if prevUp < massDropMinPrevUp {
		return false
	}
	return float64(lostOrDown) >= float64(prevUp)*massDropRatio
}

// shouldConfirmIfaceDownAfterStreak exige 2 leituras DOWN consecutivas após UP (older→prev→curr).
// Sem snapshot older: ainda não confirma (espera o próximo ciclo), salvo reconfirmação SNMP imediata.
func shouldConfirmIfaceDownAfterStreak(older, prev, curr snmpifparse.IfRow, hasOlder bool) bool {
	if !ifaceIsDownKnown(curr) {
		return false
	}
	if hasOlder {
		return ifaceIsUp(older) && ifaceIsDownKnown(prev) && ifaceIsDownKnown(curr)
	}
	// Sem older: transição UP→DOWN nesta coleta — só confirma com 2.º teste SNMP (feito à parte).
	return false
}

// confirmIfaceOperDown faz GET SNMP imediato de ifOperStatus (2.º teste no mesmo ciclo).
// Devolve: confirmedDown, known (leu valor válido), operLabel.
func confirmIfaceOperDown(ctx context.Context, host, community string, ifIndex int, timeout time.Duration) (confirmedDown, known bool, operLabel string) {
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if host == "" || community == "" || ifIndex <= 0 {
		return false, false, ""
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	oid := fmt.Sprintf("1.3.6.1.2.1.2.2.1.8.%d", ifIndex)
	res := probing.SNMPGet(ctx, probing.SNMPGetParams{
		Host: host, Port: 161, Community: community, Version: "2c",
		OIDs: []string{oid}, Timeout: timeout, Retries: 0,
	})
	if !res.OK || len(res.Vars) == 0 {
		return false, false, ""
	}
	raw := strings.TrimSpace(res.Vars[0].Value)
	n, err := strconv.Atoi(raw)
	if err != nil || !snmpifparse.HasKnownOperStatus(n) {
		return false, false, ""
	}
	label := snmpifparse.OperStatusLabel(n)
	return !snmpifparse.IsOperUp(n), true, label
}

func logIfaceDownSkip(log *zerolog.Logger, device, reason string, extra map[string]any) {
	if log == nil {
		return
	}
	e := log.Info().Str("component", "interface_alerts").Str("device", device).Str("reason", reason)
	for k, v := range extra {
		e = e.Interface(k, v)
	}
	e.Msg("interface DOWN: avaliação adiada/ignorada")
}
