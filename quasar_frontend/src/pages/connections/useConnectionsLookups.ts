import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import { NETWORK_INFRA_GC_MS, NETWORK_INFRA_STALE_MS } from "../../lib/networkInfraCache";
import { pageCachedQueryOptions, PAGE_DATA_GC_MS, PAGE_DATA_STALE_MS, wrapPageCachedQueryFn } from "../../lib/pageDataCache";
import { queryKeys } from "../../lib/queryKeys";
import type { CommercialLocality, NetworkProject } from "../../lib/networkInfrastructure";

/** Localidades (Clientes) e projetos — cache partilhado entre abas e filtros. */
export function useConnectionsLookups(enabled = true) {
  const localitiesQ = useQuery({
    queryKey: queryKeys.commercialLocalities,
    queryFn: wrapPageCachedQueryFn(queryKeys.commercialLocalities, () =>
      apiFetch<{ localities: CommercialLocality[] }>("/api/v1/commercial/localities"),
    ),
    enabled,
    ...pageCachedQueryOptions<{ localities: CommercialLocality[] }>(
      queryKeys.commercialLocalities,
      PAGE_DATA_STALE_MS,
      PAGE_DATA_GC_MS,
    ),
  });
  const projectsQ = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: wrapPageCachedQueryFn(queryKeys.networkProjects, () =>
      apiFetch<{ projects: NetworkProject[] }>("/api/v1/commercial/network/projects"),
    ),
    enabled,
    ...pageCachedQueryOptions<{ projects: NetworkProject[] }>(
      queryKeys.networkProjects,
      NETWORK_INFRA_STALE_MS,
      NETWORK_INFRA_GC_MS,
    ),
  });
  return {
    localities: localitiesQ.data?.localities ?? [],
    projects: projectsQ.data?.projects ?? [],
    isLoading: localitiesQ.isLoading || projectsQ.isLoading,
  };
}
