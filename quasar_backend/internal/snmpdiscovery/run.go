package snmpdiscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpcatalog"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmibhints"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpprofile"
)

const walkRootMIB2 = "1.3.6.1.2.1"
const walkMaxRows = 8000

var discoveryProbeOIDs = []string{
	"1.3.6.1.2.1.1.3.0",
	"1.3.6.1.2.1.25.3.3.1.2.1",
	"1.3.6.1.2.1.25.2.2.0",
	"1.3.6.1.2.1.99.1.1.1.4.1",
	"1.3.6.1.4.1.14988.1.1.3.10.0",
	"1.3.6.1.4.1.14988.1.1.3.14.0",
	"1.3.6.1.4.1.2021.11.11.0",
	"1.3.6.1.4.1.2021.4.5.0",
	"1.3.6.1.4.1.2021.4.6.0",
	"1.3.6.1.4.1.9.9.13.1.3.1.3.1",
}

const DefaultInventoryMaxAge = 24 * time.Hour

// EnsureFreshInventory garante inventário SNMP existente e atualizado.
// Se não existir ou estiver desatualizado, executa discovery.
func EnsureFreshInventory(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID, maxAge time.Duration) (string, error) {
	if maxAge <= 0 {
		maxAge = DefaultInventoryMaxAge
	}
	var discoveredAt time.Time
	err := pool.QueryRow(ctx, `SELECT discovered_at FROM device_snmp_inventory WHERE device_id=$1`, deviceID).Scan(&discoveredAt)
	switch {
	case err == nil:
		if time.Since(discoveredAt) <= maxAge {
			return "existing_fresh", nil
		}
	case err == pgx.ErrNoRows:
		// continua para discovery
	default:
		return "", err
	}
	if err := Run(ctx, pool, log, deviceID); err != nil {
		return "", err
	}
	if err == pgx.ErrNoRows {
		return "discovered_missing", nil
	}
	return "discovered_refreshed", nil
}

// Run executa walk SNMP MIB-II, gera perfil de colecta e grava em device_snmp_inventory.
func Run(ctx context.Context, pool *pgxpool.Pool, log *zerolog.Logger, deviceID uuid.UUID) error {
	if pool == nil {
		return fmt.Errorf("db nil")
	}
	var ip, brand, model, mibFolderPath *string
	var devComm *string
	err := pool.QueryRow(ctx, `SELECT host(ip)::text, snmp_community, brand, model, mib_folder_path FROM devices WHERE id=$1`, deviceID).
		Scan(&ip, &devComm, &brand, &model, &mibFolderPath)
	if err != nil {
		return err
	}
	host := ""
	if ip != nil {
		host = strings.TrimSpace(*ip)
	}
	if host == "" {
		return fmt.Errorf("equipamento sem IP")
	}
	comm := ""
	if devComm != nil {
		comm = strings.TrimSpace(*devComm)
	}
	if comm == "" {
		var def *string
		_ = pool.QueryRow(ctx, `SELECT snmp_community FROM settings_connection_defaults WHERE id=1`).Scan(&def)
		if def != nil {
			comm = strings.TrimSpace(*def)
		}
	}
	if comm == "" {
		return fmt.Errorf("sem snmp_community")
	}

	wctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	rows, truncated, walkNote := probing.SNMPWalkLimited(wctx, host, comm, walkRootMIB2, walkMaxRows, 45*time.Second)
	// Complementa com OIDs padrão de CPU/Mem/Temp (inclusive vendors), para perfil consistente.
	pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
	extra := probing.SNMPGet(pctx, probing.SNMPGetParams{
		Host: host, Community: comm, OIDs: discoveryProbeOIDs, Version: "2c", Timeout: 9 * time.Second, Retries: 0,
	})
	pcancel()
	if extra.OK && len(extra.Vars) > 0 {
		rows = mergeVarsByOID(rows, extra.Vars)
	}
	rows = sanitizeVars(rows)
	profile, profileDebug := snmpprofile.BuildCollectProfileWithDebug(rows)
	mibRes := snmpmibhints.ApplyFromFolder(strings.TrimSpace(ptrStr(mibFolderPath)), rows, &profile)
	sysDescr, sysObjectID := lookupScalar(rows, "1.3.6.1.2.1.1.1.0"), lookupScalar(rows, "1.3.6.1.2.1.1.2.0")
	vendor := detectVendor(sysDescr, sysObjectID, strings.TrimSpace(ptrStr(brand)))
	modelName := strings.TrimSpace(ptrStr(model))
	if modelName == "" {
		modelName = deriveModel(sysDescr)
	}
	pb, _ := json.Marshal(profile)
	rb, mErr := json.Marshal(rows)
	if mErr != nil || len(rb) == 0 {
		rb = []byte("[]")
	}
	summary := map[string]any{
		"root_oid":        walkRootMIB2,
		"host":            host,
		"walk_note":       walkNote,
		"row_count":       len(rows),
		"truncated":       truncated,
		"generated_at":    time.Now().UTC().Format(time.RFC3339),
		"discovery_debug": profileDebug,
		"mib_hints":       mibRes,
	}
	sb, _ := json.Marshal(summary)

	discovery := snmpcatalog.DiscoveryData{
		DeviceID:       deviceID.String(),
		Brand:          vendor,
		Model:          modelName,
		CollectedAt:    time.Now().UTC().Format(time.RFC3339),
		RootOID:        walkRootMIB2,
		RowCount:       len(rows),
		Truncated:      truncated,
		WalkNote:       walkNote,
		CollectProfile: profile,
		DiscoveryDebug: profileDebug,
		Rows:           rows,
	}
	if err := snmpcatalog.SaveEquipment(discovery); err != nil && log != nil {
		log.Warn().Err(err).Str("device", deviceID.String()).Msg("falha ao salvar discovery local")
	}
	if err := snmpcatalog.MergeBrandModel(discovery); err != nil && log != nil {
		log.Warn().Err(err).Str("device", deviceID.String()).Msg("falha ao atualizar catálogo local de marca/modelo")
	}
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer persistCancel()
	if err := persistDiscoveredOIDs(persistCtx, pool, deviceID, vendor, modelName, rows); err != nil && log != nil {
		log.Warn().Err(err).Str("device", deviceID.String()).Msg("falha ao persistir discovered_oids")
	}
	if err := persistSNMPProfile(persistCtx, pool, vendor, modelName, profile); err != nil && log != nil {
		log.Warn().Err(err).Str("device", deviceID.String()).Msg("falha ao persistir snmp_profile")
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO device_snmp_inventory (device_id, discovered_at, root_oid, row_count, truncated, walk_rows, walk_summary, collect_profile)
		VALUES ($1::uuid, now(), $2::text, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb)
		ON CONFLICT (device_id) DO UPDATE SET
			discovered_at = now(),
			root_oid = excluded.root_oid,
			row_count = excluded.row_count,
			truncated = excluded.truncated,
			walk_rows = excluded.walk_rows,
			walk_summary = excluded.walk_summary,
			collect_profile = excluded.collect_profile
	`, deviceID, walkRootMIB2, len(rows), truncated, rb, sb, pb)
	if err != nil {
		return err
	}
	if log != nil {
		log.Info().Str("device", deviceID.String()).Int("rows", len(rows)).Bool("truncated", truncated).Msg("SNMP discovery gravado")
	}
	return nil
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mergeVarsByOID(base, extra []probing.SNMPVar) []probing.SNMPVar {
	seen := make(map[string]struct{}, len(base))
	for _, v := range base {
		seen[cleanOID(v.OID)] = struct{}{}
	}
	out := append([]probing.SNMPVar{}, base...)
	for _, v := range extra {
		k := cleanOID(v.OID)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	return out
}

func lookupScalar(rows []probing.SNMPVar, oid string) string {
	for _, r := range rows {
		if cleanOID(r.OID) == cleanOID(oid) {
			return strings.TrimSpace(r.Value)
		}
	}
	return ""
}

func detectVendor(sysDescr, sysObjectID, fallback string) string {
	low := strings.ToLower(sysDescr + " " + sysObjectID)
	switch {
	case strings.Contains(low, "1.3.6.1.4.1.14988") || strings.Contains(low, "mikrotik"):
		return "mikrotik"
	case strings.Contains(low, "1.3.6.1.4.1.9") || strings.Contains(low, "cisco"):
		return "cisco"
	case strings.Contains(low, "1.3.6.1.4.1.2011") || strings.Contains(low, "huawei"):
		return "huawei"
	case strings.Contains(low, "1.3.6.1.4.1.3902") || strings.Contains(low, "zte"):
		return "zte"
	}
	fallback = strings.TrimSpace(strings.ToLower(fallback))
	if fallback != "" {
		return fallback
	}
	return "unknown"
}

func deriveModel(sysDescr string) string {
	s := strings.TrimSpace(sysDescr)
	if s == "" {
		return "unknown-model"
	}
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	parts := strings.Split(s, " ")
	if len(parts) > 4 {
		parts = parts[:4]
	}
	return strings.Join(parts, " ")
}

func normalizeOID(oid string) string {
	oid = cleanOID(oid)
	if oid == "" {
		return ""
	}
	parts := strings.Split(oid, ".")
	if len(parts) < 2 {
		return oid
	}
	last := parts[len(parts)-1]
	if _, err := strconv.Atoi(last); err != nil {
		return oid
	}
	// Mantém escalares *.0 (ex.: sysUpTime.0), remove índice final nos demais.
	if last == "0" {
		return oid
	}
	return strings.Join(parts[:len(parts)-1], ".")
}

func cleanOID(oid string) string {
	oid = strings.TrimSpace(oid)
	return strings.TrimLeft(oid, ".")
}

func sanitizeVars(rows []probing.SNMPVar) []probing.SNMPVar {
	out := make([]probing.SNMPVar, 0, len(rows))
	for _, r := range rows {
		r.OID = cleanOID(r.OID)
		out = append(out, r)
	}
	return out
}

func classifyOID(normOID, valueType, value string) string {
	switch {
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.25.3.3.1.2") || normOID == "1.3.6.1.4.1.14988.1.1.3.10.0" || normOID == "1.3.6.1.4.1.2021.11.11.0":
		return "cpu"
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.25.2.3.1.5") ||
		strings.HasPrefix(normOID, "1.3.6.1.2.1.25.2.3.1.6") ||
		normOID == "1.3.6.1.4.1.2021.4.5.0" || normOID == "1.3.6.1.4.1.2021.4.6.0":
		return "memory"
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.99.1.1.1.4") ||
		strings.HasPrefix(normOID, "1.3.6.1.4.1.9.9.13.1.3.1.3") ||
		normOID == "1.3.6.1.4.1.14988.1.1.3.14.0":
		return "temperature"
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.2.2.1") || strings.HasPrefix(normOID, "1.3.6.1.2.1.31.1.1.1"):
		return "interfaces"
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.31.1.1.1.6"):
		return "traffic_rx"
	case strings.HasPrefix(normOID, "1.3.6.1.2.1.31.1.1.1.10"):
		return "traffic_tx"
	case strings.HasPrefix(normOID, "1.3.6.1.4.1") && (strings.Contains(normOID, ".pon.") || strings.Contains(strings.ToLower(value), "pon")):
		return "pon"
	case strings.HasPrefix(normOID, "1.3.6.1.4.1") && (strings.Contains(normOID, ".onu.") || strings.Contains(strings.ToLower(value), "onu")):
		return "onu"
	}
	vt := strings.ToLower(valueType)
	if strings.Contains(vt, "counter64") {
		return "counter"
	}
	return "other"
}

func persistDiscoveredOIDs(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, vendor, model string, rows []probing.SNMPVar) error {
	var batch pgx.Batch
	queued := 0
	for _, r := range rows {
		norm := normalizeOID(r.OID)
		if norm == "" {
			continue
		}
		cat := classifyOID(norm, r.Type, r.Value)
		batch.Queue(`
			INSERT INTO discovered_oids (equipment_id, oid, normalized_oid, value, type, category, last_seen, vendor, model)
			VALUES ($1,$2,$3,$4,$5,$6,now(),$7,$8)
			ON CONFLICT (equipment_id, oid) DO UPDATE SET
				normalized_oid = EXCLUDED.normalized_oid,
				value = EXCLUDED.value,
				type = EXCLUDED.type,
				category = EXCLUDED.category,
				last_seen = now(),
				vendor = EXCLUDED.vendor,
				model = EXCLUDED.model
		`, deviceID, r.OID, norm, r.Value, r.Type, cat, vendor, model)
		queued++
	}
	br := pool.SendBatch(ctx, &batch)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func persistSNMPProfile(ctx context.Context, pool *pgxpool.Pool, vendor, model string, p snmpprofile.CollectProfile) error {
	vendor = strings.TrimSpace(strings.ToLower(vendor))
	model = strings.TrimSpace(model)
	if vendor == "" {
		vendor = "unknown"
	}
	if model == "" {
		model = "unknown-model"
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO snmp_profiles (vendor, model, cpu_oid, temp_oid, memory_used_oid, memory_size_oid, uptime_oid, interface_oid, rx_oid, tx_oid, source, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'auto_discovery',now())
		ON CONFLICT (vendor, model) DO UPDATE SET
			cpu_oid = COALESCE(EXCLUDED.cpu_oid, snmp_profiles.cpu_oid),
			temp_oid = COALESCE(EXCLUDED.temp_oid, snmp_profiles.temp_oid),
			memory_used_oid = COALESCE(EXCLUDED.memory_used_oid, snmp_profiles.memory_used_oid),
			memory_size_oid = COALESCE(EXCLUDED.memory_size_oid, snmp_profiles.memory_size_oid),
			uptime_oid = COALESCE(EXCLUDED.uptime_oid, snmp_profiles.uptime_oid),
			interface_oid = COALESCE(EXCLUDED.interface_oid, snmp_profiles.interface_oid),
			rx_oid = COALESCE(EXCLUDED.rx_oid, snmp_profiles.rx_oid),
			tx_oid = COALESCE(EXCLUDED.tx_oid, snmp_profiles.tx_oid),
			updated_at = now()
	`, vendor, model, nullIfEmpty(p.CPUPrimaryOID), nullIfEmpty(p.TempPrimaryOID), nullIfEmpty(p.MemoryUsedOID), nullIfEmpty(p.MemorySizeOID), nullIfEmpty(p.UptimeOID),
		"1.3.6.1.2.1.2.2.1", "1.3.6.1.2.1.31.1.1.1.6", "1.3.6.1.2.1.31.1.1.1.10")
	return err
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
