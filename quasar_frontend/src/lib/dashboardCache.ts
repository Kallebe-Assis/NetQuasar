import type { QueryClient } from "@tanstack/react-query";
import { apiFetch } from "./api";

export const DASHBOARD_DEFAULT_DAYS = 30;
/** Cache longo: dados pré-carregados no login e reutilizados na página. */
export const DASHBOARD_STALE_MS = 30 * 60 * 1000;
export const DASHBOARD_GC_MS = 60 * 60 * 1000;

export function dashboardAnalyticsKey(days: number) {
  return ["dashboard-analytics", days] as const;
}

export const dashboardTopLatencyKey = ["top-latency"] as const;
export const dashboardOltCapacityKey = ["dashboard-olt-capacity"] as const;

export async function prefetchDashboard(qc: QueryClient, days = DASHBOARD_DEFAULT_DAYS): Promise<void> {
  await Promise.all([
    qc.prefetchQuery({
      queryKey: dashboardAnalyticsKey(days),
      queryFn: () => apiFetch(`/api/v1/dashboard/analytics?days=${days}`),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.prefetchQuery({
      queryKey: dashboardTopLatencyKey,
      queryFn: () => apiFetch<{ top: unknown[] }>("/api/v1/overview/top-latency?limit=8"),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.prefetchQuery({
      queryKey: dashboardOltCapacityKey,
      queryFn: () => apiFetch("/api/v1/dashboard/olt-capacity"),
      staleTime: DASHBOARD_STALE_MS,
    }),
  ]);
}

/** Recarrega todas as fontes do dashboard (ignora cache). */
export async function refreshDashboard(qc: QueryClient, days = DASHBOARD_DEFAULT_DAYS): Promise<void> {
  await Promise.all([
    qc.fetchQuery({
      queryKey: dashboardAnalyticsKey(days),
      queryFn: () => apiFetch(`/api/v1/dashboard/analytics?days=${days}`),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.fetchQuery({
      queryKey: dashboardTopLatencyKey,
      queryFn: () => apiFetch<{ top: unknown[] }>("/api/v1/overview/top-latency?limit=8"),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.fetchQuery({
      queryKey: dashboardOltCapacityKey,
      queryFn: () => apiFetch("/api/v1/dashboard/olt-capacity"),
      staleTime: DASHBOARD_STALE_MS,
    }),
  ]);
}

/** Após coleta OLT/MikroTik — actualiza gráficos do dashboard se estiverem abertos. */
export function invalidateDashboardAfterCollect(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: ["dashboard-analytics"], refetchType: "active" });
  void qc.invalidateQueries({ queryKey: dashboardTopLatencyKey, refetchType: "active" });
  void qc.invalidateQueries({ queryKey: dashboardOltCapacityKey, refetchType: "active" });
  void qc.invalidateQueries({ queryKey: ["olt-reports-history"], refetchType: "active" });
}
