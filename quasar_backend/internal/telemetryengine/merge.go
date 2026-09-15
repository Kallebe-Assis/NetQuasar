package telemetryengine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// telemetryMergeLookback e telemetryMergeMaxRows — janela usada para "completar" a amostra mais
// recente de telemetry_samples com campos de amostras um pouco mais antigas. Existe porque o
// ciclo rápido de saúde (CollectHealthAndStore, a cada telemetry_seconds — ex.: 3 min) grava uma
// amostra só com CPU/memória/temperatura/uptime, sem disco/óptica/PPPoE; essas secções mais
// pesadas só vêm de uma coleta completa (botão "Coletar agora", ou o ciclo de refresco completo
// periódico — ver mikrotikFullRefresh). Como telemetry_samples é sempre INSERT (nunca upsert —
// necessário para os gráficos de histórico), sem isto a amostra "mais recente" voltava a ficar
// sem esses campos assim que o próximo tick de saúde gravasse, e a tela dava a impressão de que
// os dados "somem" pouco depois de aparecerem.
const (
	telemetryMergeLookback = 30 * time.Minute
	telemetryMergeMaxRows  = 10
)

// LoadLatestMergedTelemetry devolve a amostra mais recente de telemetry_samples, com quaisquer
// campos ausentes preenchidos a partir de amostras um pouco mais antigas (mesmo dispositivo,
// últimos telemetryMergeLookback) que os tinham. Nunca substitui um valor já presente na amostra
// mais recente — só preenche lacunas. O timestamp devolvido é sempre o da amostra mais recente.
func LoadLatestMergedTelemetry(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (collectedAt time.Time, merged json.RawMessage, mergedFrom []time.Time, err error) {
	// Corte calculado em Go e passado como timestamp (não "$2 || ' seconds'"::interval) — essa
	// forma de concatenar texto+interval confundia a inferência de tipo do pgx para o parâmetro
	// (erro visto ao vivo: "unable to encode 1800 into text format for text (OID 25)"), fazendo
	// este endpoint falhar por completo para alguns equipamentos.
	cutoff := time.Now().Add(-telemetryMergeLookback)
	rows, qerr := pool.Query(ctx, `
		SELECT collected_at, metrics::text FROM telemetry_samples
		WHERE device_id=$1 AND collected_at >= $2
		ORDER BY collected_at DESC LIMIT $3
	`, deviceID, cutoff, telemetryMergeMaxRows)
	if qerr != nil {
		err = qerr
		return
	}
	defer rows.Close()

	type sample struct {
		at   time.Time
		raw  string
		doc  map[string]any
	}
	var samples []sample
	for rows.Next() {
		var s sample
		if serr := rows.Scan(&s.at, &s.raw); serr != nil {
			err = serr
			return
		}
		samples = append(samples, s)
	}
	if err = rows.Err(); err != nil {
		return
	}
	if len(samples) == 0 {
		// Nada dentro da janela — cai para a última amostra existente (pode ser antiga), sem merge.
		var s sample
		if ferr := pool.QueryRow(ctx, `
			SELECT collected_at, metrics::text FROM telemetry_samples
			WHERE device_id=$1 ORDER BY collected_at DESC LIMIT 1
		`, deviceID).Scan(&s.at, &s.raw); ferr != nil {
			err = ferr
			return
		}
		collectedAt = s.at
		merged = json.RawMessage(s.raw)
		return
	}

	collectedAt = samples[0].at
	base := map[string]any{}
	if uerr := json.Unmarshal([]byte(samples[0].raw), &base); uerr != nil {
		// Amostra mais recente ilegível — devolve-a crua em vez de falhar (mesmo comportamento de antes).
		collectedAt = samples[0].at
		merged = json.RawMessage(samples[0].raw)
		return
	}

	for _, s := range samples[1:] {
		var cand map[string]any
		if uerr := json.Unmarshal([]byte(s.raw), &cand); uerr != nil {
			continue
		}
		if deepMergeMissing(base, cand) {
			mergedFrom = append(mergedFrom, s.at)
		}
	}

	b, merr := json.Marshal(base)
	if merr != nil {
		err = merr
		return
	}
	merged = json.RawMessage(b)
	return
}

// deepMergeMissing preenche em dst quaisquer chaves de src ausentes em dst; quando a mesma
// chave existe nos dois como objeto, desce recursivamente. Nunca sobrescreve um valor já
// presente em dst. Devolve true se alguma chave foi copiada de src.
func deepMergeMissing(dst, src map[string]any) bool {
	changed := false
	for k, sv := range src {
		dv, ok := dst[k]
		if !ok {
			dst[k] = sv
			changed = true
			continue
		}
		dm, dIsMap := dv.(map[string]any)
		sm, sIsMap := sv.(map[string]any)
		if dIsMap && sIsMap {
			if deepMergeMissing(dm, sm) {
				changed = true
			}
		}
	}
	return changed
}
