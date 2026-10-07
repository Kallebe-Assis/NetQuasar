package oltcollect

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// Walk único da tabela inteira de serial (400-500 ONUs, GET-BULK de 10 linhas numa VSOL lenta)
// costuma estourar o orçamento por métrica em CollectOnuMetrics e é cortado em ordem de PON — as
// primeiras PONs completam, as últimas ficam sem serial (ver sintoma relatado: "só as 3-4
// primeiras PONs de uma OLT de 8 recebem serial"). completeMissingSerials refaz o walk só da
// sub-árvore (`base.PON`) de cada PON que ainda tem ONU sem serial plausível, com orçamento
// próprio por PON, em vez de repetir a tabela inteira — o serial quase nunca muda, então o que
// já foi lido (e preservado do snapshot anterior por CarryForwardOnuIdentity) não é refeito.

const (
	serialCompletionPerPonBudget = 25 * time.Second
	serialCompletionMinLeft      = 6 * time.Second
)

// serialCompletionTotalBudget devolve o tempo total reservado a esta fase, proporcional ao
// orçamento da coleta (1/3), com piso e teto fixos — nunca estoura a janela de coleta por muito.
func serialCompletionTotalBudget(totalBudget time.Duration) time.Duration {
	b := totalBudget / 3
	if b < 30*time.Second {
		b = 30 * time.Second
	}
	if b > 90*time.Second {
		b = 90 * time.Second
	}
	return b
}

type serialMissingPon struct {
	seg     int // primeiro segmento do sufixo (nº da PON, ou ifIndex em perfis estilo ZTE)
	missing int
}

// missingSerialSegments agrupa as ONUs sem serial plausível pelo primeiro segmento do sufixo da
// tabela de serial — o mesmo que ParsePonOnuSuffixMapped devolve como ifIndex quando existe, senão
// o nº da PON. Ordena por mais ONUs faltando primeiro (maior ganho por walk).
func missingSerialSegments(byKey map[string]*map[string]any) []serialMissingPon {
	counts := map[int]int{}
	for _, ptr := range byKey {
		if ptr == nil {
			continue
		}
		row := *ptr
		if s, _ := row["serial"].(string); IsPlausibleOnuSerial(s) {
			continue
		}
		seg := intFromAny(row["if_index"])
		if seg <= 0 {
			seg = intFromAny(row["pon"])
		}
		if seg <= 0 {
			continue
		}
		counts[seg]++
	}
	out := make([]serialMissingPon, 0, len(counts))
	for seg, n := range counts {
		out = append(out, serialMissingPon{seg: seg, missing: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].missing != out[j].missing {
			return out[i].missing > out[j].missing
		}
		return out[i].seg < out[j].seg
	})
	return out
}

// countRowsMissingSerial conta ONUs ainda sem serial plausível.
func countRowsMissingSerial(byKey map[string]*map[string]any) int {
	n := 0
	for _, ptr := range byKey {
		if ptr == nil {
			continue
		}
		if s, _ := (*ptr)["serial"].(string); !IsPlausibleOnuSerial(s) {
			n++
		}
	}
	return n
}

// completeMissingSerials preenche `serial` nas linhas de byKey que ainda não têm, consultando só
// a sub-árvore da PON de cada uma. Só atua em tabelas com sufixo `.PON.ONU` / `.ifIndex.ONU`
// (onuPonByOnu vazio) — em tabelas de sufixo único `.ONU` o primeiro segmento não é a PON e a
// sub-árvore seria outra coisa. Devolve quantas linhas ganharam serial e uma entrada de log por
// walk (mesmo formato de onu_metrics_walks).
func completeMissingSerials(
	ctx context.Context, host, community string, def OnuMetricDef,
	byKey map[string]*map[string]any, ponByIfIndex map[int]ponIfRef, onuPonByOnu map[int]int,
	budget time.Duration,
) (filled int, walks []map[string]any) {
	base := probing.NormalizeSNMPOID(def.OID)
	if base == "" || len(onuPonByOnu) > 0 || budget <= 0 {
		return 0, nil
	}
	segs := missingSerialSegments(byKey)
	if len(segs) == 0 {
		return 0, nil
	}
	deadline := time.Now().Add(budget)
	for _, sg := range segs {
		left := time.Until(deadline)
		if left < serialCompletionMinLeft {
			break
		}
		per := serialCompletionPerPonBudget
		if per > left {
			per = left
		}
		root := base + "." + strconv.Itoa(sg.seg)
		t0 := time.Now()
		vars, trunc, note := doSNMPWalk(ctx, host, community, root, per)
		got := 0
		for _, v := range vars {
			pon, onu, _, ok := ParsePonOnuSuffixMapped(base, v.OID, ponByIfIndex, onuPonByOnu)
			if !ok {
				continue
			}
			ptr := byKey[fmt.Sprintf("%d.%d", pon, onu)]
			if ptr == nil {
				continue
			}
			row := *ptr
			if cur, _ := row["serial"].(string); IsPlausibleOnuSerial(cur) {
				continue
			}
			if val := normalizeSnmpDisplayValue(v.Value); IsPlausibleOnuSerial(val) {
				row["serial"] = val
				got++
			}
		}
		filled += got
		entry := map[string]any{
			"metric": "serial_completion", "oid": root, "status": "ok",
			"var_count": len(vars), "filled": got, "missing_before": sg.missing,
			"elapsed_ms": time.Since(t0).Milliseconds(),
		}
		if len(vars) == 0 {
			entry["status"] = "empty"
		}
		if trunc {
			entry["truncated"] = true
		}
		if note != "" {
			entry["note"] = note
		}
		walks = append(walks, entry)
	}
	return filled, walks
}

func containsMetricKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}
