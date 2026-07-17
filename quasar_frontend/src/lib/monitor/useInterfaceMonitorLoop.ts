import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../api";
import type { InterfaceSnapshotResponse, MonitorInterfaceRow } from "./types";

const SNMP_REFRESH_POLL_MS = 2000;

type Options = {
  deviceId: string | null;
  queryKey: readonly [string, string];
  canMutate: boolean;
  onTable: (rows: MonitorInterfaceRow[]) => void;
  enabled?: boolean;
  /** Se true, faz POST /refresh periódico (SNMP). Por omissão só lê o snapshot uma vez. */
  snmpAutoRefresh?: boolean;
  snmpRefreshMs?: number;
};

/**
 * Monitoramento de interfaces.
 * Por omissão: um único GET do snapshot em cache (sem loop — evita repor os mesmos bps no gráfico).
 * Com snmpAutoRefresh: POST /refresh periódico (admin).
 *
 * Nota: onTable / queryKey não devem rearmar o efeito a cada render (causava flood de GET no backend).
 */
export function useInterfaceMonitorLoop({
  deviceId,
  queryKey,
  canMutate,
  onTable,
  enabled = true,
  snmpAutoRefresh = false,
  snmpRefreshMs = 30_000,
}: Options) {
  const qc = useQueryClient();
  const onTableRef = useRef(onTable);
  onTableRef.current = onTable;
  const queryKeyRef = useRef(queryKey);
  queryKeyRef.current = queryKey;
  const qk0 = queryKey[0];
  const qk1 = queryKey[1];

  useEffect(() => {
    if (!enabled || !deviceId) return;

    let cancelled = false;
    let lastSnmpRefresh = 0;

    const readCache = async () => {
      const data = await apiFetch<InterfaceSnapshotResponse>(`/api/v1/interfaces/devices/${deviceId}`);
      if (cancelled) return;
      const rows = (data.interface_table ?? []) as MonitorInterfaceRow[];
      onTableRef.current(rows);
      qc.setQueryData(queryKeyRef.current, data);
    };

    const snmpRefresh = async () => {
      const data = await apiFetch<InterfaceSnapshotResponse>(
        `/api/v1/interfaces/devices/${deviceId}/refresh`,
        { method: "POST", json: {} },
      );
      if (cancelled) return;
      const rows = (data.interface_table ?? []) as MonitorInterfaceRow[];
      onTableRef.current(rows);
      qc.setQueryData(queryKeyRef.current, data);
      lastSnmpRefresh = Date.now();
    };

    const run = async () => {
      try {
        await readCache();
      } catch {
        /* ignore */
      }
      if (!snmpAutoRefresh || !canMutate) return;
      while (!cancelled) {
        await new Promise((r) => setTimeout(r, SNMP_REFRESH_POLL_MS));
        if (cancelled) break;
        try {
          const now = Date.now();
          if (now - lastSnmpRefresh >= snmpRefreshMs) {
            await snmpRefresh();
          }
        } catch {
          /* falhas transitórias */
        }
      }
    };

    void run();
    return () => {
      cancelled = true;
    };
  }, [deviceId, qk0, qk1, canMutate, enabled, snmpAutoRefresh, snmpRefreshMs, qc]);
}
