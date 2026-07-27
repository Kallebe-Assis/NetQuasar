import type { QueryClient } from "@tanstack/react-query";
import { apiFetch } from "./api";
import { removePageDataCache, wrapPageCachedQueryFn } from "./pageDataCache";

/** Janela mais leve por omissão — 7 dias em vez de 30. */
export const DASHBOARD_DEFAULT_DAYS = 7;
/** Cache longo no browser: reabre o dashboard instantaneamente. */
export const DASHBOARD_STALE_MS = 30 * 60 * 1000;
export const DASHBOARD_GC_MS = 60 * 60 * 1000;

export function dashboardAnalyticsKey(days: number) {
  return ["dashboard-analytics", days] as const;
}

export const dashboardTopLatencyKey = ["top-latency"] as const;
export const dashboardOltCapacityKey = ["dashboard-olt-capacity"] as const;

function dashAnalyticsFn(days: number, refresh = false) {
  const q = refresh ? `?days=${days}&refresh=1` : `?days=${days}`;
  return wrapPageCachedQueryFn(dashboardAnalyticsKey(days), () =>
    apiFetch(`/api/v1/dashboard/analytics${q}`),
  );
}

function dashTopLatencyFn() {
  return wrapPageCachedQueryFn(dashboardTopLatencyKey, () =>
    apiFetch<{ top: unknown[] }>("/api/v1/overview/top-latency?limit=8"),
  );
}

function dashOltCapacityFn(refresh = false) {
  const q = refresh ? "?refresh=1" : "";
  return wrapPageCachedQueryFn(dashboardOltCapacityKey, () => apiFetch(`/api/v1/dashboard/olt-capacity${q}`));
}

export async function prefetchDashboard(qc: QueryClient, days = DASHBOARD_DEFAULT_DAYS): Promise<void> {
  await Promise.all([
    qc.prefetchQuery({
      queryKey: dashboardAnalyticsKey(days),
      queryFn: dashAnalyticsFn(days),
      staleTime: DASHBOARD_STALE_MS,
      gcTime: DASHBOARD_GC_MS,
    }),
    qc.prefetchQuery({
      queryKey: dashboardTopLatencyKey,
      queryFn: dashTopLatencyFn(),
      staleTime: DASHBOARD_STALE_MS,
      gcTime: DASHBOARD_GC_MS,
    }),
    qc.prefetchQuery({
      queryKey: dashboardOltCapacityKey,
      queryFn: dashOltCapacityFn(),
      staleTime: DASHBOARD_STALE_MS,
      gcTime: DASHBOARD_GC_MS,
    }),
  ]);
}

/** Recarrega todas as fontes do dashboard (ignora cache browser + servidor). */
export async function refreshDashboard(qc: QueryClient, days = DASHBOARD_DEFAULT_DAYS): Promise<void> {
  removePageDataCache(dashboardAnalyticsKey(days));
  removePageDataCache(dashboardTopLatencyKey);
  removePageDataCache(dashboardOltCapacityKey);
  await Promise.all([
    qc.fetchQuery({
      queryKey: dashboardAnalyticsKey(days),
      queryFn: dashAnalyticsFn(days, true),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.fetchQuery({
      queryKey: dashboardTopLatencyKey,
      queryFn: dashTopLatencyFn(),
      staleTime: DASHBOARD_STALE_MS,
    }),
    qc.fetchQuery({
      queryKey: dashboardOltCapacityKey,
      queryFn: dashOltCapacityFn(true),
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
