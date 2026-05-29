import { EM_DASH } from "./formatDisplay";

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

/** Formata octetos com base 1000 (B → KB → MB → GB → TB). */
export function formatBytes(bytes: number | null | undefined, opts?: { decimals?: number; maxUnit?: number }): string {
  if (bytes == null || !Number.isFinite(Number(bytes)) || Number(bytes) < 0) return EM_DASH;
  let v = Number(bytes);
  if (v === 0) return "0 B";
  let i = 0;
  const maxUnit = opts?.maxUnit ?? UNITS.length - 1;
  while (v >= 1000 && i < maxUnit) {
    v /= 1000;
    i++;
  }
  const digits =
    opts?.decimals ?? (v >= 100 ? 0 : v >= 10 ? 1 : v >= 1 ? 2 : 3);
  return `${v.toFixed(digits)} ${UNITS[i]}`;
}

/** hrStorage / memória SNMP em KB → exibição legível. */
export function formatKilobytes(kb: number | null | undefined): string {
  if (kb == null || !Number.isFinite(Number(kb))) return EM_DASH;
  return formatBytes(Number(kb) * 1000);
}

/** Timeticks SNMP (centésimos de segundo) → texto legível. */
export function formatUptimeTicks(ticks: unknown): string {
  const n = Number(ticks);
  if (!Number.isFinite(n) || n < 0) return EM_DASH;
  const sec = Math.floor(n / 100);
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0 || d > 0) parts.push(`${h}h`);
  if (m > 0 || h > 0 || d > 0) parts.push(`${m}m`);
  parts.push(`${s}s`);
  return parts.join(" ");
}
