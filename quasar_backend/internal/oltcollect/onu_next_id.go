package oltcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxOnuIDFromEntries devolve o maior ID de ONU presente na listagem.
func MaxOnuIDFromEntries(entries []SerialSearchOnuEntry, pon int) int {
	max := 0
	for _, e := range entries {
		if pon > 0 && e.Pon > 0 && e.Pon != pon {
			continue
		}
		if e.Onu > max {
			max = e.Onu
		}
	}
	return max
}

// CollectOnuIDsFromEntries junta os IDs de ONU usados na PON.
func CollectOnuIDsFromEntries(entries []SerialSearchOnuEntry, pon int) map[int]struct{} {
	out := map[int]struct{}{}
	for _, e := range entries {
		if e.Onu <= 0 {
			continue
		}
		if pon > 0 && e.Pon > 0 && e.Pon != pon {
			continue
		}
		out[e.Onu] = struct{}{}
	}
	return out
}

// CollectOnuIDsFromSnapshotSummary junta IDs de ONU da PON a partir do summary.
func CollectOnuIDsFromSnapshotSummary(summaryJSON []byte, pon int) map[int]struct{} {
	out := map[int]struct{}{}
	if len(summaryJSON) == 0 || pon <= 0 {
		return out
	}
	var summary map[string]any
	if json.Unmarshal(summaryJSON, &summary) != nil {
		return out
	}
	for _, row := range OnuRowsFromSummary(summary) {
		p := intFromAny(row["pon"])
		o := intFromAny(row["onu"])
		if p == pon && o > 0 {
			out[o] = struct{}{}
		}
	}
	return out
}

// FirstAvailableOnuID devolve o menor ID livre (≥1), preenchendo buracos (ex.: 7 se 1–6 e 8–10 existem).
func FirstAvailableOnuID(used map[int]struct{}) int {
	if used == nil {
		return 1
	}
	for id := 1; ; id++ {
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

// MaxOnuIDFromSnapshotSummary devolve o maior ONU ID na PON a partir do summary.
func MaxOnuIDFromSnapshotSummary(summaryJSON []byte, pon int) int {
	max := 0
	for id := range CollectOnuIDsFromSnapshotSummary(summaryJSON, pon) {
		if id > max {
			max = id
		}
	}
	return max
}

// PonOnuListCommand template de listagem de ONUs autorizadas na porta (para achar próximo ID livre).
func PonOnuListCommand(brand string, cfg OnuReportConfig) string {
	if tpl := strings.TrimSpace(cfg.SerialListSearchCommand); tpl != "" {
		if strings.Contains(tpl, "{pon}") && !strings.Contains(strings.ToLower(tpl), "{serial}") {
			return tpl
		}
	}
	if tpl := strings.TrimSpace(cfg.SerialSearchCommand); tpl != "" {
		low := strings.ToLower(tpl)
		if strings.Contains(tpl, "{pon}") && !strings.Contains(low, "{serial}") {
			return tpl
		}
	}
	b := strings.ToUpper(strings.TrimSpace(brand))
	if strings.Contains(b, "VSOL") {
		return "show onu info {pon}"
	}
	if strings.Contains(b, "ZTE") {
		return "show gpon onu state gpon-olt_1/1/{pon}"
	}
	return "show onu info {pon}"
}

// ResolveNextAvailableOnuID consulta snapshot + listagem telnet da PON e devolve o menor ID livre.
func ResolveNextAvailableOnuID(
	ctx context.Context,
	pool *pgxpool.Pool,
	deviceID uuid.UUID,
	host, user, password, enable, brand string,
	cfg OnuReportConfig,
	secrets TelnetSecrets,
	pon int,
	timeout time.Duration,
) (next int, listCmd string, err error) {
	if pon <= 0 {
		return 0, "", fmt.Errorf("PON inválida para alocar ID de ONU")
	}
	used := map[int]struct{}{}
	if pool != nil && deviceID != uuid.Nil {
		var sum []byte
		_ = pool.QueryRow(ctx, `SELECT COALESCE(summary::text, '{}') FROM olt_snapshots WHERE device_id=$1`, deviceID).Scan(&sum)
		for id := range CollectOnuIDsFromSnapshotSummary(sum, pon) {
			used[id] = struct{}{}
		}
	}

	cmdTpl := PonOnuListCommand(brand, cfg)
	target := OnuReportTarget{Pon: pon}
	listRes := RunOnuTelnetActionWithPre(ctx, host, user, password, enable, cfg.PreCommands, secrets, target, cmdTpl, timeout)
	listCmd = strings.TrimSpace(listRes.Command)
	if listRes.Output != "" {
		entries := ParseOnuListFromTelnetOutput(listRes.Output)
		for id := range CollectOnuIDsFromEntries(entries, pon) {
			used[id] = struct{}{}
		}
	}

	if !listRes.OK && listRes.Error != "" && len(used) == 0 {
		// PON aparentemente vazia ou listagem falhou — usar ID 1.
		return 1, listCmd, nil
	}
	return FirstAvailableOnuID(used), listCmd, nil
}
