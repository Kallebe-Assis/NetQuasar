package oltcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OnuTelnetEnrichOpts estado opcional para rodízio e preservação de dados CLI anteriores.
type OnuTelnetEnrichOpts struct {
	PrevRows      []map[string]any
	RotateOffset  int
}

// LoadPrevOnuTelnetState lê linhas ONU e offset de rodízio do snapshot anterior da OLT.
func LoadPrevOnuTelnetState(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (prevRows []map[string]any, rotateOffset int) {
	if pool == nil || deviceID == uuid.Nil {
		return nil, 0
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT COALESCE(summary::text, '{}') FROM olt_snapshots WHERE device_id=$1`, deviceID).
		Scan(&raw); err != nil || len(raw) == 0 {
		return nil, 0
	}
	var summary map[string]any
	if json.Unmarshal(raw, &summary) != nil {
		return nil, 0
	}
	return OnuRowsFromSummary(summary), intFromAny(summary["onu_telnet_rotate_offset"])
}

func onuRowKey(row map[string]any) string {
	return fmt.Sprintf("%d.%d", intFromRow(row, "pon"), intFromRow(row, "onu"))
}

func sortOnuRowsByPonOnu(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		pi, oi := intFromRow(rows[i], "pon"), intFromRow(rows[i], "onu")
		pj, oj := intFromRow(rows[j], "pon"), intFromRow(rows[j], "onu")
		if pi != pj {
			return pi < pj
		}
		return oi < oj
	})
}

func buildOnuTelnetCandidates(rows []map[string]any, cfg OnuReportConfig) []map[string]any {
	candidates := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if cfg.MonitorOnlineOnly && !onuRowOnline(row) {
			continue
		}
		candidates = append(candidates, row)
	}
	if len(candidates) == 0 {
		candidates = append(candidates, rows...)
	}
	sortOnuRowsByPonOnu(candidates)
	return candidates
}

// selectRotatingOnuBatch escolhe o lote deste ciclo e calcula o offset para o próximo.
func selectRotatingOnuBatch(candidates []map[string]any, maxN, offset int) (batch []map[string]any, nextOffset int) {
	total := len(candidates)
	if total == 0 || maxN <= 0 {
		return nil, 0
	}
	if maxN >= total {
		return candidates, 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		offset = offset % total
	}
	batch = make([]map[string]any, 0, maxN)
	for i := 0; i < maxN; i++ {
		batch = append(batch, candidates[(offset+i)%total])
	}
	nextOffset = (offset + len(batch)) % total
	return batch, nextOffset
}

// onuTelnetCarryKeys campos de identidade/config — fazem sentido preservar do snapshot
// anterior mesmo que a ONU esteja offline agora (não são "leituras ao vivo").
var onuTelnetCarryKeys = []string{
	"serial", "model", "profile_name", "phase_sta", "channel",
	"telnet_report_at", "data_source_telnet", "telnet_fields",
}

// onuTelnetLiveCarryKeys leituras ópticas/eléctricas — só fazem sentido herdadas do ciclo
// anterior (ONU fora do lote de rodízio desta vez) se a ONU segue online agora; herdar por
// cima de uma ONU que caiu reintroduzia dado obsoleto (rx/tx/temp/voltage de quando ainda
// respondia), o mesmo problema já corrigido do lado SNMP em BuildOnuTable.
var onuTelnetLiveCarryKeys = []string{"rx_pwr", "rx_dbm", "tx_pwr", "tx_dbm", "temp", "voltage"}

func carryTelnetFieldsFromPrev(dst, src map[string]any) {
	if dst == nil || src == nil {
		return
	}
	if src["data_source_telnet"] != true {
		return
	}
	for _, k := range onuTelnetCarryKeys {
		if v, ok := src[k]; ok && v != nil {
			dst[k] = v
		}
	}
	if onuRowOnline(dst) {
		for _, k := range onuTelnetLiveCarryKeys {
			if v, ok := src[k]; ok && v != nil {
				dst[k] = v
			}
		}
	}
}

func carryForwardTelnetFromPrev(outRows, prevRows []map[string]any, refreshKeys map[string]bool) {
	if len(prevRows) == 0 {
		return
	}
	prevByKey := make(map[string]map[string]any, len(prevRows))
	for _, r := range prevRows {
		if k := onuRowKey(r); k != "0.0" {
			prevByKey[k] = r
		}
	}
	for i := range outRows {
		k := onuRowKey(outRows[i])
		if refreshKeys[k] {
			continue
		}
		if prev, ok := prevByKey[k]; ok {
			carryTelnetFieldsFromPrev(outRows[i], prev)
		}
	}
}
