import {
  buildMikrotikNocKpis,
  buildMikrotikPppoeTop,
  buildMikrotikSfpPanels,
  buildMikrotikSystemInfo,
  type MikrotikIfRow,
  type MikrotikNocKpis,
  type MikrotikPppoeRow,
  type MikrotikSfpPanel,
  type MikrotikSystemInfo,
} from "./mikrotikNocData";
import { EM_DASH } from "./formatDisplay";

export type { MikrotikIfRow, MikrotikNocKpis, MikrotikPppoeRow, MikrotikSfpPanel, MikrotikSystemInfo };

function mapSwitchMetrics(metrics: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!metrics) return undefined;
  return {
    ...metrics,
    mikrotik_collection: metrics.switch_collection ?? metrics.mikrotik_collection,
    mikrotik_telnet_collection: metrics.switch_telnet_collection ?? metrics.mikrotik_telnet_collection,
  };
}

export function buildSwitchNocKpis(metrics: Record<string, unknown> | undefined, deviceName: string): MikrotikNocKpis {
  return buildMikrotikNocKpis(mapSwitchMetrics(metrics), deviceName);
}

export function buildSwitchPppoeTop(metrics: Record<string, unknown> | undefined, limit = 5): MikrotikPppoeRow[] {
  return buildMikrotikPppoeTop(mapSwitchMetrics(metrics), limit);
}

export function buildSwitchSfpPanels(metrics: Record<string, unknown> | undefined, ifaces: MikrotikIfRow[]): MikrotikSfpPanel[] {
  return buildMikrotikSfpPanels(mapSwitchMetrics(metrics), ifaces);
}

export function buildSwitchSystemInfo(metrics: Record<string, unknown> | undefined, kpis: MikrotikNocKpis): MikrotikSystemInfo {
  const mapped = mapSwitchMetrics(metrics);
  const base = buildMikrotikSystemInfo(mapped, kpis);
  const descr =
    String((mapped?.mikrotik_collection as { fields?: Record<string, { value?: unknown }> } | undefined)?.fields?.sys_descr?.value ?? "")
      .trim() || base.model;
  if (base.version === EM_DASH && descr) {
    const ver = descr.match(/Version\s+([^,]+)/i);
    if (ver?.[1]) base.version = ver[1].trim();
  }
  if (base.model === EM_DASH && descr) {
    base.model = descr.split(",")[0]?.trim() || descr;
  }
  if (base.board === EM_DASH && base.model !== EM_DASH) {
    base.board = base.model;
  }
  return base;
}

export function parseSwitchCollectionStatus(metrics: Record<string, unknown> | undefined) {
  const raw = metrics?.switch_collection as Record<string, unknown> | undefined;
  if (!raw) return null;
  const status = raw.status as Record<string, unknown> | undefined;
  return {
    message: typeof status?.message === "string" ? status.message : undefined,
    missingOid: Array.isArray(status?.missing_oid) ? (status!.missing_oid as string[]) : [],
    collected: typeof status?.collected === "number" ? status.collected : 0,
    failed: typeof status?.failed === "number" ? status.failed : 0,
  };
}
