import { useEffect, useRef } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { invalidateAlertListQueries, queryKeys } from "./queryKeys";

/** Campos de GET /api/v1/monitoring/state usados para invalidar caches após coletas. */
export type MonitoringStateSync = {
  is_running?: boolean;
  runtime_updated_at?: string | null;
  last_alerts_change_at?: string | null;
  last_telemetry_cycle_at?: string | null;
  last_latency_cycle_at?: string | null;
  last_interface_snapshot_cycle_at?: string | null;
  last_olt_if_derived_cycle_at?: string | null;
  activity_updated_at?: string | null;
};

function syncSignature(s: MonitoringStateSync | undefined): string {
  if (!s) return "";
  return [
    s.runtime_updated_at ?? "",
    s.last_alerts_change_at ?? "",
    s.last_telemetry_cycle_at ?? "",
    s.last_latency_cycle_at ?? "",
    s.last_interface_snapshot_cycle_at ?? "",
    s.last_olt_if_derived_cycle_at ?? "",
    s.activity_updated_at ?? "",
  ].join("|");
}

export type MonitoringLiveSyncOptions = {
  monitoring?: boolean;
  alerts?: boolean;
  olt?: boolean;
};

/** Invalida listas quando o worker grava novas leituras (telemetria, ping, alertas, OLT). */
export function applyMonitoringLiveSync(
  qc: QueryClient,
  opts: MonitoringLiveSyncOptions = {},
): void {
  const { monitoring = true, alerts = true, olt = false } = opts;
  if (monitoring) {
    void qc.invalidateQueries({ queryKey: queryKeys.monitoringActiveEquipment });
  }
  if (alerts) {
    void invalidateAlertListQueries(qc);
  }
  if (olt) {
    void qc.invalidateQueries({ queryKey: queryKeys.oltDevices });
    void qc.invalidateQueries({ queryKey: ["olt-device"] });
  }
}

export function useMonitoringLiveSync(
  monState: MonitoringStateSync | undefined,
  opts: MonitoringLiveSyncOptions = {},
): void {
  const qc = useQueryClient();
  const prevSig = useRef("");
  const debounceRef = useRef<number | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  useEffect(() => {
    const sig = syncSignature(monState);
    if (!sig || sig === prevSig.current) return;
    prevSig.current = sig;
    if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      debounceRef.current = null;
      applyMonitoringLiveSync(qc, optsRef.current);
    }, 280);
    return () => {
      if (debounceRef.current != null) {
        window.clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
    };
  }, [monState, qc]);
}

/** Intervalo de polling quando o monitoramento está ligado (mín. 2,5 s). */
export function monitoringPollMs(baseMs: number, isRunning: boolean | undefined): number {
  if (!isRunning) return baseMs;
  return Math.min(Math.max(baseMs, 2500), 3000);
}
