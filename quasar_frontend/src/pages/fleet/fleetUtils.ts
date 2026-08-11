import { getAuthToken } from "../../lib/auth";

export function fleetMoney(n: number | null | undefined) {
  if (n == null || !Number.isFinite(n)) return "—";
  return n.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

export function fleetNum(n: number | null | undefined, digits = 2) {
  if (n == null || !Number.isFinite(n)) return "—";
  return n.toLocaleString("pt-BR", { maximumFractionDigits: digits, minimumFractionDigits: 0 });
}

export function monthStartISO() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-01`;
}

export function todayISO() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function addDaysISO(iso: string, days: number) {
  const [y, m, d] = iso.split("-").map(Number);
  const dt = new Date(y, m - 1, d + days);
  return `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, "0")}-${String(dt.getDate()).padStart(2, "0")}`;
}

export function monthEndISO(ym: string) {
  const [y, m] = ym.split("-").map(Number);
  const dt = new Date(y, m, 0);
  return `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, "0")}-${String(dt.getDate()).padStart(2, "0")}`;
}

export async function downloadFleetReport(kind: string, from: string, to: string) {
  const token = getAuthToken();
  const qs = new URLSearchParams({ from, to });
  const res = await fetch(`/api/v1/fleet/reports/${encodeURIComponent(kind)}?${qs}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) throw new Error(`Falha ao exportar (${res.status})`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `frota-${kind}-${from}-${to}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export const VEHICLE_STATUS = [
  { value: "active", label: "Ativo" },
  { value: "inactive", label: "Inativo" },
  { value: "maintenance", label: "Em manutenção" },
  { value: "stopped", label: "Parado" },
  { value: "sold", label: "Vendido" },
  { value: "written_off", label: "Baixado" },
  { value: "rented", label: "Locado" },
];

export function isFleetVehicleInactive(status?: string | null) {
  return status === "inactive";
}

/** Status que não podem receber lançamento manual (importação CSV continua permitida). */
export function isFleetVehicleLaunchBlocked(status?: string | null) {
  return status === "inactive" || status === "sold" || status === "written_off";
}

export function formatISODateBR(iso: string) {
  const [y, m, d] = String(iso ?? "").split("-");
  if (!y || !m || !d) return iso;
  return `${d}/${m}/${y}`;
}

export function fleetDateOnly(iso?: string | null) {
  if (!iso) return "";
  return String(iso).slice(0, 10);
}

export function fleetLicenseExpired(iso?: string | null) {
  const d = fleetDateOnly(iso);
  return !!d && d < todayISO();
}

export const STATION_KIND_LABEL: Record<string, string> = {
  conveniado: "Conveniado",
  proprio: "Próprio",
  fornecedor: "Fornecedor",
  other: "Outro",
};

export const STATION_STATUS_LABEL: Record<string, string> = {
  active: "Ativo",
  inactive: "Inativo",
};

/** Formata placa brasileira: AAA-0000 (antiga) ou AAA-0A00 (Mercosul). */
export function formatFleetPlate(raw: string): string {
  const alnum = String(raw ?? "")
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, "")
    .slice(0, 7);
  if (alnum.length <= 3) return alnum;
  return `${alnum.slice(0, 3)}-${alnum.slice(3)}`;
}

export function isValidFleetPlate(raw: string): boolean {
  return /^[A-Z]{3}-[0-9][A-Z0-9][0-9]{2}$/.test(formatFleetPlate(raw));
}

export const DRIVER_STATUS = [
  { value: "active", label: "Ativo" },
  { value: "inactive", label: "Inativo" },
  { value: "blocked", label: "Bloqueado" },
];

export const UF_OPTIONS = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG",
  "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO",
];
