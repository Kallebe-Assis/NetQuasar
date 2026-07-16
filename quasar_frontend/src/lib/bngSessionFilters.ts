import {
  formatBngIpType,
  formatBngKbitRate,
  sessionDisplayOnline,
  type PppoeSessionFields,
} from "./bngDisplay";

export type BngSessionSearchField = "login" | "ipv4" | "ipv6" | "mac" | "vlan";

/** Filtro por tipo IP exibido na coluna (mesma lógica de formatBngIpType). */
export type BngIpTypeFilter = "any" | "ipv4" | "ipv6" | "dual";

/** @deprecated use BngIpTypeFilter — mantido para compatibilidade de imports. */
export type BngDualStackFilter = "any" | "yes" | "no";

export type BngSessionAdvancedFilters = {
  ipv4Like: string;
  /** Preferido: any | ipv4 | ipv6 | dual */
  ipType: BngIpTypeFilter;
  /** @deprecated mapeado a partir de dualStack legado se presente */
  dualStack?: BngDualStackFilter;
  vlans: string;
  minOnlineSec: string;
  dnLimitKbps: string;
};

export const BNG_SESSION_SEARCH_FIELDS: { value: BngSessionSearchField; label: string }[] = [
  { value: "login", label: "Login" },
  { value: "ipv4", label: "IPv4" },
  { value: "ipv6", label: "IPv6" },
  { value: "mac", label: "MAC" },
  { value: "vlan", label: "VLAN" },
];

export const EMPTY_BNG_SESSION_FILTERS: BngSessionAdvancedFilters = {
  ipv4Like: "",
  ipType: "any",
  vlans: "",
  minOnlineSec: "",
  dnLimitKbps: "",
};

export type BngSessionLike = PppoeSessionFields & {
  index?: string;
  login?: string;
  ipv4?: string;
  ipv6?: string;
  ipv6_pd?: string;
  mac?: string;
  vlan?: string;
  status?: string;
  auth_state?: string;
  ip_type?: string;
  ip_type_raw?: string;
  online_time_sec?: string;
  car_dn_cir_kbps?: string;
};

/** Normaliza MAC para comparação parcial (aceita :, -, . ou compacto). */
export function normalizeMacDigits(raw: string): string {
  return raw.toLowerCase().replace(/[^0-9a-f]/g, "");
}

/** Extrai IDs numéricos de VLAN (QinQ, «VLAN 743», etc.). */
export function extractVlanIds(raw: string): string[] {
  const s = String(raw ?? "").trim();
  if (!s) return [];
  const nums = s.match(/\d+/g);
  return nums ?? [];
}

/**
 * Match de VLAN: se o termo for só dígitos, exige ID exacto (743 ≠ 1743).
 * Caso contrário, igualdade exacta case-insensitive ou contains do texto.
 */
export function vlanMatchesNeedle(sessionVlan: string, needle: string): boolean {
  const n = needle.trim();
  if (!n) return true;
  const sv = String(sessionVlan ?? "").trim();
  if (!sv) return false;
  if (sv === n || sv.toLowerCase() === n.toLowerCase()) return true;

  if (/^\d+$/.test(n)) {
    return extractVlanIds(sv).includes(n);
  }

  return sv.toLowerCase().includes(n.toLowerCase());
}

/** Normaliza IPv4 (dotted ou hex SNMP tipo c0:a8:01:01) para comparação. */
export function normalizeBngIpv4ForMatch(raw: string): string {
  const s = String(raw ?? "").trim().toLowerCase();
  if (!s || s === "0.0.0.0") return "";
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(s)) return s;
  const colon = s.split(":");
  if (colon.length === 4 && colon.every((p) => /^[0-9a-f]{1,2}$/i.test(p))) {
    return colon.map((p) => String(parseInt(p, 16))).join(".");
  }
  const compact = s.replace(/[^0-9a-f]/gi, "");
  if (compact.length === 8 && /^[0-9a-f]+$/i.test(compact)) {
    const octets: string[] = [];
    for (let i = 0; i < 8; i += 2) {
      octets.push(String(parseInt(compact.slice(i, i + 2), 16)));
    }
    return octets.join(".");
  }
  return s;
}

function ipv4MatchesNeedle(sessionIpv4: string, needle: string): boolean {
  const q = needle.trim().toLowerCase();
  if (!q) return true;
  const raw = String(sessionIpv4 ?? "").trim();
  if (!raw) return false;
  const norm = normalizeBngIpv4ForMatch(raw);
  const needleNorm = normalizeBngIpv4ForMatch(q);
  if (norm && (norm === needleNorm || norm.includes(q) || (needleNorm && norm.includes(needleNorm)))) {
    return true;
  }
  return raw.toLowerCase().includes(q);
}

function sessionIpv6Haystack(s: BngSessionLike): string {
  return `${s.ipv6 ?? ""} ${s.ipv6_pd ?? ""}`.toLowerCase();
}

function sessionOnlineSeconds(s: BngSessionLike): number {
  const n = Number(String(s.online_time_sec ?? "").trim());
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function sessionDnLimitKbps(s: BngSessionLike): number {
  const n = Number(String(s.car_dn_cir_kbps ?? "").trim());
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/** Tipo IP canónico alinhado com a coluna da tabela. */
export function sessionCanonicalIpType(s: BngSessionLike): "ipv4" | "ipv6" | "dual" | "" {
  const t = formatBngIpType(s.ip_type, s.ip_type_raw, s).toLowerCase();
  if (t === "ipv4/v6" || t.includes("dual")) return "dual";
  if (t === "ipv4" || t.startsWith("ipv4")) return "ipv4";
  if (t === "ipv6" || t.startsWith("ipv6")) return "ipv6";
  return "";
}

function resolveIpTypeFilter(filters: BngSessionAdvancedFilters): BngIpTypeFilter {
  if (filters.ipType != null) return filters.ipType;
  // Legado dualStack → ipType
  if (filters.dualStack === "yes") return "dual";
  if (filters.dualStack === "no") return "ipv4";
  return "any";
}

export function matchBngSessionSearch(s: BngSessionLike, field: BngSessionSearchField, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  switch (field) {
    case "login": {
      const login = String(s.login ?? "").toLowerCase();
      return login === q;
    }
    case "ipv4":
      return ipv4MatchesNeedle(String(s.ipv4 ?? ""), q);
    case "ipv6":
      return sessionIpv6Haystack(s).includes(q);
    case "mac": {
      const hay = normalizeMacDigits(String(s.mac ?? ""));
      const needle = normalizeMacDigits(q);
      return needle.length > 0 && hay.includes(needle);
    }
    case "vlan":
      return vlanMatchesNeedle(String(s.vlan ?? ""), q);
    default:
      return true;
  }
}

function parseVlanList(raw: string): string[] {
  return raw
    .split(/[,;\s]+/)
    .map((v) => v.trim())
    .filter(Boolean);
}

export function applyBngSessionAdvancedFilters(s: BngSessionLike, filters: BngSessionAdvancedFilters): boolean {
  const ipv4Like = filters.ipv4Like.trim();
  if (ipv4Like && !ipv4MatchesNeedle(String(s.ipv4 ?? ""), ipv4Like)) {
    return false;
  }

  const ipType = resolveIpTypeFilter(filters);
  if (ipType !== "any") {
    const got = sessionCanonicalIpType(s);
    if (ipType === "dual" && got !== "dual") return false;
    if (ipType === "ipv4" && got !== "ipv4") return false;
    if (ipType === "ipv6" && got !== "ipv6") return false;
  }

  const vlans = parseVlanList(filters.vlans);
  if (vlans.length > 0) {
    if (!vlans.some((v) => vlanMatchesNeedle(String(s.vlan ?? ""), v))) return false;
  }

  const minSec = Number(filters.minOnlineSec.trim());
  if (Number.isFinite(minSec) && minSec > 0 && sessionOnlineSeconds(s) < minSec) {
    return false;
  }

  const limitRaw = filters.dnLimitKbps.trim();
  if (limitRaw) {
    const want = Number(limitRaw);
    if (Number.isFinite(want) && want > 0) {
      const got = sessionDnLimitKbps(s);
      if (got !== want) return false;
    }
  }

  return true;
}

export function filterBngSessions(
  sessions: BngSessionLike[],
  searchField: BngSessionSearchField,
  searchQuery: string,
  advanced: BngSessionAdvancedFilters,
): BngSessionLike[] {
  return sessions.filter(
    (s) => matchBngSessionSearch(s, searchField, searchQuery) && applyBngSessionAdvancedFilters(s, advanced),
  );
}

export function countActiveBngSessionFilters(advanced: BngSessionAdvancedFilters): number {
  let n = 0;
  if (advanced.ipv4Like.trim()) n++;
  if (resolveIpTypeFilter(advanced) !== "any") n++;
  if (advanced.vlans.trim()) n++;
  if (advanced.minOnlineSec.trim()) n++;
  if (advanced.dnLimitKbps.trim()) n++;
  return n;
}

/** Label legível do tempo online (para tabela). */
export function sessionTableOnlineLabel(s: BngSessionLike): string {
  return sessionDisplayOnline(s);
}

/** Label legível do limite downstream. */
export function sessionTableDnLimitLabel(s: BngSessionLike): string {
  if (s.car_dn_cir_display?.trim()) return s.car_dn_cir_display;
  return formatBngKbitRate(s.car_dn_cir_kbps);
}
