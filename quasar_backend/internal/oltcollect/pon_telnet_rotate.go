package oltcollect

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// PonTelnetEnrichOpts rodízio e preservação de métricas CLI entre ciclos.
type PonTelnetEnrichOpts struct {
	PrevRows      []map[string]any
	RotateOffset  int
}

// LoadPrevPonTelnetState lê PONs e offset de rodízio do snapshot anterior.
func LoadPrevPonTelnetState(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) (prevPons []map[string]any, rotateOffset int) {
	if pool == nil || deviceID == uuid.Nil {
		return nil, 0
	}
	var ponsRaw, sumRaw []byte
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(pons::text, '[]'), COALESCE(summary::text, '{}')
		FROM olt_snapshots WHERE device_id=$1
	`, deviceID).Scan(&ponsRaw, &sumRaw)
	if err != nil {
		return nil, 0
	}
	var arr []any
	if json.Unmarshal(ponsRaw, &arr) == nil {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				prevPons = append(prevPons, m)
			}
		}
	}
	var summary map[string]any
	if json.Unmarshal(sumRaw, &summary) == nil {
		rotateOffset = intFromAny(summary["pon_telnet_rotate_offset"])
	}
	return prevPons, rotateOffset
}

func ponRowSortKey(row map[string]any) int {
	return ponIndexFromRowMap(row)
}

func sortPonRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return ponRowSortKey(rows[i]) < ponRowSortKey(rows[j])
	})
}

func selectRotatingPonBatch(pons []map[string]any, maxN, offset int) (batch []map[string]any, nextOffset int) {
	total := len(pons)
	if total == 0 || maxN <= 0 {
		return nil, 0
	}
	if maxN >= total {
		return pons, 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		offset = offset % total
	}
	batch = make([]map[string]any, 0, maxN)
	for i := 0; i < maxN; i++ {
		batch = append(batch, pons[(offset+i)%total])
	}
	nextOffset = (offset + len(batch)) % total
	return batch, nextOffset
}

var ponTelnetCarryKeys = []string{
	"temperature", "voltage", "tx_dbm", "rx_dbm", "current",
	"pon_telnet_at", "pon_telnet_source", "pon_telnet_fields",
}

func carryPonTelnetFromPrev(dst, src map[string]any) {
	if dst == nil || src == nil || src["pon_telnet_source"] != true {
		return
	}
	for _, k := range ponTelnetCarryKeys {
		if v, ok := src[k]; ok && v != nil {
			dst[k] = v
		}
	}
}

func carryForwardPonTelnetFromPrev(pons, prevPons []map[string]any, refreshKeys map[string]bool) {
	if len(prevPons) == 0 {
		return
	}
	prevByKey := map[string]map[string]any{}
	for _, p := range prevPons {
		k := ponStableKey(p)
		if k != "" {
			prevByKey[k] = p
		}
	}
	for i := range pons {
		k := ponStableKey(pons[i])
		if refreshKeys[k] {
			continue
		}
		if prev, ok := prevByKey[k]; ok {
			carryPonTelnetFromPrev(pons[i], prev)
		}
	}
}

func ponStableKey(row map[string]any) string {
	if row == nil {
		return ""
	}
	if p := ponIndexFromRowMap(row); p > 0 {
		return fmt.Sprintf("pon:%d", p)
	}
	if id := strings.TrimSpace(stringFromAny(row["id"])); id != "" {
		return "id:" + id
	}
	return ""
}

func openPonTelnetSession(
	ctx context.Context,
	host string,
	creds TelnetCredentials,
	cfg PonTelnetConfig,
	secrets TelnetSecrets,
	totalBudget time.Duration,
) (*probing.TelnetSessionHandle, error) {
	firstPon := 1
	preRendered := cfg.RenderPreCommands(OnuReportTarget{Pon: firstPon}, secrets)
	return probing.OpenTelnetSession(ctx, probing.TelnetRunScriptParams{
		Host: host, Port: "23", Timeout: totalBudget,
		User: creds.User, Password: creds.Password, Enable: creds.Enable,
		PreCommands: preRendered, RawPreCommands: cfg.PreCommands,
		MaxReadBytes: 120000,
	})
}
