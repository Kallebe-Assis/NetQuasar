package snmpcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpprofile"
)

const baseDir = "data/snmp-discovery"

type DiscoveryData struct {
	DeviceID      string                   `json:"device_id"`
	Brand         string                   `json:"brand,omitempty"`
	Model         string                   `json:"model,omitempty"`
	CollectedAt   string                   `json:"collected_at"`
	RootOID       string                   `json:"root_oid"`
	RowCount      int                      `json:"row_count"`
	Truncated     bool                     `json:"truncated"`
	WalkNote      string                   `json:"walk_note,omitempty"`
	ClassSummary  map[string]int           `json:"class_summary"`
	CollectProfile snmpprofile.CollectProfile `json:"collect_profile"`
	DiscoveryDebug snmpprofile.DiscoveryDebug `json:"discovery_debug,omitempty"`
	Rows          []probing.SNMPVar        `json:"rows"`
}

type BrandModelCatalog struct {
	Key            string                   `json:"key"`
	Brand          string                   `json:"brand,omitempty"`
	Model          string                   `json:"model,omitempty"`
	UpdatedAt      string                   `json:"updated_at"`
	SampleCount    int                      `json:"sample_count"`
	KnownOIDs      []string                 `json:"known_oids"`
	ClassSummary   map[string]int           `json:"class_summary"`
	CollectProfile snmpprofile.CollectProfile `json:"collect_profile"`
}

func SaveEquipment(data DiscoveryData) error {
	if strings.TrimSpace(data.DeviceID) == "" {
		return fmt.Errorf("device_id vazio")
	}
	if data.CollectedAt == "" {
		data.CollectedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if data.ClassSummary == nil {
		data.ClassSummary = classifyRows(data.Rows)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "equipment"), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(equipmentPath(data.DeviceID), b, 0o644)
}

func LoadEquipment(deviceID string) (*DiscoveryData, error) {
	b, err := os.ReadFile(equipmentPath(deviceID))
	if err != nil {
		return nil, err
	}
	var out DiscoveryData
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func MergeBrandModel(data DiscoveryData) error {
	key := brandModelKey(data.Brand, data.Model)
	if key == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "catalog"), 0o755); err != nil {
		return err
	}
	p := catalogPath(key)
	cur := BrandModelCatalog{
		Key:          key,
		Brand:        strings.TrimSpace(data.Brand),
		Model:        strings.TrimSpace(data.Model),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		SampleCount:  0,
		KnownOIDs:    []string{},
		ClassSummary: map[string]int{},
	}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &cur)
	}
	cur.SampleCount++
	cur.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cur.CollectProfile = data.CollectProfile

	oidSeen := make(map[string]struct{}, len(cur.KnownOIDs))
	for _, o := range cur.KnownOIDs {
		oidSeen[o] = struct{}{}
	}
	for _, r := range data.Rows {
		if strings.TrimSpace(r.OID) == "" {
			continue
		}
		if _, ok := oidSeen[r.OID]; ok {
			continue
		}
		oidSeen[r.OID] = struct{}{}
		cur.KnownOIDs = append(cur.KnownOIDs, r.OID)
	}
	if cur.ClassSummary == nil {
		cur.ClassSummary = map[string]int{}
	}
	for k, v := range data.ClassSummary {
		cur.ClassSummary[k] += v
	}
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func equipmentPath(deviceID string) string {
	return filepath.Join(baseDir, "equipment", sanitizeFilePart(deviceID)+".json")
}

func catalogPath(key string) string {
	return filepath.Join(baseDir, "catalog", sanitizeFilePart(key)+".json")
}

func brandModelKey(brand, model string) string {
	b := strings.TrimSpace(brand)
	m := strings.TrimSpace(model)
	if b == "" && m == "" {
		return ""
	}
	if b == "" {
		b = "unknown-brand"
	}
	if m == "" {
		m = "unknown-model"
	}
	return strings.ToLower(b + "__" + m)
}

func sanitizeFilePart(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "unknown"
	}
	r := strings.NewReplacer(" ", "_", "/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	return r.Replace(v)
}

func classifyRows(rows []probing.SNMPVar) map[string]int {
	out := map[string]int{
		"system":         0,
		"interfaces":     0,
		"ip":             0,
		"tcp_udp":        0,
		"host_resources": 0,
		"bridge":         0,
		"snmp":           0,
		"other":          0,
	}
	for _, r := range rows {
		oid := strings.TrimSpace(r.OID)
		switch {
		case strings.HasPrefix(oid, "1.3.6.1.2.1.1."):
			out["system"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.2.") || strings.HasPrefix(oid, "1.3.6.1.2.1.31."):
			out["interfaces"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4."):
			out["ip"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.6.") || strings.HasPrefix(oid, "1.3.6.1.2.1.7."):
			out["tcp_udp"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.25."):
			out["host_resources"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.17."):
			out["bridge"]++
		case strings.HasPrefix(oid, "1.3.6.1.2.1.11."):
			out["snmp"]++
		default:
			out["other"]++
		}
	}
	return out
}
