/** Parsing local de coordenadas / URLs de mapa (antes de chamar a API). */

const LAT_LNG_RE =
  /^\s*(-?\d{1,3}(?:[.,]\d+)?)\s*[,;\s]\s*(-?\d{1,3}(?:[.,]\d+)?)\s*$/i;

const MAPS_AT_RE = /@(-?\d{1,3}(?:\.\d+)?),\s*(-?\d{1,3}(?:\.\d+)?)/i;
const MAPS_Q_RE = /[?&](?:q|query|ll)=(-?\d{1,3}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)/i;
const MAPS_3D4D_RE = /!3d(-?\d{1,3}(?:\.\d+)?)!4d(-?\d{1,3}(?:\.\d+)?)/i;
const OSM_HASH_RE = /#map=\d+\/(-?\d{1,3}(?:\.\d+)?)\/(-?\d{1,3}(?:\.\d+)?)/i;

function parseFloatLoose(s: string): number | null {
  const n = Number(String(s).trim().replace(",", "."));
  return Number.isFinite(n) ? n : null;
}

function validLatLng(lat: number, lng: number): boolean {
  return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180;
}

export function parseLatLngPair(raw: string): { lat: number; lng: number } | null {
  const m = LAT_LNG_RE.exec(String(raw ?? "").trim());
  if (!m) return null;
  const lat = parseFloatLoose(m[1]);
  const lng = parseFloatLoose(m[2]);
  if (lat == null || lng == null || !validLatLng(lat, lng)) return null;
  return { lat, lng };
}

export function parseCoordsFromMapsURL(raw: string): { lat: number; lng: number } | null {
  const s = String(raw ?? "").trim();
  if (!s) return null;
  for (const re of [MAPS_AT_RE, MAPS_Q_RE, MAPS_3D4D_RE, OSM_HASH_RE]) {
    const m = re.exec(s);
    if (!m) continue;
    const lat = parseFloatLoose(m[1]);
    const lng = parseFloatLoose(m[2]);
    if (lat != null && lng != null && validLatLng(lat, lng)) return { lat, lng };
  }
  try {
    const u = new URL(s);
    for (const key of ["q", "query", "ll"]) {
      const v = u.searchParams.get(key);
      if (v) {
        const pair = parseLatLngPair(v);
        if (pair) return pair;
      }
    }
  } catch {
    /* ignore */
  }
  return null;
}

export function looksLikeHTTPURL(raw: string): boolean {
  const s = String(raw ?? "").trim().toLowerCase();
  return s.startsWith("http://") || s.startsWith("https://");
}

/** Deve ir a /map/locate em vez de (ou além de) /map/search. */
export function shouldLocateQuery(raw: string): boolean {
  const s = String(raw ?? "").trim();
  if (!s) return false;
  if (parseLatLngPair(s)) return true;
  if (looksLikeHTTPURL(s)) return true;
  // Endereço / localidade: evita prefixos de pesquisa de elementos (cto:, login:…).
  if (/^(cto|poste|login|logins|equip|equipamento|equipamentos|infra)\s*:/i.test(s)) return false;
  return s.length >= 3;
}

export type MapLocateHit = {
  lat: number;
  lng: number;
  label: string;
  source: "coords" | "maps_url" | "geocode" | string;
  display?: string;
};
