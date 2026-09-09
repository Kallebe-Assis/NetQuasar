// Package oltsamples grava o histórico de contagens de ONU por OLT (olt_onu_samples) — a
// única fonte de dados para os gráficos "variação ao longo do dia/período" (ver
// GET /api/v1/olt/reports/history, internal/api/handlers_olt_reports.go). Vive num pacote à
// parte (não em internal/api) porque tanto os handlers HTTP (refresh manual de uma OLT)
// quanto o internal/monitorworker (coleta periódica automática) precisam de chamar isto —
// e, até esta correção, só o refresh manual chamava: a coleta periódica (que é a maior parte
// das amostras ao longo do dia) actualizava olt_snapshots (o "estado atual") mas nunca
// gravava em olt_onu_samples, por isso o gráfico "últimas 24h" só tinha 1 ponto (o do último
// refresh manual, não das várias coletas automáticas do dia).
package oltsamples

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltparse"
)

// RecordSample grava uma amostra de onu_total/online/offline para o device — chamar sempre
// que um snapshot OLT (summary+pons) for calculado/persistido, seja por refresh manual ou
// pela coleta periódica do monitor worker.
func RecordSample(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, summaryJSON, ponsJSON []byte) {
	if pool == nil {
		return
	}
	c := oltparse.SnapshotComputed(summaryJSON, ponsJSON)
	total := intVal(c, "onu_total_sum")
	online := intVal(c, "onu_online_sum")
	offline := intVal(c, "onu_offline_sum")
	if total == 0 && online == 0 && offline == 0 {
		return
	}
	_, _ = pool.Exec(ctx, `
		INSERT INTO olt_onu_samples (device_id, onu_total, onu_online, onu_offline)
		VALUES ($1, $2, $3, $4)
	`, deviceID, total, online, offline)
}

// RecordOnuHistory grava uma linha por ONU (pon+onu) — histórico para o botão "Histórico" (3
// pontinhos) na aba de ONUs, ver GET /api/v1/olt/devices/{id}/onu-history. Chamado nos mesmos
// pontos que RecordSample (ver comentário do pacote) — mesma summaryJSON já usada lá, só que
// aqui interessa a chave "vsol_onu_rows" (a mesma tabela de ONUs devolvida à UI, ver
// internal/api/handlers_olt_vsol_collect.go). Poda para as últimas 10 colectas por ONU a cada
// gravação — não fica a crescer sem fim.
func RecordOnuHistory(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, summaryJSON []byte) {
	if pool == nil || len(summaryJSON) == 0 {
		return
	}
	var sum map[string]any
	if err := json.Unmarshal(summaryJSON, &sum); err != nil {
		return
	}
	rowsRaw, ok := sum["vsol_onu_rows"].([]any)
	if !ok || len(rowsRaw) == 0 {
		return
	}
	batch := &pgx.Batch{}
	n := 0
	for _, it := range rowsRaw {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		pon := intVal(m, "pon")
		onu := intVal(m, "onu")
		if pon < 1 || onu < 1 {
			continue
		}
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		batch.Queue(`INSERT INTO olt_onu_history (device_id, pon, onu, row) VALUES ($1, $2, $3, $4::jsonb)`,
			deviceID, pon, onu, string(b))
		n++
	}
	if n == 0 {
		return
	}
	br := pool.SendBatch(ctx, batch)
	for i := 0; i < n; i++ {
		_, _ = br.Exec()
	}
	_ = br.Close()
	_, _ = pool.Exec(ctx, `
		DELETE FROM olt_onu_history WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY device_id, pon, onu ORDER BY collected_at DESC) AS rn
				FROM olt_onu_history WHERE device_id = $1
			) t WHERE rn > 10
		)
	`, deviceID)
}

func intVal(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
