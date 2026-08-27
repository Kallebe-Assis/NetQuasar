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

	"github.com/google/uuid"
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
