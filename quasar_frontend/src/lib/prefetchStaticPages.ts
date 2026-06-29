import type { QueryClient } from "@tanstack/react-query";
import { apiFetch } from "./api";
import { DASHBOARD_GC_MS, DASHBOARD_STALE_MS, prefetchDashboard } from "./dashboardCache";
import { NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "./networkInfraCache";
import { PAGE_DATA_GC_MS, PAGE_DATA_STALE_MS, prefetchPageCachedQuery } from "./pageDataCache";
import { queryKeys } from "./queryKeys";

/** Pré-carrega páginas com dados pouco voláteis (login / shell). */
export async function prefetchStaticPages(qc: QueryClient): Promise<void> {
  const month = `${new Date().getFullYear()}-${String(new Date().getMonth() + 1).padStart(2, "0")}`;
  await Promise.allSettled([
    prefetchDashboard(qc),
    prefetchPageCachedQuery(
      qc,
      queryKeys.pops,
      () => apiFetch("/api/v1/pops"),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.commercialLocalities,
      () => apiFetch("/api/v1/commercial/localities"),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.commercialRecords,
      () => apiFetch("/api/v1/commercial/monthly-records"),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.commercialTotalsHistory,
      () => apiFetch("/api/v1/commercial/totals-history?months=18"),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.commercialAgg(month),
      () => apiFetch(`/api/v1/commercial/aggregates?month=${encodeURIComponent(month)}`),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.commercialCmp(month),
      () => apiFetch(`/api/v1/commercial/comparison?month=${encodeURIComponent(month)}`),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.clientConnectionsList,
      () => apiFetch("/api/v1/commercial/connections"),
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.networkProjects,
      () => apiFetch("/api/v1/commercial/network/projects"),
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.networkCtos,
      () => apiFetch("/api/v1/commercial/network/ctos"),
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.networkSpliceBoxes,
      () => apiFetch("/api/v1/commercial/network/splice-boxes"),
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.networkCables,
      () => apiFetch("/api/v1/commercial/network/cables"),
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
    prefetchPageCachedQuery(
      qc,
      queryKeys.networkPoles,
      () => apiFetch("/api/v1/commercial/network/poles"),
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
  ]);
}

export { DASHBOARD_STALE_MS, DASHBOARD_GC_MS };
