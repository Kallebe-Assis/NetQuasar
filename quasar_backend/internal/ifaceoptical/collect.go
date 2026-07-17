package ifaceoptical

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/switchcollect"
)

// CollectTelnetPorts executa métricas Telnet de interfaces/óptica e devolve portas com potência.
func CollectTelnetPorts(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host string, isSwitch bool, timeout time.Duration) []Port {
	host = strings.TrimSpace(host)
	if pool == nil || host == "" || deviceID == uuid.Nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	if timeout > 90*time.Second {
		timeout = 90 * time.Second
	}
	// Orçamento próprio: o ctx pai costuma estar quase esgotado após o walk SNMP.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	creds := mikrotikcollect.LoadTelnetCredentialsForDevice(runCtx, pool, deviceID)
	if strings.TrimSpace(creds.User) == "" || strings.TrimSpace(creds.Password) == "" {
		return nil
	}

	var out mikrotikcollect.TelnetCollectOutput
	if isSwitch {
		profile := switchcollect.LoadTelnetProfileForDevice(runCtx, pool, deviceID)
		out = mikrotikcollect.CollectTelnetInterfaceMetricsWithCatalog(runCtx, host, creds, profile, timeout, switchcollect.TelnetMetricCatalog)
	} else {
		profile := mikrotikcollect.LoadTelnetProfileForDevice(runCtx, pool, deviceID)
		if !mikrotikcollect.HasEnabledTelnetMetrics(profile.Metrics) {
			return nil
		}
		out = mikrotikcollect.CollectTelnetInterfaceMetrics(runCtx, host, creds, profile, timeout)
	}
	return PortsFromTelnet(out)
}

// EnrichSnapshotArray corre Telnet óptico e acrescenta meta ao array do snapshot.
func EnrichSnapshotArray(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host string, isSwitch bool, arr []map[string]any, timeout time.Duration) []map[string]any {
	ports := CollectTelnetPorts(ctx, pool, deviceID, host, isSwitch, timeout)
	if len(ports) == 0 {
		// Fallback: última telemetria (já pode ter show interface transceiver details).
		ports = LoadPortsFromLatestTelemetry(context.WithoutCancel(ctx), pool, deviceID)
	}
	if len(ports) == 0 {
		return arr
	}
	vars := walkVarsFromMaps(arr)
	ifRows := snmpifparse.BuildIfTable(vars)
	ports = ResolveIfIndexes(ports, ifRows)
	return AppendMeta(arr, ports)
}

func walkVarsFromMaps(arr []map[string]any) []probing.SNMPVar {
	out := make([]probing.SNMPVar, 0, len(arr))
	for _, row := range arr {
		oid := strings.TrimSpace(str(row["oid"]))
		if oid == "" || strings.HasPrefix(oid, "__netquasar.") {
			continue
		}
		out = append(out, probing.SNMPVar{
			OID:   oid,
			Value: str(row["value"]),
			Type:  str(row["type"]),
		})
	}
	return out
}