/**
 * Comparação geográfica entre logins (client_connections) e CTOs.
 * Para cada login com coordenadas, encontra a CTO mais próxima com base em lat/long (Haversine).
 */

export type GeoPoint = {
  latitude: number;
  longitude: number;
};

export type NullableGeoPoint = {
  latitude?: number | null;
  longitude?: number | null;
};

export type CtoCandidate = NullableGeoPoint & {
  id?: string;
  display_number?: number;
  description?: string;
  latitude?: number | null;
  longitude?: number | null;
};

export type LoginWithCoords = NullableGeoPoint & {
  id?: string;
  display_number?: number;
  login?: string;
  client_name?: string;
  cto?: string | null;
};

export type NearestCtoMatch = {
  login: LoginWithCoords;
  nearestCto: CtoCandidate | null;
  /** Distância em metros; null se login ou CTOs não tiverem coordenadas válidas. */
  distanceMeters: number | null;
};

const EARTH_RADIUS_M = 6_371_000;

/** Verifica se lat/long são finitas e dentro dos intervalos válidos. */
export function isValidGeoPoint(p: NullableGeoPoint | null | undefined): p is GeoPoint {
  if (p == null) return false;
  const lat = p.latitude;
  const lon = p.longitude;
  if (lat == null || lon == null) return false;
  if (!Number.isFinite(lat) || !Number.isFinite(lon)) return false;
  if (lat < -90 || lat > 90) return false;
  if (lon < -180 || lon > 180) return false;
  return true;
}

function toRad(deg: number): number {
  return (deg * Math.PI) / 180;
}

/**
 * Distância em linha recta na superfície terrestre (Haversine), em metros.
 */
export function haversineDistanceMeters(a: GeoPoint, b: GeoPoint): number {
  const dLat = toRad(b.latitude - a.latitude);
  const dLon = toRad(b.longitude - a.longitude);
  const lat1 = toRad(a.latitude);
  const lat2 = toRad(b.latitude);
  const h =
    Math.sin(dLat / 2) ** 2 + Math.cos(lat1) * Math.cos(lat2) * Math.sin(dLon / 2) ** 2;
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(h)));
}

/**
 * Compara um ponto (login) com todas as CTOs e devolve a mais próxima.
 * CTOs sem coordenadas válidas são ignoradas.
 */
export function findNearestCto(
  point: NullableGeoPoint,
  ctos: CtoCandidate[],
): { cto: CtoCandidate | null; distanceMeters: number | null } {
  if (!isValidGeoPoint(point)) {
    return { cto: null, distanceMeters: null };
  }

  let best: CtoCandidate | null = null;
  let bestDist = Infinity;

  for (const cto of ctos) {
    if (!isValidGeoPoint(cto)) continue;
    const dist = haversineDistanceMeters(point, cto);
    if (dist < bestDist) {
      bestDist = dist;
      best = cto;
    }
  }

  if (best == null) {
    return { cto: null, distanceMeters: null };
  }
  return { cto: best, distanceMeters: bestDist };
}

/**
 * Para cada login com coordenadas, encontra a única CTO mais próxima entre todas as CTOs disponíveis.
 */
export function matchLoginsToNearestCtos(
  logins: LoginWithCoords[],
  ctos: CtoCandidate[],
): NearestCtoMatch[] {
  return logins.map((login) => {
    const { cto, distanceMeters } = findNearestCto(login, ctos);
    return {
      login,
      nearestCto: cto,
      distanceMeters,
    };
  });
}

/** Formata distância para exibição (metros ou km). */
export function formatDistanceMeters(meters: number | null | undefined): string {
  if (meters == null || !Number.isFinite(meters)) return "—";
  if (meters < 1000) return `${Math.round(meters)} m`;
  return `${(meters / 1000).toFixed(2)} km`;
}

/** Rótulo legível da CTO (descrição ou número). */
export function ctoLabel(cto: CtoCandidate | null | undefined): string {
  if (!cto) return "—";
  if (cto.description?.trim()) return cto.description.trim();
  if (cto.display_number != null) return `CTO ${cto.display_number}`;
  return cto.id ?? "—";
}

export const NEAREST_CTO_CSV_HEADER = [
  "login",
  "cliente",
  "latitude",
  "longitude",
  "cto_atual",
  "cto_sugerida",
  "cto_numero",
  "distancia_m",
] as const;

/** Linhas CSV da comparação login ↔ CTO mais próxima. */
export function nearestCtoMatchesToCsvRows(matches: NearestCtoMatch[]): string[][] {
  const rows: string[][] = [NEAREST_CTO_CSV_HEADER.slice()];
  for (const m of matches) {
    const lat = m.login.latitude;
    const lon = m.login.longitude;
    rows.push([
      m.login.login ?? "",
      m.login.client_name ?? "",
      lat != null && Number.isFinite(lat) ? String(lat) : "",
      lon != null && Number.isFinite(lon) ? String(lon) : "",
      m.login.cto ?? "",
      ctoLabel(m.nearestCto),
      m.nearestCto?.display_number != null ? String(m.nearestCto.display_number) : "",
      m.distanceMeters != null && Number.isFinite(m.distanceMeters) ? String(Math.round(m.distanceMeters)) : "",
    ]);
  }
  return rows;
}
