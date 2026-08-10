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
  a.download = `fleet-${kind}.csv`;
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

export const DRIVER_STATUS = [
  { value: "active", label: "Ativo" },
  { value: "inactive", label: "Inativo" },
  { value: "blocked", label: "Bloqueado" },
];

export const UF_OPTIONS = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG",
  "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO",
];
