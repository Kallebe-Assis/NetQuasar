import type { QueryClient, QueryKey } from "@tanstack/react-query";

const STORAGE_PREFIX = "netquasar:page-cache:";

/** Dados estáticos/semi-estáticos: POP, Clientes, Conexões, lookups. */
export const PAGE_DATA_STALE_MS = 15 * 60 * 1000;
export const PAGE_DATA_GC_MS = 45 * 60 * 1000;

type CacheEntry<T> = { data: T; savedAt: number };

function cacheStorageKey(queryKey: QueryKey): string {
  return STORAGE_PREFIX + JSON.stringify(queryKey);
}

export function readPageDataCache<T>(queryKey: QueryKey, maxAgeMs: number): { data: T; updatedAt: number } | undefined {
  try {
    const raw = sessionStorage.getItem(cacheStorageKey(queryKey));
    if (!raw) return undefined;
    const entry = JSON.parse(raw) as CacheEntry<T>;
    if (!entry || entry.savedAt == null || entry.data === undefined) return undefined;
    if (Date.now() - entry.savedAt > maxAgeMs) {
      sessionStorage.removeItem(cacheStorageKey(queryKey));
      return undefined;
    }
    return { data: entry.data, updatedAt: entry.savedAt };
  } catch {
    return undefined;
  }
}

export function writePageDataCache<T>(queryKey: QueryKey, data: T): void {
  try {
    const entry: CacheEntry<T> = { data, savedAt: Date.now() };
    sessionStorage.setItem(cacheStorageKey(queryKey), JSON.stringify(entry));
  } catch {
    /* quota ou modo privado */
  }
}

export function removePageDataCache(queryKey: QueryKey): void {
  try {
    sessionStorage.removeItem(cacheStorageKey(queryKey));
  } catch {
    /* ignore */
  }
}

/** Envolve queryFn para gravar em sessionStorage após fetch bem-sucedido. */
export function wrapPageCachedQueryFn<T>(queryKey: QueryKey, fn: () => Promise<T>): () => Promise<T> {
  return async () => {
    const data = await fn();
    writePageDataCache(queryKey, data);
    return data;
  };
}

/** Opções partilhadas: stale longo, hidratação a partir de sessionStorage. */
export function pageCachedQueryOptions<T>(
  queryKey: QueryKey,
  staleMs = PAGE_DATA_STALE_MS,
  gcMs = PAGE_DATA_GC_MS,
) {
  const cached = readPageDataCache<T>(queryKey, staleMs);
  return {
    staleTime: staleMs,
    gcTime: gcMs,
    refetchOnWindowFocus: false,
    initialData: cached?.data,
    initialDataUpdatedAt: cached?.updatedAt,
  };
}

export async function prefetchPageCachedQuery<T>(
  qc: QueryClient,
  queryKey: QueryKey,
  queryFn: () => Promise<T>,
  staleMs = PAGE_DATA_STALE_MS,
  gcMs = PAGE_DATA_GC_MS,
): Promise<void> {
  const cached = readPageDataCache<T>(queryKey, staleMs);
  if (cached) {
    qc.setQueryData(queryKey, cached.data, { updatedAt: cached.updatedAt });
  }
  await qc.prefetchQuery({
    queryKey,
    queryFn: wrapPageCachedQueryFn(queryKey, queryFn),
    staleTime: staleMs,
    gcTime: gcMs,
  });
}

/** Invalida React Query e apaga entrada em sessionStorage. */
export function invalidatePageCachedQuery(qc: QueryClient, queryKey: QueryKey): Promise<void> {
  removePageDataCache(queryKey);
  return qc.invalidateQueries({ queryKey });
}
