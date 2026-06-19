import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import type { CommercialLocality, NetworkProject } from "../../lib/networkInfrastructure";

const LOOKUP_STALE_MS = 5 * 60 * 1000;

/** Localidades (Clientes) e projetos — cache partilhado entre abas e filtros. */
export function useConnectionsLookups(enabled = true) {
  const localitiesQ = useQuery({
    queryKey: queryKeys.commercialLocalities,
    queryFn: () => apiFetch<{ localities: CommercialLocality[] }>("/api/v1/commercial/localities"),
    enabled,
    staleTime: LOOKUP_STALE_MS,
  });
  const projectsQ = useQuery({
    queryKey: queryKeys.networkProjects,
    queryFn: () => apiFetch<{ projects: NetworkProject[] }>("/api/v1/commercial/network/projects"),
    enabled,
    staleTime: LOOKUP_STALE_MS,
  });
  return {
    localities: localitiesQ.data?.localities ?? [],
    projects: projectsQ.data?.projects ?? [],
    isLoading: localitiesQ.isLoading || projectsQ.isLoading,
  };
}
