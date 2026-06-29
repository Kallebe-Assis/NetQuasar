import {
  formatBngIpType,
  formatBngKbitRate,
  sessionDisplayOnline,
  type PppoeSessionFields,
} from "./bngDisplay";

export type BngSessionSearchField = "login" | "ipv4" | "ipv6" | "mac" | "vlan";

export type BngDualStackFilter = "any" | "yes" | "no";

export type BngSessionAdvancedFilters = {
  ipv4Like: string;
  dualStack: BngDualStackFilter;
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
  dualStack: "any",
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

function isDualStackSession(s: BngSessionLike): boolean {
  const t = formatBngIpType(s.ip_type, s.ip_type_raw, s).toLowerCase();
  if (t.includes("v4/v6") || t.includes("dual")) return true;
  const has4 = Boolean(String(s.ipv4 ?? "").trim() && s.ipv4 !== "0.0.0.0");
  const has6 = Boolean(String(s.ipv6 ?? "").trim() || String(s.ipv6_pd ?? "").trim());
  return has4 && has6;
}

export function matchBngSessionSearch(s: BngSessionLike, field: BngSessionSearchField, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  switch (field) {
    case "login":
      return String(s.login ?? "").toLowerCase().includes(q);
    case "ipv4":
      return String(s.ipv4 ?? "").toLowerCase().includes(q);
    case "ipv6":
      return sessionIpv6Haystack(s).includes(q);
    case "mac": {
      const hay = normalizeMacDigits(String(s.mac ?? ""));
      const needle = normalizeMacDigits(q);
      return needle.length > 0 && hay.includes(needle);
    }
    case "vlan":
      return String(s.vlan ?? "").toLowerCase().includes(q);
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
  const ipv4Like = filters.ipv4Like.trim().toLowerCase();
  if (ipv4Like && !String(s.ipv4 ?? "").toLowerCase().includes(ipv4Like)) {
    return false;
  }

  if (filters.dualStack === "yes" && !isDualStackSession(s)) return false;
  if (filters.dualStack === "no" && isDualStackSession(s)) return false;

  const vlans = parseVlanList(filters.vlans);
  if (vlans.length > 0) {
    const sv = String(s.vlan ?? "").trim();
    if (!vlans.some((v) => v === sv)) return false;
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
  if (advanced.dualStack !== "any") n++;
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
