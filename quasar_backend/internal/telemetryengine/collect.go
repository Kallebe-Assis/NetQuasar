package telemetryengine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpprofile"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

type CollectResult struct {
	OK      bool                  `json:"ok"`
	OIDs    []string              `json:"oids"`
	SNMP    probing.SNMPGetResult `json:"snmp"`
	Metrics map[string]any        `json:"metrics"`
}

type defaultOIDProfile struct {
	CPUOID         string
	CPUAvailOID    string
	MemoryUsedOID  string
	MemorySizeOID  string
	TempOID        string
	UptimeOID      string
	ExtraOIDs      []string
	ExtraOIDLabels map[string]string
}

type categoryOIDOverride struct {
	CPUOID        string            `json:"cpu_oid"`
	CPUAvailOID   string            `json:"cpu_available_oid"`
	MemoryUsedOID string            `json:"memory_used_oid"`
	MemorySizeOID string            `json:"memory_size_oid"`
	TempOID       string            `json:"temp_oid"`
	UptimeOID     string            `json:"uptime_oid"`
	InterfaceOIDs []string          `json:"interface_oids"`
	OpticalOIDs   []string          `json:"optical_oids"`
	PonOIDs       []string          `json:"pon_oids"`
	OnuOIDs       []string          `json:"onu_oids"`
	BridgeOIDs    []string          `json:"bridge_oids"`
	TrafficOIDs   []string          `json:"traffic_oids"`
	OIDLabels     map[string]string `json:"oid_labels"`
}

type snmpOIDOverridesDoc struct {
	OLT      categoryOIDOverride `json:"olt"`
	Mikrotik categoryOIDOverride `json:"mikrotik"`
	Servidor categoryOIDOverride `json:"servidor"`
	Bridge   categoryOIDOverride `json:"bridge"`
}

func oidsFromProfileJSON(profileJSON []byte) []string {
	base := []string{"1.3.6.1.2.1.1.3.0", "1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.1.0"}
	if len(profileJSON) == 0 {
		return base
	}
	var p snmpprofile.CollectProfile
	if json.Unmarshal(profileJSON, &p) != nil {
		return base
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(oid string) {
		oid = strings.TrimSpace(oid)
		if oid == "" {
			return
		}
		if _, ok := seen[oid]; ok {
			return
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	for _, o := range base {
		add(o)
	}
	if p.CPUPrimaryOID != "" {
		add(p.CPUPrimaryOID)
	}
	if p.CPUAvailableOID != "" {
		add(p.CPUAvailableOID)
	}
	if p.MemoryUsedOID != "" {
		add(p.MemoryUsedOID)
	}
	if p.MemorySizeOID != "" {
		add(p.MemorySizeOID)
	}
	if p.TempPrimaryOID != "" {
		add(p.TempPrimaryOID)
	}
	if p.UptimeOID != "" {
		add(p.UptimeOID)
	}
	if p.SysNameOID != "" {
		add(p.SysNameOID)
	}
	if p.SysDescrOID != "" {
		add(p.SysDescrOID)
	}
	for _, o := range p.CPUOIDs {
		add(o)
	}
	for _, o := range p.MemoryOIDs {
		add(o)
	}
	for _, o := range p.TempOIDs {
		add(o)
	}
	return out
}

func normalizeOIDKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, ".")
	return s
}

func profileJSONForDevice(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) []byte {
	// Modo estrito: monitoramento usa apenas OIDs configurados (sem seguir inventário/walk).
	d := loadDefaultOIDProfile(ctx, pool, deviceID)
	p := snmpprofile.CollectProfile{
		UptimeOID:       strings.TrimSpace(d.UptimeOID),
		CPUPrimaryOID:   strings.TrimSpace(d.CPUOID),
		CPUAvailableOID: strings.TrimSpace(d.CPUAvailOID),
		MemoryUsedOID:   strings.TrimSpace(d.MemoryUsedOID),
		MemorySizeOID:   strings.TrimSpace(d.MemorySizeOID),
		TempPrimaryOID:  strings.TrimSpace(d.TempOID),
	}
	if len(d.ExtraOIDLabels) > 0 {
		p.ExtraOIDLabels = make(map[string]string, len(d.ExtraOIDLabels))
		for k, v := range d.ExtraOIDLabels {
			kk := normalizeOIDKey(k)
			if kk == "" {
				continue
			}
			vv := strings.TrimSpace(v)
			if vv == "" {
				continue
			}
			p.ExtraOIDLabels[kk] = vv
		}
		if len(p.ExtraOIDLabels) == 0 {
			p.ExtraOIDLabels = nil
		}
	}
	// Se não houver uptime configurado, mantém OID universal para evitar amostra vazia.
	if p.UptimeOID == "" {
		p.UptimeOID = "1.3.6.1.2.1.1.3.0"
	}
	for _, o := range d.ExtraOIDs {
		p.CPUOIDs = appendUniqueOID(p.CPUOIDs, o)
	}
	if b, e := json.Marshal(p); e == nil {
		return b
	}
	return []byte(`{"uptime_oid":"1.3.6.1.2.1.1.3.0"}`)
}

func loadDefaultOIDProfile(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) defaultOIDProfile {
	var category, brand, networkStatus string
	var oidStrategy string
	var deviceOverridesRaw []byte
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(category,''), coalesce(brand,''), coalesce(network_status,''),
			coalesce(telemetry_oid_strategy,'default'), coalesce(telemetry_oid_overrides::text,'{}')
		FROM devices WHERE id=$1
	`, deviceID).Scan(&category, &brand, &networkStatus, &oidStrategy, &deviceOverridesRaw); err != nil {
		return defaultOIDProfile{}
	}
	category = strings.ToLower(strings.TrimSpace(category))
	brand = strings.ToLower(strings.TrimSpace(brand))
	networkStatus = strings.ToLower(strings.TrimSpace(networkStatus))
	prefix := "server"
	if networkStatus == "bridge" {
		prefix = "bridge"
	} else if category == "olt" {
		prefix = "olt"
	} else if strings.Contains(brand, "mikrotik") || strings.Contains(category, "mikrotik") {
		prefix = "mikrotik"
	} else if category == "servidor" || category == "server" {
		prefix = "server"
	}
	var d defaultOIDProfile
	switch prefix {
	case "olt":
		_ = pool.QueryRow(ctx, `
			SELECT coalesce(olt_cpu_oid,''), coalesce(olt_cpu_available_oid,''),
				coalesce(olt_memory_used_oid,''), coalesce(olt_memory_size_oid,''), coalesce(olt_temp_oid,''), coalesce(olt_uptime_oid,'')
			FROM settings_connection_defaults WHERE id=1
		`).Scan(&d.CPUOID, &d.CPUAvailOID, &d.MemoryUsedOID, &d.MemorySizeOID, &d.TempOID, &d.UptimeOID)
	case "mikrotik":
		_ = pool.QueryRow(ctx, `
			SELECT coalesce(mikrotik_cpu_oid,''), coalesce(mikrotik_cpu_available_oid,''),
				coalesce(mikrotik_memory_used_oid,''), coalesce(mikrotik_memory_size_oid,''), coalesce(mikrotik_temp_oid,''), coalesce(mikrotik_uptime_oid,'')
			FROM settings_connection_defaults WHERE id=1
		`).Scan(&d.CPUOID, &d.CPUAvailOID, &d.MemoryUsedOID, &d.MemorySizeOID, &d.TempOID, &d.UptimeOID)
	default:
		_ = pool.QueryRow(ctx, `
			SELECT coalesce(server_cpu_oid,''), coalesce(server_cpu_available_oid,''),
				coalesce(server_memory_used_oid,''), coalesce(server_memory_size_oid,''), coalesce(server_temp_oid,''), coalesce(server_uptime_oid,'')
			FROM settings_connection_defaults WHERE id=1
		`).Scan(&d.CPUOID, &d.CPUAvailOID, &d.MemoryUsedOID, &d.MemorySizeOID, &d.TempOID, &d.UptimeOID)
	}
	var raw []byte
	_ = pool.QueryRow(ctx, `SELECT snmp_oid_overrides::text FROM settings_connection_defaults WHERE id=1`).Scan(&raw)
	if len(raw) > 0 {
		var doc snmpOIDOverridesDoc
		if json.Unmarshal(raw, &doc) == nil {
			var c categoryOIDOverride
			switch prefix {
			case "olt":
				c = doc.OLT
			case "mikrotik":
				c = doc.Mikrotik
			case "bridge":
				c = doc.Bridge
			default:
				c = doc.Servidor
			}
			if strings.TrimSpace(c.CPUOID) != "" {
				d.CPUOID = strings.TrimSpace(c.CPUOID)
			}
			if strings.TrimSpace(c.CPUAvailOID) != "" {
				d.CPUAvailOID = strings.TrimSpace(c.CPUAvailOID)
			}
			if strings.TrimSpace(c.MemoryUsedOID) != "" {
				d.MemoryUsedOID = strings.TrimSpace(c.MemoryUsedOID)
			}
			if strings.TrimSpace(c.MemorySizeOID) != "" {
				d.MemorySizeOID = strings.TrimSpace(c.MemorySizeOID)
			}
			if strings.TrimSpace(c.TempOID) != "" {
				d.TempOID = strings.TrimSpace(c.TempOID)
			}
			if strings.TrimSpace(c.UptimeOID) != "" {
				d.UptimeOID = strings.TrimSpace(c.UptimeOID)
			}
			join := [][]string{c.InterfaceOIDs, c.OpticalOIDs, c.PonOIDs, c.OnuOIDs, c.BridgeOIDs, c.TrafficOIDs}
			for _, arr := range join {
				for _, o := range arr {
					if s := strings.TrimSpace(o); s != "" {
						d.ExtraOIDs = appendUniqueOID(d.ExtraOIDs, s)
					}
				}
			}
			if len(c.OIDLabels) > 0 {
				if d.ExtraOIDLabels == nil {
					d.ExtraOIDLabels = make(map[string]string)
				}
				for k, v := range c.OIDLabels {
					kk := normalizeOIDKey(k)
					vv := strings.TrimSpace(v)
					if kk == "" || vv == "" {
						continue
					}
					d.ExtraOIDLabels[kk] = vv
				}
			}
		}
	}
	// Para categoria "Outros", permite OIDs manuais por equipamento.
	if strings.EqualFold(strings.TrimSpace(category), "outros") && strings.EqualFold(strings.TrimSpace(oidStrategy), "manual") && len(deviceOverridesRaw) > 0 {
		var dev categoryOIDOverride
		if json.Unmarshal(deviceOverridesRaw, &dev) == nil {
			if strings.TrimSpace(dev.CPUOID) != "" {
				d.CPUOID = strings.TrimSpace(dev.CPUOID)
			}
			if strings.TrimSpace(dev.CPUAvailOID) != "" {
				d.CPUAvailOID = strings.TrimSpace(dev.CPUAvailOID)
			}
			if strings.TrimSpace(dev.MemoryUsedOID) != "" {
				d.MemoryUsedOID = strings.TrimSpace(dev.MemoryUsedOID)
			}
			if strings.TrimSpace(dev.MemorySizeOID) != "" {
				d.MemorySizeOID = strings.TrimSpace(dev.MemorySizeOID)
			}
			if strings.TrimSpace(dev.TempOID) != "" {
				d.TempOID = strings.TrimSpace(dev.TempOID)
			}
			if strings.TrimSpace(dev.UptimeOID) != "" {
				d.UptimeOID = strings.TrimSpace(dev.UptimeOID)
			}
		}
	}
	if prefix == "olt" {
		applyVsolUptimeDefault(brand, &d)
	}
	return d
}

func applyVsolUptimeDefault(brand string, d *defaultOIDProfile) {
	if d == nil {
		return
	}
	b := strings.ToLower(strings.TrimSpace(brand))
	if !strings.Contains(b, "vsol") && !strings.Contains(b, "v1600") && !strings.Contains(b, "1600g") {
		return
	}
	u := strings.TrimSpace(d.UptimeOID)
	if u == "" || u == "1.3.6.1.2.1.1.3.0" {
		d.UptimeOID = vsolparse.OIDVsolUptime
	}
}

func appendUniqueOID(list []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// CollectAndStore executa GET SNMP dinâmico e persiste em telemetry_samples.
func CollectAndStore(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host, community string) (CollectResult, error) {
	profJSON := profileJSONForDevice(ctx, pool, deviceID)
	oids := oidsFromProfileJSON(profJSON)

	var merged []probing.SNMPVar
	ok := true
	var lastErr string
	const chunk = 32
	for i := 0; i < len(oids); i += chunk {
		j := i + chunk
		if j > len(oids) {
			j = len(oids)
		}
		part := probing.SNMPGet(ctx, probing.SNMPGetParams{
			Host: strings.TrimSpace(host), Community: strings.TrimSpace(community), OIDs: oids[i:j], Version: "2c", Timeout: 12 * time.Second, Retries: 0,
		})
		merged = append(merged, part.Vars...)
		if !part.OK {
			ok = false
			if part.Error != "" {
				lastErr = part.Error
			}
		}
	}
	sn := probing.SNMPGetResult{OK: ok, Vars: merged, Error: lastErr, Note: "SNMP GET em lotes; OIDs do inventário quando existir."}
	var profileAny any
	if len(profJSON) > 0 {
		_ = json.Unmarshal(profJSON, &profileAny)
	}
	metrics := map[string]any{"snmp": sn, "oids": oids, "profile": profileAny}
	b, _ := json.Marshal(metrics)
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return CollectResult{}, ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		_, err = pool.Exec(ctx, `INSERT INTO telemetry_samples (device_id, collected_at, metrics) VALUES ($1, now(), $2::jsonb)`, deviceID, b)
		if err == nil {
			break
		}
	}
	if err != nil {
		return CollectResult{}, err
	}
	return CollectResult{OK: sn.OK, OIDs: oids, SNMP: sn, Metrics: metrics}, nil
}
