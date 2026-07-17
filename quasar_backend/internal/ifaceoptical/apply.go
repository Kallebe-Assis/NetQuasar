package ifaceoptical

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
)

var reCiscoPortKey = regexp.MustCompile(`(?i)(?:ethernet|eth|gi|te|port-channel|po)(\d+(?:/\d+){1,2})`)

// ApplyPortsToOpticalMap cruza portas Telnet com IF-MIB por ifIndex e por nome (Eth↔Ethernet).
func ApplyPortsToOpticalMap(opt map[int]snmpmikrotik.OpticalPower, ports []Port, rows []snmpifparse.IfRow) map[int]snmpmikrotik.OpticalPower {
	if opt == nil {
		opt = map[int]snmpmikrotik.OpticalPower{}
	}
	if len(ports) == 0 {
		return opt
	}
	ports = ResolveIfIndexes(ports, rows)
	opt = MergeIntoOpticalMap(opt, ports)

	// Segunda passagem: portas ainda sem ifIndex → match por chave de porta (1/12).
	for _, p := range ports {
		if p.IfIndex > 0 {
			continue
		}
		if idx, ok := matchPortKeyToIfIndex(p.Name, rows); ok {
			p.IfIndex = idx
			opt = MergeIntoOpticalMap(opt, []Port{p})
		}
	}
	return opt
}

func matchPortKeyToIfIndex(name string, rows []snmpifparse.IfRow) (int, bool) {
	want := portKey(name)
	if want == "" {
		return 0, false
	}
	var hits []int
	seen := map[int]struct{}{}
	for _, r := range rows {
		for _, h := range []string{r.IfName, r.DisplayName, r.Descr} {
			got := portKey(h)
			if got == "" || got != want {
				continue
			}
			if _, ok := seen[r.IfIndex]; ok {
				continue
			}
			seen[r.IfIndex] = struct{}{}
			hits = append(hits, r.IfIndex)
		}
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	return 0, false
}

func portKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	m := reCiscoPortKey.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// PortsFromTelnetCollectionJSON extrai portas de mikrotik_telnet_collection / switch_telnet_collection.
func PortsFromTelnetCollectionJSON(raw []byte) []Port {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var doc struct {
		Fields map[string]struct {
			OK    bool `json:"ok"`
			Value any  `json:"value"`
		} `json:"fields"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Fields) == 0 {
		// métricas completas: { "switch_telnet_collection": { "fields": ... } }
		var wrap map[string]json.RawMessage
		if json.Unmarshal(raw, &wrap) != nil {
			return nil
		}
		for _, key := range []string{"switch_telnet_collection", "mikrotik_telnet_collection"} {
			if b, ok := wrap[key]; ok {
				return PortsFromTelnetCollectionJSON(b)
			}
		}
		return nil
	}
	out := mikrotikcollect.TelnetCollectOutput{Fields: map[string]mikrotikcollect.TelnetFieldResult{}}
	for k, fr := range doc.Fields {
		out.Fields[k] = mikrotikcollect.TelnetFieldResult{OK: fr.OK, Value: fr.Value}
	}
	return PortsFromTelnet(out)
}

// LoadPortsFromLatestTelemetry lê a última telemetria do equipamento e extrai óptica Telnet.
func LoadPortsFromLatestTelemetry(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) []Port {
	if pool == nil || deviceID == uuid.Nil {
		return nil
	}
	var metrics []byte
	err := pool.QueryRow(ctx, `
		SELECT metrics FROM telemetry_samples
		WHERE device_id = $1
		ORDER BY collected_at DESC
		LIMIT 1
	`, deviceID).Scan(&metrics)
	if err != nil || len(metrics) == 0 {
		return nil
	}
	return PortsFromTelnetCollectionJSON(metrics)
}
