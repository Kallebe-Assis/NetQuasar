package oltcollect

import (
	"context"
	"fmt"
	"strings"
)

// Linhas de ONU sem número de série.
//
// Uma ONU sem serial não serve para nada na operação (não dá para vincular a cliente, nem
// reconhecer, nem alertar por serial) e, na prática, são entradas «fantasma» das tabelas de
// potência/status da OLT (slot que respondeu mas nunca foi provisionado, ou leitura residual). Por
// isso, depois de tentar de tudo (walk do serial, completar por PON, e herdar do snapshot anterior
// pelo par pon+onu — o serial é identidade estável e continua válido offline), a linha que
// continua sem serial plausível NÃO entra no snapshot.
//
// Só se descarta quando a leitura do serial foi confiável nesta volta (o walk trouxe linhas e não
// foi cortado). Se o walk do serial falhou ou veio truncado, as linhas ficam como estão — uma
// volta ruim não pode apagar ONUs reais.

type prevOnuRowsKey struct{}

// WithPrevOnuRows entrega ao coletor as linhas ONU do snapshot anterior (para herdar o serial).
func WithPrevOnuRows(ctx context.Context, rows []map[string]any) context.Context {
	if len(rows) == 0 {
		return ctx
	}
	return context.WithValue(ctx, prevOnuRowsKey{}, rows)
}

func prevOnuRowsFrom(ctx context.Context) []map[string]any {
	rows, _ := ctx.Value(prevOnuRowsKey{}).([]map[string]any)
	return rows
}

// serialWalkReliable diz se o walk do serial desta volta foi confiável (trouxe linhas, sem corte).
func serialWalkReliable(walkLog []map[string]any) bool {
	for _, e := range walkLog {
		if e["metric"] != MetricSerial {
			continue
		}
		if trunc, _ := e["truncated"].(bool); trunc {
			return false
		}
		if intFromAny(e["matched_rows"]) > 0 {
			return true
		}
	}
	return false
}

// pruneOnuRowsWithoutSerial herda o serial do snapshot anterior e, se `reliable`, remove as
// linhas que continuam sem serial plausível. Devolve as linhas mantidas e a descrição das removidas.
func pruneOnuRowsWithoutSerial(rows, prev []map[string]any, reliable bool) (kept []map[string]any, dropped []string) {
	if len(prev) > 0 {
		sum := map[string]any{"vsol_onu_rows": toAnySlice(rows)}
		CarryForwardOnuIdentity(sum, prev)
	}
	kept = make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if s, _ := r["serial"].(string); IsPlausibleOnuSerial(s) || !reliable {
			kept = append(kept, r)
			continue
		}
		dropped = append(dropped, fmt.Sprintf("%d/%d", intFromRow(r, "pon"), intFromRow(r, "onu")))
	}
	return kept, dropped
}

func toAnySlice(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

// isLiveTelnetField — campos do relatório telnet que são leitura ao vivo (potência, temperatura,
// tensão, corrente, bias, distância, tempo online). Identidade (SN, nome, modelo, HW/SW, config) fica.
func isLiveTelnetField(k string) bool {
	l := strings.ToLower(strings.TrimSpace(k))
	for _, p := range []string{"rx", "tx"} {
		if l == p || strings.HasPrefix(l, p+" ") || strings.HasPrefix(l, p+"_") || strings.HasPrefix(l, p+"-") {
			return true
		}
	}
	for _, sub := range []string{"pot", "temp", "volt", "corrente", "current", "bias", "dist", "uptime", "online time", "dbm", "optical", "óptic", "optic"} {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
