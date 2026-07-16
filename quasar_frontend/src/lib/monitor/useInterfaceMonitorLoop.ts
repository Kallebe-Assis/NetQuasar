import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../api";
import type { InterfaceSnapshotResponse, MonitorInterfaceRow } from "./types";

const CACHE_POLL_MS = 2000;
const SNMP_REFRESH_MS = 30_000;

type Options = {
  deviceId: string | null;
  queryKey: readonly [string, string];
  canMutate: boolean;
  onTable: (rows: MonitorInterfaceRow[]) => void;
  enabled?: boolean;
};

/**
 * Loop de monitoramento de interfaces:
 * - GET cache a cada 2s (leve)
 * - POST SNMP refresh a cada 30s quando admin (evita INSERT a cada 2s)
 */
export function useInterfaceMonitorLoop({ deviceId, queryKey, canMutate, onTable, enabled = true }: Options) {
  const qc = useQueryClient();

  useEffect(() => {
    if (!enabled || !deviceId) return;

    let cancelled = false;
    let lastSnmpRefresh = 0;

    const readCache = async () => {
      const data = await apiFetch<InterfaceSnapshotResponse>(`/api/v1/interfaces/devices/${deviceId}`);
      if (cancelled) return;
      const rows = (data.interface_table ?? []) as MonitorInterfaceRow[];
      onTable(rows);
      qc.setQueryData(queryKey, data);
    };

    const snmpRefresh = async () => {
      const data = await apiFetch<InterfaceSnapshotResponse>(
        `/api/v1/interfaces/devices/${deviceId}/refresh`,
        { method: "POST", json: {} },
      );
      if (cancelled) return;
      const rows = (data.interface_table ?? []) as MonitorInterfaceRow[];
      onTable(rows);
      qc.setQueryData(queryKey, data);
      lastSnmpRefresh = Date.now();
    };

    const loop = async () => {
      while (!cancelled) {
        try {
          const now = Date.now();
          if (canMutate && now - lastSnmpRefresh >= SNMP_REFRESH_MS) {
            await snmpRefresh();
          } else {
            await readCache();
          }
        } catch {
          /* falhas transitórias no ciclo automático */
        }
        if (cancelled) break;
        await new Promise((r) => setTimeout(r, CACHE_POLL_MS));
      }
    };

    void loop();
    return () => {
      cancelled = true;
    };
  }, [deviceId, queryKey, canMutate, onTable, enabled, qc]);
}
